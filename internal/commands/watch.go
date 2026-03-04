package commands

import (
	"argo-values/internal/config"
	"argo-values/internal/kubernetes"
	"argo-values/internal/logger"
	"argo-values/internal/utils"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/strings/slices"
)

var (
	debounceDuration time.Duration
	namespaces       []string
)

func init() {
	WatchCmd.Flags().DurationVarP(&debounceDuration, "event-interval", "i", 2*time.Second, "Debounce interval for resource change events")
	WatchCmd.Flags().StringArrayVar(&namespaces, "namespaces", config.GetArrayFromEnv("ARGOCD_VALUES_NAMESPACES", []string{"argocd", "default"}), "Namespaces to watch for resource changes")
}

var WatchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Watch Kubernetes resources and refresh Argo applications",
	Long: `Watches Kubernetes resources in specified namespaces and automatically refreshes
Argo CD applications when their dependent resources (ConfigMaps, Secrets) change.

This command establishes watches on ConfigMaps and Secrets in the specified namespaces
and triggers application refreshes when any watched resource changes. The refresh
debounce multiple rapid changes according to the event-interval flag.

The command maintains a mapping of which applications depend on which resources and
only refreshes applications that are affected by the specific resource changes.

Use this command to enable automatic application updates when configuration changes
without requiring manual intervention or waiting for the next reconciliation cycle.`,

	Run: func(cmd *cobra.Command, args []string) {
		logger.Debug("Connecting to Kubernetes cluster")
		kubeClient, err := kubernetes.NewClient(config.KubeConfigPath)
		if err != nil {
			logger.Fatalf("Failed to create Kubernetes client: %v", err)
		}

		// Create and start the watcher
		watcher, err := kubernetes.NewResourceWatcher(
			kubeClient, debounceDuration, namespaces,
			updateResources(config.DryRun, kubeClient, make(map[string][]string)),
		)
		if err != nil {
			log.Fatalf("Failed to create watcher: %v", err)
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
	},
}

func updateResources(dryRun bool, kubeClient *kubernetes.Client, applicationResources map[string][]string) kubernetes.EventHandler {
	return func(events map[string]kubernetes.Event) {
		logger.Debugf("Found %d changed resources", len(events))

		var otherResources []string
		existingApplications := make(map[string]bool)
		for key, event := range events {
			if event.Obj.GetKind() == "Application" {
				_, existingApplications[key] = applicationResources[key]
				applicationResources[key] = getApplicationResources(event.Obj)
			} else {
				otherResources = append(otherResources, key)
			}
		}

		var changedApplications []string
		for _, changedResource := range otherResources {
			for application, key := range applicationResources {
				existing, found := existingApplications[application]
				if !existing && found {
					continue
				}
				if !slices.Contains(changedApplications, application) && slices.Contains(key, changedResource) {
					changedApplications = append(changedApplications, application)
					break
				}
			}
		}

		for _, key := range changedApplications {
			parts := strings.Split(key, "/")
			if len(parts) != 3 {
				logger.Errorf("Invalid application key %s found!", key)
				continue
			}

			namespace := parts[1]
			name := parts[2]
			logger.Debugf("Refresh application %s in namespace %s", name, namespace)

			if !dryRun {
				err := kubeClient.RefreshApplication(name, namespace)
				if err != nil {
					logger.Errorf("Failed to refresh application %s in namespace %s: %v", name, namespace, err)
				}
			}
		}
	}
}

func getApplicationResources(obj *unstructured.Unstructured) []string {
	var resources []string

	spec, found, err := unstructured.NestedMap(obj.Object, "spec")
	if err != nil || !found {
		return resources
	}

	source, found, err := unstructured.NestedMap(spec, "source")
	if err != nil || !found {
		return resources
	}

	plugin, found, err := unstructured.NestedMap(source, "plugin")
	if err != nil || !found {
		return resources
	}

	envs, found, err := unstructured.NestedSlice(plugin, "env")
	if err == nil && found && envs != nil {
		resources = append(resources, getApplicationResourcesFromEnv(envs)...)
	}

	parameters, found, err := unstructured.NestedSlice(plugin, "parameters")
	if err == nil && found && parameters != nil {
		resources = append(resources, getApplicationResourcesFromParameters(parameters)...)
	}

	return resources
}

func getApplicationResourcesFromParameters(parameters []interface{}) []string {
	var resources []string

	for _, param := range parameters {
		if paramMap, ok := param.(map[string]interface{}); ok {
			kind := getKindFromName(utils.GetStringFromMap(paramMap, "name"))
			if kind == "" {
				continue
			}
			resources = append(resources, getKeysFromObjs(kind, utils.NonEmpty(utils.GetArrayFromMap(paramMap, "array")))...)
		}
	}

	return resources
}

func getApplicationResourcesFromEnv(envs []interface{}) []string {
	var resources []string

	for _, env := range envs {
		if envMap, ok := env.(map[string]interface{}); ok {
			kind := getKindFromName(utils.GetStringFromMap(envMap, "name"))
			if kind == "" {
				continue
			}
			resources = append(resources, getKeysFromObjs(kind, utils.NonEmpty(strings.Split(utils.GetStringFromMap(envMap, "value"), ",")))...)
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

func getKeysFromObjs(kind string, names []string) []string {
	for i, name := range names {
		if strings.Index(name, "/") > 0 {
			names[i] = fmt.Sprintf("%s/%s", kind, name)
		} else {
			names[i] = fmt.Sprintf("%s/default/%s", kind, name)
		}
	}
	return names
}
