package kubernetes

import (
	"argo-values/internal/logger"
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	refreshTimeout     = 10 * time.Second
	refreshAttempts    = 3
	refreshBaseBackoff = 100 * time.Millisecond
	refreshMaxBackoff  = time.Second
)

// Client wraps Kubernetes client functionality
type Client struct {
	clientSet kubernetes.Interface
	client    dynamic.Interface
}

// NewClient creates a new Kubernetes client
func NewClient(kubeconfig string) (*Client, error) {
	var config *rest.Config
	var err error

	if kubeconfig == "" {
		config, err = rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to create In-Cluster Kubernetes config: %w", err)
		}
	} else {
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create Kubernetes config: %w", err)
		}
	}

	client, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	clientSet, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	return &Client{
		clientSet: clientSet,
		client:    client,
	}, nil
}

func (k *Client) Resource(resource schema.GroupVersionResource) dynamic.NamespaceableResourceInterface {
	return k.client.Resource(resource)
}

func (k *Client) RefreshApplication(name, namespace string) error {
	appResource := schema.GroupVersionResource{
		Group:    "argoproj.io",
		Version:  "v1alpha1",
		Resource: "applications",
	}

	patchData := []byte(`{"metadata": {"annotations": {"argocd.argoproj.io/refresh": "hard"}}}`)

	ctx, cancel := context.WithTimeout(context.Background(), refreshTimeout)
	defer cancel()

	var lastErr error
	for attempt := 0; attempt < refreshAttempts; attempt++ {
		_, err := k.client.Resource(appResource).Namespace(namespace).Patch(
			ctx,
			name,
			types.MergePatchType,
			patchData,
			metav1.PatchOptions{},
		)
		if err == nil {
			return nil
		}
		lastErr = err
		if !isRetryableRefreshError(err) {
			return err
		}
		if attempt == refreshAttempts-1 {
			break
		}
		delay := refreshBaseBackoff << attempt
		if delay > refreshMaxBackoff {
			delay = refreshMaxBackoff
		}
		logger.Debugf("Retrying refresh for application %s/%s after %v (attempt %d/%d)", namespace, name, err, attempt+1, refreshAttempts)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
	return lastErr
}

// isRetryableRefreshError reports whether a failed refresh patch is worth
// retrying. Client errors (bad request, not found, auth issues) are permanent;
// transient server, throttling, conflict and network/timeout errors are retried.
func isRetryableRefreshError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	switch {
	case apierrors.IsNotFound(err),
		apierrors.IsForbidden(err),
		apierrors.IsUnauthorized(err),
		apierrors.IsBadRequest(err),
		apierrors.IsInvalid(err):
		return false
	}
	return apierrors.IsTooManyRequests(err) ||
		apierrors.IsServerTimeout(err) ||
		apierrors.IsServiceUnavailable(err) ||
		apierrors.IsInternalError(err) ||
		apierrors.IsTimeout(err) ||
		apierrors.IsConflict(err)
}

// parseNamespacedName splits a "namespace/name" reference. Unqualified names
// (including "/name" and "namespace/") use defaultNamespace, falling back to
// "default" when it is empty. Only the first "/" separates both parts.
func parseNamespacedName(name, defaultNamespace string) (namespace, resource string) {
	if defaultNamespace == "" {
		defaultNamespace = "default"
	}

	separator := strings.Index(name, "/")
	if separator <= 0 || separator == len(name)-1 {
		return defaultNamespace, name
	}

	return name[:separator], name[separator+1:]
}

func (k *Client) GetConfigmap(name, defaultNamespace string) (*v1.ConfigMap, error) {
	namespace, resource := parseNamespacedName(name, defaultNamespace)
	logger.Debugf("Get ConfigMap %s from namespace %s", resource, namespace)
	return k.clientSet.CoreV1().ConfigMaps(namespace).Get(context.Background(), resource, metav1.GetOptions{})
}

func (k *Client) GetSecret(name, defaultNamespace string) (*v1.Secret, error) {
	namespace, resource := parseNamespacedName(name, defaultNamespace)
	logger.Debugf("Get Secret %s from namespace %s", resource, namespace)
	return k.clientSet.CoreV1().Secrets(namespace).Get(context.Background(), resource, metav1.GetOptions{})
}
