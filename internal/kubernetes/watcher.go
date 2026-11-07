package kubernetes

import (
	"argo-values/internal/logger"
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
)

const (
	// watcherReconnectDelay is the base pause before re-establishing a watch after
	// its channel was closed by the API server or the network.
	watcherReconnectDelay = time.Second
	// watcherMaxReconnectDelay caps the exponential reconnect backoff.
	watcherMaxReconnectDelay = 30 * time.Second
	// watcherResyncInterval triggers a periodic re-list and diff so that changes
	// missed while a watch was disconnected are not lost.
	watcherResyncInterval = 5 * time.Minute
)

// Event holds an event waiting to be flushed.
type Event struct {
	EventType string
	Obj       *unstructured.Unstructured
	Resource  schema.GroupVersionResource
	Namespace string
}

// IsDeleted reports whether the event represents a deleted object.
func (e Event) IsDeleted() bool {
	return e.EventType == string(watch.Deleted)
}

type EventHandler func(map[string]Event)

type ResourceWatcher struct {
	client           *Client
	namespaces       []string
	stopCh           chan struct{}
	stopOnce         sync.Once
	wg               sync.WaitGroup
	ctx              context.Context
	cancelFunc       context.CancelFunc
	eventHandler     EventHandler
	debounceInterval time.Duration

	eventMu       sync.Mutex
	pendingEvents map[string]Event
}

