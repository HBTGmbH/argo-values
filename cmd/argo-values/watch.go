package main

import (
	"argo-values/internal/kubernetes"
	"argo-values/internal/logger"
	"argo-values/internal/utils"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/strings/slices"
)

// refreshConcurrency limits how many application refreshes run in parallel.
const refreshConcurrency = 5

type WatchCmd struct {
	RefreshInterval int      `name:"refresh-interval" short:"i" env:"ARGOCD_VALUES_REFRESH_SECONDS" default:"2" help:"Debounce interval in seconds for resource change events"`
	Namespaces      []string `name:"namespaces" env:"ARGOCD_VALUES_NAMESPACES" default:"argocd,default" help:"Namespaces to watch for resource changes"`
}

func (WatchCmd) Help() string {
	return `Watches Kubernetes resources in specified namespaces and automatically refreshes
Argo CD applications when their dependent resources (ConfigMaps, Secrets) change.

This command establishes watches on ConfigMaps and Secrets in the specified namespaces
and triggers application refreshes when any watched resource changes. The refresh
debounce multiple rapid changes according to the refresh-interval flag.

The command maintains a mapping of which applications depend on which resources and
only refreshes applications that are affected by the specific resource changes.

Use this command to enable automatic application updates when configuration changes
without requiring manual intervention or waiting for the next reconciliation cycle.`
}

func (c *WatchCmd) Run(g *Global) error {
	logger.Debug("Connecting to Kubernetes cluster")
	kubeClient, err := kubernetes.NewClient(g.KubeConfigPath)
	if err != nil {
		logger.Fatalf("Failed to create Kubernetes client: %v", err)
	}

	// Create and start the watcher
	watcher, err := kubernetes.NewResourceWatcher(
		kubeClient, time.Duration(c.RefreshInterval)*time.Second, c.Namespaces,
		updateResources(g.DryRun, kubeClient.RefreshApplication, make(map[string][]string)),
	)
	if err != nil {
		logger.Fatalf("Failed to create watcher: %v", err)
	}

	// Handle interrupt signal for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		logger.Info("Received interrupt signal, shutting down...")
		watcher.Stop()
	}()

	// Start watching
	if err := watcher.Start(); err != nil {
		logger.Fatalf("Start watching resources failed: %v", err)
	}

	logger.Info("Watcher stopped successfully")

	return nil
}

type refreshApplicationFunc func(name, namespace string) error

// updateResources returns an event handler that keeps the in-memory mapping of
// Applications to the ConfigMaps/Secrets they reference up to date and refreshes
// the Applications affected by changed resources.
//
// applicationResources is keyed by "applications/<namespace>/<name>" and holds the
// resource keys ("configmaps/<namespace>/<name>" or "secrets/<namespace>/<name>")
// referenced by that Application.
func updateResources(dryRun bool, refresh refreshApplicationFunc, applicationResources map[string][]string) kubernetes.EventHandler {
	return func(events map[string]kubernetes.Event) {
		logger.Debugf("Found %d changed resources", len(events))

		// applicationsInBatch records which Applications had an event in this batch.
		// Key presence means "this Application was seen in the current batch"; the
		// value means "this Application was already known before the batch".
		//
		// The first loop updates the Application -> referenced ConfigMaps/Secrets
		// mapping and collects the changed ConfigMaps/Secrets (everything that is
		// not an Application event).
		var changedResources []string
		applicationsInBatch := make(map[string]bool)
		for key, event := range events {
			if event.Resource.Resource == "applications" {
				if event.IsDeleted() {
					delete(applicationResources, key)
					continue
				}
				_, wasKnown := applicationResources[key]
				applicationsInBatch[key] = wasKnown
				applicationResources[key] = getApplicationResources(event.Obj)
			} else {
				changedResources = append(changedResources, key)
			}
		}

		// The second loop deliberately iterates over ALL known Applications, not just
		// the ones in this batch. This way Applications that did not receive their own
		// event are still refreshed when one of their ConfigMaps/Secrets changed.
		var affectedApplications []string
		for _, changedResource := range changedResources {
			for application, resources := range applicationResources {
				wasKnown, inBatch := applicationsInBatch[application]
				// New Applications seen for the first time in this batch are not
				// refreshed immediately: initial seeding at startup must not refresh
				// every Application. Already known Applications are still refreshed.
				if inBatch && !wasKnown {
					continue
				}
				if !slices.Contains(affectedApplications, application) && slices.Contains(resources, changedResource) {
					affectedApplications = append(affectedApplications, application)
				}
			}
		}

		var wg sync.WaitGroup
		sem := make(chan struct{}, refreshConcurrency)
		for _, key := range affectedApplications {
			parts := strings.Split(key, "/")
			if len(parts) != 3 {
				logger.Errorf("Invalid application key %s found!", key)
				continue
			}

			namespace := parts[1]
			name := parts[2]
			logger.Debugf("Refresh application %s in namespace %s", name, namespace)

			if dryRun {
				continue
			}

			wg.Add(1)
			sem <- struct{}{}
			go func(name, namespace string) {
				defer wg.Done()
				defer func() { <-sem }()
				if err := refresh(name, namespace); err != nil {
					logger.Errorf("Failed to refresh application %s in namespace %s: %v", name, namespace, err)
				}
			}(name, namespace)
		}
		wg.Wait()
	}
}

