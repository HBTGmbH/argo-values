package kubernetes

import (
	"context"
	"errors"
	"net"
	"testing"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

var testApplicationResource = schema.GroupVersionResource{
	Group:    "argoproj.io",
	Version:  "v1alpha1",
	Resource: "applications",
}

func TestParseResourceReference(t *testing.T) {
	tests := []struct {
		name             string
		reference        string
		defaultNamespace string
		wantNamespace    string
		wantName         string
		wantErr          bool
	}{
		{name: "unqualified", reference: "cm", defaultNamespace: "argocd", wantNamespace: "argocd", wantName: "cm"},
		{name: "qualified", reference: "ns/cm", defaultNamespace: "argocd", wantNamespace: "ns", wantName: "cm"},
		{name: "unqualified without default namespace", reference: "cm", defaultNamespace: "", wantNamespace: "default", wantName: "cm"},
		{name: "whitespace around parts", reference: " ns / cm ", defaultNamespace: "argocd", wantNamespace: "ns", wantName: "cm"},
		{name: "empty namespace", reference: "/cm", defaultNamespace: "argocd", wantErr: true},
		{name: "empty name", reference: "ns/", defaultNamespace: "argocd", wantErr: true},
		{name: "too many parts", reference: "a/b/c", defaultNamespace: "argocd", wantErr: true},
		{name: "empty reference", reference: "", defaultNamespace: "argocd", wantErr: true},
		{name: "whitespace only", reference: " / ", defaultNamespace: "argocd", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			namespace, name, err := ParseResourceReference(tt.reference, tt.defaultNamespace)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseResourceReference(%q, %q) = (%q, %q), want error",
						tt.reference, tt.defaultNamespace, namespace, name)
				}
				if !errors.Is(err, ErrInvalidResourceReference) {
					t.Fatalf("expected ErrInvalidResourceReference, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseResourceReference(%q, %q) returned unexpected error: %v",
					tt.reference, tt.defaultNamespace, err)
			}
			if namespace != tt.wantNamespace || name != tt.wantName {
				t.Fatalf("ParseResourceReference(%q, %q) = (%q, %q), want (%q, %q)",
					tt.reference, tt.defaultNamespace, namespace, name, tt.wantNamespace, tt.wantName)
			}
		})
	}
}

func TestGetConfigmapRejectsInvalidReference(t *testing.T) {
	client := &Client{clientSet: fake.NewSimpleClientset()}

	if _, err := client.GetConfigmap("a/b/c", "default"); err == nil {
		t.Fatal("expected GetConfigmap to reject an invalid reference")
	}
}

func TestGetSecretRejectsInvalidReference(t *testing.T) {
	client := &Client{clientSet: fake.NewSimpleClientset()}

	if _, err := client.GetSecret("a/b/c", "default"); err == nil {
		t.Fatal("expected GetSecret to reject an invalid reference")
	}
}

func TestGetConfigmapUsesQualifiedReference(t *testing.T) {
	client := &Client{clientSet: fake.NewSimpleClientset(&v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: "ns"},
	})}

	configMap, err := client.GetConfigmap("ns/cm", "default")
	if err != nil {
		t.Fatalf("GetConfigmap returned unexpected error: %v", err)
	}
	if configMap.Name != "cm" || configMap.Namespace != "ns" {
		t.Fatalf("GetConfigmap returned %s/%s, want ns/cm", configMap.Namespace, configMap.Name)
	}
}

func TestIsRetryableRefreshError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "too many requests", err: apierrors.NewTooManyRequests("", 0), want: true},
		{name: "internal error", err: apierrors.NewInternalError(errors.New("boom")), want: true},
		{name: "service unavailable", err: apierrors.NewServiceUnavailable("unavailable"), want: true},
		{name: "server timeout", err: apierrors.NewServerTimeout(schema.GroupResource{Group: "argoproj.io", Resource: "applications"}, "patch", 1), want: true},
		{name: "timeout", err: apierrors.NewTimeoutError("timeout", 1), want: true},
		{name: "conflict", err: apierrors.NewConflict(schema.GroupResource{Group: "argoproj.io", Resource: "applications"}, "app", errors.New("conflict")), want: true},
		{name: "not found", err: apierrors.NewNotFound(schema.GroupResource{Group: "argoproj.io", Resource: "applications"}, "app"), want: false},
		{name: "forbidden", err: apierrors.NewForbidden(schema.GroupResource{Group: "argoproj.io", Resource: "applications"}, "app", errors.New("forbidden")), want: false},
		{name: "unauthorized", err: apierrors.NewUnauthorized("unauthorized"), want: false},
		{name: "bad request", err: apierrors.NewBadRequest("bad request"), want: false},
		{name: "invalid", err: apierrors.NewInvalid(schema.GroupKind{Group: "argoproj.io", Kind: "Application"}, "app", nil), want: false},
		{name: "context deadline exceeded", err: context.DeadlineExceeded, want: true},
		{name: "wrapped context deadline exceeded", err: errors.Join(errors.New("patch failed"), context.DeadlineExceeded), want: true},
		{name: "net timeout", err: &net.DNSError{IsTimeout: true}, want: true},
		{name: "plain error", err: errors.New("boom"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isRetryableRefreshError(tt.err); got != tt.want {
				t.Fatalf("isRetryableRefreshError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestRefreshApplicationRetriesTransientErrors(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			testApplicationResource: "ApplicationList",
		},
	)

	calls := 0
	client.PrependReactor("patch", "applications", func(action k8stesting.Action) (bool, runtime.Object, error) {
		calls++
		if calls < 3 {
			return true, nil, apierrors.NewTooManyRequests("", 0)
		}
		return true, &unstructured.Unstructured{}, nil
	})

	kubeClient := &Client{client: client}
	if err := kubeClient.RefreshApplication("app", "argocd"); err != nil {
		t.Fatalf("expected refresh to succeed after retries, got %v", err)
	}
	if calls != 3 {
		t.Fatalf("expected 3 patch attempts, got %d", calls)
	}
}

func TestRefreshApplicationDoesNotRetryNotFound(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			testApplicationResource: "ApplicationList",
		},
	)

	calls := 0
	client.PrependReactor("patch", "applications", func(action k8stesting.Action) (bool, runtime.Object, error) {
		calls++
		return true, nil, apierrors.NewNotFound(schema.GroupResource{Group: "argoproj.io", Resource: "applications"}, "app")
	})

	kubeClient := &Client{client: client}
	if err := kubeClient.RefreshApplication("app", "argocd"); err == nil {
		t.Fatal("expected refresh to fail for a not-found application")
	}
	if calls != 1 {
		t.Fatalf("expected exactly 1 patch attempt, got %d", calls)
	}
}