// NewResourceWatcher creates a ResourceWatcher for the given client, namespaces and event handler.
// debounceInterval defines how often pending events are flushed and must be greater than zero.
// It returns an error if debounceInterval is zero or negative. The client is only used once
// Start is called, so it may be nil if the watcher is not started.
func NewResourceWatcher(client *Client, debounceInterval time.Duration, namespaces []string, eventHandler EventHandler) (*ResourceWatcher, error) {
	if debounceInterval <= 0 {
		return nil, fmt.Errorf("refresh interval must be greater than zero, got %s", debounceInterval)
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &ResourceWatcher{
		client:           client,
		namespaces:       normalizeNamespaces(namespaces),
		stopCh:           make(chan struct{}),
		ctx:              ctx,
		cancelFunc:       cancel,
		pendingEvents:    make(map[string]Event),
		eventHandler:     eventHandler,
		debounceInterval: debounceInterval,
	}, nil
}

// normalizeNamespaces trims whitespace, drops empty entries and removes duplicates
// while preserving the original order.
func normalizeNamespaces(namespaces []string) []string {
	normalized := make([]string, 0, len(namespaces))
	seen := make(map[string]struct{}, len(namespaces))
	for _, namespace := range namespaces {
		namespace = strings.TrimSpace(namespace)
		if namespace == "" {
			continue
		}
		if _, ok := seen[namespace]; ok {
			continue
		}
		seen[namespace] = struct{}{}
		normalized = append(normalized, namespace)
	}
	return normalized
}

// Start starts watching all configured resources. Errors for a single resource
// are logged and retried with a backoff instead of stopping the watcher, so a
// failure to watch one resource (for example missing RBAC) does not prevent the
// others from working. Start only returns once Stop is called.
func (w *ResourceWatcher) Start() error {
	if len(w.namespaces) == 0 {
		return fmt.Errorf("no namespaces configured")
	}

	logger.Debug("Starting resource watcher...")

	resourcesToWatch := []schema.GroupVersionResource{
		{Group: "", Version: "v1", Resource: "configmaps"},
		{Group: "", Version: "v1", Resource: "secrets"},
		{Group: "argoproj.io", Version: "v1alpha1", Resource: "applications"},
	}

	for _, ns := range w.namespaces {
		for _, resource := range resourcesToWatch {
			w.wg.Go(func() { w.watchResource(resource, ns) })
		}
	}

	// Start the debounced flush loop
	w.wg.Go(w.flushLoop)

	<-w.stopCh
	w.cancelFunc()
	w.wg.Wait()

	// Final flush after all watch goroutines have finished so events queued
	// shortly before the stop are not lost.
	w.flush()
	return nil
}

// Stop stops the watcher. It is safe to call Stop more than once.
func (w *ResourceWatcher) Stop() {
	w.stopOnce.Do(func() {
		logger.Debug("Stopping resource watcher...")
		w.cancelFunc()
		close(w.stopCh)
	})
}

// flushLoop ticks every debounceInterval and flushes all collected events.
func (w *ResourceWatcher) flushLoop() {
	ticker := time.NewTicker(w.debounceInterval)
	defer ticker.Stop()

	for {
		select {
		case <-w.stopCh:
			// The final flush is performed by Start after all goroutines exited.
			return
		case <-ticker.C:
			w.flush()
		}
	}
}

// flush drains the pending events map and passes each batch to the event handler.
func (w *ResourceWatcher) flush() {
	w.eventMu.Lock()
	events := w.pendingEvents
	w.pendingEvents = make(map[string]Event)
	w.eventMu.Unlock()

	if len(events) > 0 {
		w.eventHandler(events)
	}
}

// watchResource watches a single resource in a namespace. Before every watch it
// lists the current objects and diffs them against the resource versions known
// from the previous watch. This closes the gap between a closed watch and the
// new one, so changes that happened in between are not missed.
//
// Applications are seeded as ADDED on the initial list so the in-memory
// Application -> resources mapping is built. ConfigMaps and Secrets are only
// cached on the initial list, because seeding them would trigger a refresh of
// every application on startup.
func (w *ResourceWatcher) watchResource(resource schema.GroupVersionResource, namespace string) {
	known := make(map[string]string)
	seeded := false
	failures := 0

	for {
		list, err := w.client.Resource(resource).Namespace(namespace).List(w.ctx, v1.ListOptions{})
		if err != nil {
			logger.Errorf("failed to list %s in %s: %v (will retry)", resource.Resource, namespace, err)
			if !w.backoff(failures) {
				return
			}
			failures++
			continue
		}
		failures = 0

		if seeded {
			w.enqueueDiff(resource, namespace, known, list)
		} else if resource.Resource == "applications" {
			w.enqueueAll(resource, namespace, list)
		}
		known = resourceVersions(list)
		seeded = true

		listOptions := v1.ListOptions{ResourceVersion: list.GetResourceVersion()}

		wi, err := w.client.Resource(resource).Namespace(namespace).Watch(w.ctx, listOptions)
		if err != nil {
			logger.Errorf("failed to watch %s in %s: %v (will retry)", resource.Resource, namespace, err)
			if !w.backoff(failures) {
				return
			}
			failures++
			continue
		}
		failures = 0

		resyncTicker := time.NewTicker(watcherResyncInterval)
		channelClosed := false

		for !channelClosed {
			select {
			case <-w.stopCh:
				logger.Debugf("Stopping %s resource watcher in %s ...", resource.Resource, namespace)
				resyncTicker.Stop()
				wi.Stop()
				return
			case <-resyncTicker.C:
				channelClosed = true
			case event, ok := <-wi.ResultChan():
				if !ok {
					channelClosed = true
					break
				}
				w.handleEvent(event, resource, namespace)
				updateResourceVersions(known, event)
				if event.Type == watch.Error {
					// The watch is no longer valid (for example "410 Gone").
					// Re-list and diff instead of just logging the error.
					channelClosed = true
				}
			}
		}

		resyncTicker.Stop()
		wi.Stop()

		if w.ctx.Err() != nil {
			return
		}

		logger.Debugf("Watch channel closed for %s in %s, reconnecting ...", resource.Resource, namespace)
		if !w.backoff(0) {
			return
		}
	}
}

// enqueueAll queues every object of a list as an ADDED event. It writes under
// eventMu into pendingEvents instead of calling the event handler directly,
// because the handler is invoked by the flush goroutine and must not race it.
func (w *ResourceWatcher) enqueueAll(resource schema.GroupVersionResource, namespace string, list *unstructured.UnstructuredList) {
	w.eventMu.Lock()
	defer w.eventMu.Unlock()

	for i := range list.Items {
		obj := &list.Items[i]
		w.pendingEvents[resourceKey(resource, namespace, obj.GetName())] = Event{
			EventType: string(watch.Added),
			Obj:       obj,
			Resource:  resource,
			Namespace: namespace,
		}
	}
}

// enqueueDiff compares a freshly listed set of objects with the known resource
// versions and enqueues ADDED/MODIFIED/DELETED events for the differences.
func (w *ResourceWatcher) enqueueDiff(resource schema.GroupVersionResource, namespace string, known map[string]string, list *unstructured.UnstructuredList) {
	w.eventMu.Lock()
	defer w.eventMu.Unlock()

	seen := make(map[string]struct{}, len(list.Items))
	for i := range list.Items {
		obj := &list.Items[i]
		name := obj.GetName()
		seen[name] = struct{}{}

		knownVersion, existed := known[name]
		if existed && knownVersion == obj.GetResourceVersion() {
			continue
		}

		eventType := string(watch.Added)
		if existed {
			eventType = string(watch.Modified)
		}

		w.pendingEvents[resourceKey(resource, namespace, name)] = Event{
			EventType: eventType,
			Obj:       obj,
			Resource:  resource,
			Namespace: namespace,
		}
	}

	for name := range known {
		if _, ok := seen[name]; ok {
			continue
		}
		w.pendingEvents[resourceKey(resource, namespace, name)] = Event{
			EventType: string(watch.Deleted),
			Obj:       minimalObject(resource, namespace, name),
			Resource:  resource,
			Namespace: namespace,
		}
	}
}

// resourceVersions returns a map from object name to resource version. Only the
// resource version is kept, so no object payloads (in particular no secret
// data) are retained across watch reconnects.
func resourceVersions(list *unstructured.UnstructuredList) map[string]string {
	versions := make(map[string]string, len(list.Items))
	for i := range list.Items {
		obj := &list.Items[i]
		versions[obj.GetName()] = obj.GetResourceVersion()
	}
	return versions
}

// updateResourceVersions keeps the local resource version cache in sync with a
// watch event.
func updateResourceVersions(known map[string]string, event watch.Event) {
	obj, ok := event.Object.(*unstructured.Unstructured)
	if !ok {
		return
	}

	switch event.Type {
	case watch.Added, watch.Modified:
		known[obj.GetName()] = obj.GetResourceVersion()
	case watch.Deleted:
		delete(known, obj.GetName())
	}
}

// enqueue stores a single event for the next flush. It is safe for concurrent
// use because pendingEvents is guarded by eventMu.
func (w *ResourceWatcher) enqueue(resource schema.GroupVersionResource, namespace string, obj *unstructured.Unstructured, eventType string) {
	w.eventMu.Lock()
	defer w.eventMu.Unlock()

	w.pendingEvents[resourceKey(resource, namespace, obj.GetName())] = Event{
		EventType: eventType,
		Obj:       obj,
		Resource:  resource,
		Namespace: namespace,
	}
}

// resourceKey builds the stable key used for pending events and the
// Application -> resources mapping.
func resourceKey(resource schema.GroupVersionResource, namespace, name string) string {
	return fmt.Sprintf("%s/%s/%s", resource.Resource, namespace, name)
}

// minimalObject builds a minimal object for a resource that no longer exists in
// the list. It carries just enough information for the event handler.
func minimalObject(resource schema.GroupVersionResource, namespace, name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resource.GroupVersion().String(),
			"kind":       kindForResource(resource),
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
			},
		},
	}
}

