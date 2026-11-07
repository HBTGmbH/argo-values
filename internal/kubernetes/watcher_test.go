package kubernetes

import (
	"context"
	"fmt"
	"maps"
	"strings"
	"sync"
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

func testObject(name, resourceVersion string) unstructured.Unstructured {
	obj := unstructured.Unstructured{Object: map[string]any{}}
	obj.SetName(name)
	obj.SetResourceVersion(resourceVersion)
	return obj
}

func pendingEventsSnapshot(w *ResourceWatcher) map[string]Event {
	w.eventMu.Lock()
	defer w.eventMu.Unlock()

	snapshot := make(map[string]Event, len(w.pendingEvents))
	maps.Copy(snapshot, w.pendingEvents)
	return snapshot
}

func TestDiffResources(t *testing.T) {
	watcher, err := NewResourceWatcher(nil, time.Second, nil, nil)
	if err != nil {
		t.Fatalf("NewResourceWatcher returned error: %v", err)
	}

	resource := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}
	known := map[string]string{"a": "1", "b": "2"}
	list := &unstructured.UnstructuredList{
		Items: []unstructured.Unstructured{
			testObject("a", "1"),
			testObject("c", "3"),
		},
	}

	watcher.enqueueDiff(resource, "ns", known, list)
	events := pendingEventsSnapshot(watcher)

	if len(events) != 2 {
		t.Fatalf("expected 2 diff events, got %d: %v", len(events), events)
	}

	if _, ok := events["configmaps/ns/a"]; ok {
		t.Errorf("expected no event for unchanged object a")
	}

	added, ok := events["configmaps/ns/c"]
	if !ok {
		t.Fatalf("expected ADDED event for c, got %v", events)
	}
	if added.EventType != string(watch.Added) {
		t.Errorf("expected ADDED event for c, got %q", added.EventType)
	}
	if added.Resource != resource {
		t.Errorf("expected resource %v for c, got %v", resource, added.Resource)
	}
	if added.Obj.GetName() != "c" {
		t.Errorf("expected object c, got %s", added.Obj.GetName())
	}

	deleted, ok := events["configmaps/ns/b"]
	if !ok {
		t.Fatalf("expected DELETED event for b, got %v", events)
	}
	if deleted.EventType != string(watch.Deleted) {
		t.Errorf("expected DELETED event for b, got %q", deleted.EventType)
	}
	if deleted.Resource != resource {
		t.Errorf("expected resource %v for b, got %v", resource, deleted.Resource)
	}
	if deleted.Obj.GetName() != "b" || deleted.Obj.GetNamespace() != "ns" || deleted.Obj.GetKind() != "ConfigMap" {
		t.Errorf("unexpected minimal deleted object: %v", deleted.Obj.Object)
	}
}

