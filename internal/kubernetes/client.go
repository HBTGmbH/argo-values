package kubernetes

import (
	"argo-values/internal/logger"
	"context"
	"fmt"
	"strings"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Client wraps Kubernetes client functionality
type Client struct {
	clientSet *kubernetes.Clientset
	client    *dynamic.DynamicClient
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

	_, err := k.client.Resource(appResource).Namespace(namespace).Patch(
		context.TODO(),
		name,
		types.MergePatchType,
		patchData,
		metav1.PatchOptions{},
	)

	return err
}

func (k *Client) GetConfigmap(name string) (*v1.ConfigMap, error) {
	nameParts := strings.Split(name, "/")
	if len(nameParts) > 1 {
		logger.Debugf("Get ConfigMap %s from namespace %s", nameParts[1], nameParts[0])
		return k.clientSet.CoreV1().ConfigMaps(nameParts[0]).Get(context.Background(), nameParts[1], metav1.GetOptions{})
	}
	logger.Debugf("Get ConfigMap %s from namespace default", name)
	return k.clientSet.CoreV1().ConfigMaps("default").Get(context.Background(), name, metav1.GetOptions{})
}

func (k *Client) GetSecret(name string) (*v1.Secret, error) {
	nameParts := strings.Split(name, "/")
	if len(nameParts) > 1 {
		logger.Debugf("Get Secret %s from namespace %s", nameParts[1], nameParts[0])
		return k.clientSet.CoreV1().Secrets(nameParts[0]).Get(context.Background(), nameParts[1], metav1.GetOptions{})
	}
	logger.Debugf("Get Secret %s from namespace default", name)
	return k.clientSet.CoreV1().Secrets("default").Get(context.Background(), name, metav1.GetOptions{})
}
