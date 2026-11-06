package kubernetes

import "testing"

func TestParseNamespacedName(t *testing.T) {
	tests := []struct {
		name             string
		defaultNamespace string
		wantNamespace    string
		wantResource     string
	}{
		{name: "ns/cm", defaultNamespace: "argocd", wantNamespace: "ns", wantResource: "cm"},
		{name: "cm", defaultNamespace: "argocd", wantNamespace: "argocd", wantResource: "cm"},
		{name: "cm", defaultNamespace: "", wantNamespace: "default", wantResource: "cm"},
		{name: "/cm", defaultNamespace: "argocd", wantNamespace: "argocd", wantResource: "/cm"},
		{name: "ns/", defaultNamespace: "argocd", wantNamespace: "argocd", wantResource: "ns/"},
		{name: "a/b/c", defaultNamespace: "argocd", wantNamespace: "a", wantResource: "b/c"},
	}

	for _, tt := range tests {
		t.Run(tt.name+"/"+tt.defaultNamespace, func(t *testing.T) {
			namespace, resource := parseNamespacedName(tt.name, tt.defaultNamespace)
			if namespace != tt.wantNamespace || resource != tt.wantResource {
				t.Fatalf("parseNamespacedName(%q, %q) = (%q, %q), want (%q, %q)",
					tt.name, tt.defaultNamespace, namespace, resource, tt.wantNamespace, tt.wantResource)
			}
		})
	}
}
