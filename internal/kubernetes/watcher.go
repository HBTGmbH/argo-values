package kubernetes

import (
	"argo-values/internal/logger"
	"context"
	"fmt"
	"sync"
	"time"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
)

// watcherReconnectDelay is the pause before re-establishing a watch after its
// channel was closed by the API server or the network.
const watcherReconnectDelay = time.Second

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
	wg               sync.WaitGroup
	ctx              context.Context
	cancelFunc       context.CancelFunc
	errorCh          chan error
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
		namespaces:       namespaces,
		stopCh:           make(chan struct{}),
		ctx:              ctx,
		cancelFunc:       cancel,
		errorCh:          make(chan error, 1),
		pendingEvents:    make(map[string]Event),
		eventHandler:     eventHandler,
		debounceInterval: debounceInterval,
	}, nil
}

func (w *ResourceWatcher) Start() error {
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

	select {
	case err := <-w.errorCh:
		w.Stop()
		w.wg.Wait()
		return err
	case <-w.stopCh:
		w.wg.Wait()
		return nil
	}
}

func (w *ResourceWatcher) Stop() {
	logger.Debug("Stopping resource watcher...")
	w.cancelFunc()
	close(w.stopCh)
}

// flushLoop ticks every debounceInterval and logs all collected events.
func (w *ResourceWatcher) flushLoop() {
	ticker := time.NewTicker(w.debounceInterval)
	defer ticker.Stop()

	for {
		select {
		case <-w.stopCh:
			// Final flush before exit
			w.flush()
			return
		case <-ticker.C:
			w.flush()
		}
	}
}

// flush drains the pending events map and logs each one.
func (w *ResourceWatcher) flush() {
	w.eventMu.Lock()
	events := w.pendingEvents
	w.pendingEvents = make(map[string]Event)
	w.eventMu.Unlock()

	if len(events) > 0 {
		w.eventHandler(events)
	}
}

// watchResource watches a single resource in a namespace. For applications it
// first lists the existing objects so they are known before any change arrives,
// then starts the watch from the returned resource version. Whenever the watch
// channel closes (API timeout, "resourceVersion too old", network failure) it
// reconnects after watcherReconnectDelay instead of terminating the watcher.
// ConfigMaps and Secrets are not listed initially, because seeding them would
// trigger a refresh of every application on startup.
func (w *ResourceWatcher) watchResource(resource schema.GroupVersionResource, namespace string) {
	for {
		listOptions := v1.ListOptions{}

		if resource.Resource == "applications" {
			list, err := w.client.Resource(resource).Namespace(namespace).List(w.ctx, listOptions)
			if err != nil {
				w.handleError(fmt.Errorf("failed to list %s in %s: %v", resource.Resource, namespace, err))
				return
			}
			w.enqueueList(resource, namespace, list)
			listOptions.ResourceVersion = list.GetResourceVersion()
		}

		wi, err := w.client.Resource(resource).Namespace(namespace).Watch(w.ctx, listOptions)
		if err != nil {
			w.handleError(fmt.Errorf("failed to watch %s in %s: %v", resource.Resource, namespace, err))
			return
		}

		channelClosed := false
		for !channelClosed {
			select {
			case <-w.stopCh:
				logger.Debugf("Stopping %s resource watcher in %s ...", resource.Resource, namespace)
				wi.Stop()
				return
			case event, ok := <-wi.ResultChan():
				if !ok {
					channelClosed = true
					break
				}
				w.handleEvent(event, resource, namespace)
			}
		}

		wi.Stop()

		if w.ctx.Err() != nil {
			return
		}

		logger.Debugf("Watch channel closed for %s in %s, reconnecting ...", resource.Resource, namespace)
		select {
		case <-w.stopCh:
			return
		case <-time.After(watcherReconnectDelay):
		}
	}
}

// enqueueList queues every object of a list as an ADDED event. It writes under
// eventMu into pendingEvents instead of calling the event handler directly,
// because the handler is invoked by the flush goroutine and must not race it.
func (w *ResourceWatcher) enqueueList(resource schema.GroupVersionResource, namespace string, list *unstructured.UnstructuredList) {
	w.eventMu.Lock()
	defer w.eventMu.Unlock()

	for i := range list.Items {
		obj := &list.Items[i]
		key := fmt.Sprintf("%s/%s/%s", resource.Resource, namespace, obj.GetName())
		w.pendingEvents[key] = Event{
			EventType: string(watch.Added),
			Obj:       obj,
			Resource:  resource,
			Namespace: namespace,
		}
	}
}

func (w *ResourceWatcher) handleError(err error) {
	select {
	case w.errorCh <- err:
	default:
		logger.Debugf("Error channel already has an error, ignoring new error: %v", err)
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

		key := fmt.Sprintf("%s/%s/%s", resource.Resource, namespace, unstructuredObj.GetName())

		w.eventMu.Lock()
		w.pendingEvents[key] = Event{
			EventType: string(event.Type),
			Obj:       unstructuredObj,
			Resource:  resource,
			Namespace: namespace,
		}
		w.eventMu.Unlock()

	case watch.Error:
		if status, ok := event.Object.(*v1.Status); ok {
			logger.Errorf("Error watching %s in %s: %v", resource.Resource, namespace, status.Message)
		} else {
			logger.Errorf("Error watching %s in %s: %v", resource.Resource, namespace, event.Object)
		}
	}
}