func kindForResource(resource schema.GroupVersionResource) string {
	switch resource.Resource {
	case "configmaps":
		return "ConfigMap"
	case "secrets":
		return "Secret"
	case "applications":
		return "Application"
	default:
		return ""
	}
}

// backoff waits for an exponentially growing delay (capped at
// watcherMaxReconnectDelay) before retrying a failed list or watch. It returns
// false if the watcher was stopped during the wait.
func (w *ResourceWatcher) backoff(attempt int) bool {
	if attempt < 0 {
		attempt = 0
	}
	// 1s << 5 = 32s is already above the cap, so larger attempts add nothing.
	if attempt > 5 {
		attempt = 5
	}

	delay := min(watcherReconnectDelay<<attempt, watcherMaxReconnectDelay)

	select {
	case <-w.stopCh:
		return false
	case <-time.After(delay):
		return true
	}
}

func (w *ResourceWatcher) handleEvent(event watch.Event, resource schema.GroupVersionResource, namespace string) {
	switch event.Type {
	case watch.Added, watch.Modified, watch.Deleted:
		unstructuredObj, ok := event.Object.(*unstructured.Unstructured)
		if !ok {
			logger.Errorf("Received non-unstructured object: %T", event.Object)
			return
		}

		w.enqueue(resource, namespace, unstructuredObj, string(event.Type))

	case watch.Error:
		if status, ok := event.Object.(*v1.Status); ok {
			logger.Errorf("Error watching %s in %s: %v", resource.Resource, namespace, status.Message)
		} else {
			logger.Errorf("Error watching %s in %s: %v", resource.Resource, namespace, event.Object)
		}
	}
}