func getApplicationResources(obj *unstructured.Unstructured) []string {
	var resources []string

	spec, found, err := unstructured.NestedMap(obj.Object, "spec")
	if err != nil || !found {
		return resources
	}

	defaultNamespace := destinationNamespace(spec)

	if source, found, err := unstructured.NestedMap(spec, "source"); err == nil && found {
		resources = append(resources, getResourcesFromSource(source, defaultNamespace)...)
	}

	if sources, found, err := unstructured.NestedSlice(spec, "sources"); err == nil && found {
		for _, source := range sources {
			sourceMap, ok := source.(map[string]any)
			if !ok {
				continue
			}
			resources = append(resources, getResourcesFromSource(sourceMap, defaultNamespace)...)
		}
	}

	return resources
}

func getResourcesFromSource(source map[string]any, defaultNamespace string) []string {
	var resources []string

	plugin, found, err := unstructured.NestedMap(source, "plugin")
	if err != nil || !found {
		return resources
	}

	envs, found, err := unstructured.NestedSlice(plugin, "env")
	if err == nil && found && envs != nil {
		resources = append(resources, getApplicationResourcesFromEnv(envs, defaultNamespace)...)
	}

	parameters, found, err := unstructured.NestedSlice(plugin, "parameters")
	if err == nil && found && parameters != nil {
		resources = append(resources, getApplicationResourcesFromParameters(parameters, defaultNamespace)...)
	}

	return resources
}

// destinationNamespace returns the destination namespace of an Application,
// falling back to "default" like init does when ARGOCD_APP_NAMESPACE is unset.
func destinationNamespace(spec map[string]any) string {
	namespace, found, err := unstructured.NestedString(spec, "destination", "namespace")
	if err == nil && found && namespace != "" {
		return namespace
	}
	return "default"
}

func getApplicationResourcesFromParameters(parameters []any, defaultNamespace string) []string {
	var resources []string

	for _, param := range parameters {
		if paramMap, ok := param.(map[string]any); ok {
			kind := getKindFromName(utils.GetStringFromMap(paramMap, "name"))
			if kind == "" {
				continue
			}
			resources = append(resources, getKeysFromObjs(kind, utils.Trimmed(utils.GetArrayFromMap(paramMap, "array")), defaultNamespace)...)
		}
	}

	return resources
}

func getApplicationResourcesFromEnv(envs []any, defaultNamespace string) []string {
	var resources []string

	for _, env := range envs {
		if envMap, ok := env.(map[string]any); ok {
			kind := getKindFromName(utils.GetStringFromMap(envMap, "name"))
			if kind == "" {
				continue
			}
			resources = append(resources, getKeysFromObjs(kind, utils.Trimmed(strings.Split(utils.GetStringFromMap(envMap, "value"), ",")), defaultNamespace)...)
		}
	}

	return resources
}
func getKindFromName(name string) string {
	name = strings.ToLower(strings.Replace(name, "-", "", 1))
	if strings.HasPrefix(name, "env") || strings.HasPrefix(name, "value") {
		if strings.HasSuffix(name, "configs") {
			return "configmaps"
		}
		if strings.HasSuffix(name, "secrets") {
			return "secrets"
		}
	}
	return ""
}

func getKeysFromObjs(kind string, names []string, defaultNamespace string) []string {
	keys := make([]string, 0, len(names))
	for _, name := range names {
		namespace, resource, err := kubernetes.ParseResourceReference(name, defaultNamespace)
		if err != nil {
			logger.Warnf("Ignoring invalid %s reference %q: %v", kind, name, err)
			continue
		}
		keys = append(keys, fmt.Sprintf("%s/%s/%s", kind, namespace, resource))
	}
	return keys
}
