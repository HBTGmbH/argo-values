package kubernetes

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func TestNewResourceWatcherRejectsNonPositiveInterval(t *testing.T) {
	tests := []struct {
		name     string
		interval time.Duration
	}{
		{name: "zero", interval: 0},
		{name: "negative", interval: -1 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			watcher, err := NewResourceWatcher(nil, tt.interval, nil, nil)
			if err == nil {
				t.Fatalf("NewResourceWatcher(%v) returned no error, want error", tt.interval)
			}
			if watcher != nil {
				t.Errorf("NewResourceWatcher(%v) returned watcher %v, want nil", tt.interval, watcher)
			}
			if !strings.Contains(err.Error(), "refresh interval") {
				t.Errorf("error message %q does not contain %q", err.Error(), "refresh interval")
			}
		})
	}
}

func TestNewResourceWatcherAcceptsPositiveInterval(t *testing.T) {
	watcher, err := NewResourceWatcher(nil, time.Second, nil, nil)
	if err != nil {
		t.Fatalf("NewResourceWatcher(time.Second) returned error: %v", err)
	}
	if watcher == nil {
		t.Fatal("NewResourceWatcher(time.Second) returned nil watcher, want non-nil")
	}
}

func TestResourceWatcherSeedsExistingApplications(t *testing.T) {
	gvrToListKind := map[schema.GroupVersionResource]string{
		{Group: "argoproj.io", Version: "v1alpha1", Resource: "applications"}: "ApplicationList",
		{Group: "", Version: "v1", Resource: "configmaps"}:                    "ConfigMapList",
		{Group: "", Version: "v1", Resource: "secrets"}:                       "SecretList",
	}

	appObj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "Application",
			"metadata": map[string]interface{}{
				"name":      "app1",
				"namespace": "argocd",
			},
			"spec": map[string]interface{}{
				"source": map[string]interface{}{
					"plugin": map[string]interface{}{
						"env": []interface{}{
							map[string]interface{}{"name": "value-configs", "value": "cm"},
						},
					},
				},
			},
		},
	}

	fakeClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), gvrToListKind, appObj)
	client := &Client{client: fakeClient}

	batches := make(chan map[string]Event, 10)
	handler := func(events map[string]Event) {
		batches <- events
	}

	watcher, err := NewResourceWatcher(client, 10*time.Millisecond, []string{"argocd"}, handler)
	if err != nil {
		t.Fatalf("NewResourceWatcher returned error: %v", err)
	}

	startDone := make(chan error, 1)
	go func() {
		startDone <- watcher.Start()
	}()

	deadline := time.After(2 * time.Second)

	var seeded Event
	found := false
	for !found {
		select {
		case batch := <-batches:
			event, ok := batch["applications/argocd/app1"]
			if !ok {
				continue
			}
			seeded = event
			found = true
		case <-deadline:
			watcher.Stop()
			t.Fatal("timed out waiting for seeded application event")
		}
	}

	if seeded.EventType != string(watch.Added) {
		t.Errorf("expected event type %q, got %q", watch.Added, seeded.EventType)
	}
	if seeded.Obj.GetName() != "app1" || seeded.Obj.GetNamespace() != "argocd" {
		t.Errorf("expected app1 in argocd, got %s in %s", seeded.Obj.GetName(), seeded.Obj.GetNamespace())
	}
	envs, foundEnvs, err := unstructured.NestedSlice(seeded.Obj.Object, "spec", "source", "plugin", "env")
	if err != nil || !foundEnvs || len(envs) != 1 {
		t.Fatalf("expected one plugin env entry, got %v (found=%v, err=%v)", envs, foundEnvs, err)
	}
	env, ok := envs[0].(map[string]interface{})
	if !ok || env["value"] != "cm" {
		t.Errorf("expected env value %q, got %v", "cm", envs[0])
	}

	watcher.Stop()

	select {
	case err := <-startDone:
		if err != nil {
			t.Fatalf("Start returned error after Stop: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return within 2s after Stop")
	}
}

type failingWatchClient struct{ dynamic.Interface }

func (c failingWatchClient) Resource(gvr schema.GroupVersionResource) dynamic.NamespaceableResourceInterface {
	return failingWatchResource{c.Interface.Resource(gvr)}
}

type failingWatchResource struct {
	dynamic.NamespaceableResourceInterface
}

func (r failingWatchResource) Namespace(ns string) dynamic.ResourceInterface {
	return failingWatchNamespaced{r.NamespaceableResourceInterface.Namespace(ns)}
}

type failingWatchNamespaced struct{ dynamic.ResourceInterface }

func (r failingWatchNamespaced) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return nil, fmt.Errorf("simulated watch failure")
}

func TestResourceWatcherSurvivesWatchErrors(t *testing.T) {
	gvrToListKind := map[schema.GroupVersionResource]string{
		{Group: "argoproj.io", Version: "v1alpha1", Resource: "applications"}: "ApplicationList",
		{Group: "", Version: "v1", Resource: "configmaps"}:                    "ConfigMapList",
		{Group: "", Version: "v1", Resource: "secrets"}:                       "SecretList",
	}

	appObj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "Application",
			"metadata": map[string]interface{}{
				"name":      "app1",
				"namespace": "argocd",
			},
		},
	}

	fakeClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), gvrToListKind, appObj)
	client := &Client{client: failingWatchClient{fakeClient}}

	watcher, err := NewResourceWatcher(client, 10*time.Millisecond, []string{"argocd"}, func(map[string]Event) {})
	if err != nil {
		t.Fatalf("NewResourceWatcher returned error: %v", err)
	}

	startDone := make(chan error, 1)
	go func() {
		startDone <- watcher.Start()
	}()

	select {
	case err := <-startDone:
		watcher.Stop()
		t.Fatalf("Start returned early despite watch errors: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	watcher.Stop()

	select {
	case err := <-startDone:
		if err != nil {
			t.Fatalf("Start returned error after Stop: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return within 2s after Stop")
	}
}