func TestResourceWatcherDoesNotSeedConfigMaps(t *testing.T) {
	gvrToListKind := map[schema.GroupVersionResource]string{
		{Group: "argoproj.io", Version: "v1alpha1", Resource: "applications"}: "ApplicationList",
		{Group: "", Version: "v1", Resource: "configmaps"}:                    "ConfigMapList",
		{Group: "", Version: "v1", Resource: "secrets"}:                       "SecretList",
	}

	configMap := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]any{
				"name":      "cm",
				"namespace": "argocd",
			},
		},
	}

	fakeClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), gvrToListKind, configMap)
	client := &Client{client: fakeClient}

	watcher, err := NewResourceWatcher(client, time.Hour, []string{"argocd"}, func(map[string]Event) {})
	if err != nil {
		t.Fatalf("NewResourceWatcher returned error: %v", err)
	}

	startDone := make(chan error, 1)
	go func() {
		startDone <- watcher.Start()
	}()

	time.Sleep(100 * time.Millisecond)

	for key := range pendingEventsSnapshot(watcher) {
		if strings.HasPrefix(key, "configmaps/") || strings.HasPrefix(key, "secrets/") {
			t.Errorf("expected no seeded event for %q", key)
		}
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

func TestStopIsIdempotent(t *testing.T) {
	watcher, err := NewResourceWatcher(nil, time.Second, nil, nil)
	if err != nil {
		t.Fatalf("NewResourceWatcher returned error: %v", err)
	}

	watcher.Stop()
	watcher.Stop()
}

func TestStartRejectsEmptyNamespaces(t *testing.T) {
	watcher, err := NewResourceWatcher(nil, time.Second, []string{"", " "}, nil)
	if err != nil {
		t.Fatalf("NewResourceWatcher returned error: %v", err)
	}

	if err := watcher.Start(); err == nil {
		t.Fatal("expected Start to reject empty namespaces, got nil error")
	} else if !strings.Contains(err.Error(), "no namespaces configured") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// channelWatch is a minimal watch.Interface backed by a channel. Stop is
// idempotent because the test closes the channel to simulate a disconnect and
// the watcher then calls Stop as well.
type channelWatch struct {
	ch   chan watch.Event
	once sync.Once
}

func newChannelWatch() *channelWatch {
	return &channelWatch{ch: make(chan watch.Event)}
}

func (w *channelWatch) ResultChan() <-chan watch.Event {
	return w.ch
}

func (w *channelWatch) Stop() {
	w.once.Do(func() { close(w.ch) })
}

// close simulates the API server closing the watch channel.
func (w *channelWatch) close() {
	w.Stop()
}

type watchScript struct {
	mu         sync.Mutex
	lists      []*unstructured.UnstructuredList
	listIndex  int
	watches    []*channelWatch
	watchIndex int
	ready      chan int
}

func (s *watchScript) nextList() (*unstructured.UnstructuredList, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.listIndex >= len(s.lists) {
		return &unstructured.UnstructuredList{}, nil
	}
	list := s.lists[s.listIndex]
	s.listIndex++
	return list, nil
}

func (s *watchScript) nextWatch() (watch.Interface, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.watchIndex >= len(s.watches) {
		s.watches = append(s.watches, newChannelWatch())
	}
	w := s.watches[s.watchIndex]
	s.watchIndex++
	s.ready <- s.watchIndex
	return w, nil
}

func (s *watchScript) getWatch(index int) *channelWatch {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.watches[index]
}

type scriptedResourceClient struct {
	dynamic.Interface
	script *watchScript
}

func (c *scriptedResourceClient) Resource(schema.GroupVersionResource) dynamic.NamespaceableResourceInterface {
	return &scriptedResource{script: c.script}
}

type scriptedResource struct {
	dynamic.NamespaceableResourceInterface
	script *watchScript
}

func (r *scriptedResource) Namespace(string) dynamic.ResourceInterface {
	return &scriptedNamespaced{script: r.script}
}

type scriptedNamespaced struct {
	dynamic.ResourceInterface
	script *watchScript
}

func (r *scriptedNamespaced) List(context.Context, metav1.ListOptions) (*unstructured.UnstructuredList, error) {
	return r.script.nextList()
}

func (r *scriptedNamespaced) Watch(context.Context, metav1.ListOptions) (watch.Interface, error) {
	return r.script.nextWatch()
}

func TestResourceWatcherDiffOnReconnect(t *testing.T) {
	list := func(items ...unstructured.Unstructured) *unstructured.UnstructuredList {
		return &unstructured.UnstructuredList{Items: items}
	}

	resource := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}
	script := &watchScript{
		lists: []*unstructured.UnstructuredList{
			list(testObject("a", "1")),
			list(testObject("a", "2"), testObject("c", "3")),
			list(),
		},
		ready: make(chan int, 8),
	}

	client := &Client{client: &scriptedResourceClient{script: script}}
	watcher, err := NewResourceWatcher(client, time.Hour, []string{"argocd"}, func(map[string]Event) {})
	if err != nil {
		t.Fatalf("NewResourceWatcher returned error: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		watcher.watchResource(resource, "argocd")
	}()

	waitForWatch := func(want int) {
		t.Helper()
		select {
		case got := <-script.ready:
			if got != want {
				t.Fatalf("expected watch %d, got %d", want, got)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for watch %d", want)
		}
	}

	// Initial list: only the cache is filled, no events.
	waitForWatch(1)
	if events := pendingEventsSnapshot(watcher); len(events) != 0 {
		t.Fatalf("expected no events after initial seed, got %v", events)
	}

	// Close the watch: the re-list reports a change and a new object.
	script.getWatch(0).close()
	waitForWatch(2)

	events := pendingEventsSnapshot(watcher)
	modified, ok := events["configmaps/argocd/a"]
	if !ok || modified.EventType != string(watch.Modified) {
		t.Fatalf("expected MODIFIED event for a, got %v", events)
	}
	added, ok := events["configmaps/argocd/c"]
	if !ok || added.EventType != string(watch.Added) {
		t.Fatalf("expected ADDED event for c, got %v", events)
	}

	// Close again: the object has disappeared from the list.
	script.getWatch(1).close()
	waitForWatch(3)

	events = pendingEventsSnapshot(watcher)
	for _, name := range []string{"a", "c"} {
		deleted, ok := events["configmaps/argocd/"+name]
		if !ok || deleted.EventType != string(watch.Deleted) {
			t.Fatalf("expected DELETED event for %s, got %v", name, events)
		}
	}

	watcher.Stop()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("watchResource did not return after Stop")
	}
}

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
		Object: map[string]any{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "Application",
			"metadata": map[string]any{
				"name":      "app1",
				"namespace": "argocd",
			},
			"spec": map[string]any{
				"source": map[string]any{
					"plugin": map[string]any{
						"env": []any{
							map[string]any{"name": "value-configs", "value": "cm"},
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
	env, ok := envs[0].(map[string]any)
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
		Object: map[string]any{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "Application",
			"metadata": map[string]any{
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
