package commands

import (
	"reflect"
	"testing"
)

func TestHelmTemplateArgs(t *testing.T) {
	tests := []struct {
		name            string
		appName         string
		appNamespace    string
		targetDirectory string
		valuesFile      string
		kubeVersion     string
		apiVersions     []string
		wantValuesPath  string
		want            []string
	}{
		{
			name:            "current directory",
			appName:         "app",
			appNamespace:    "argocd",
			targetDirectory: ".",
			valuesFile:      "app-values.yaml",
			wantValuesPath:  "app-values.yaml",
			want: []string{
				"template", "app", ".",
				"--namespace", "argocd",
				"--include-crds",
				"--dependency-update",
				"-f", "app-values.yaml",
			},
		},
		{
			name:            "relative chart directory",
			appName:         "app",
			appNamespace:    "argocd",
			targetDirectory: "./chart",
			valuesFile:      "app-values.yaml",
			wantValuesPath:  "chart/app-values.yaml",
			want: []string{
				"template", "app", "./chart",
				"--namespace", "argocd",
				"--include-crds",
				"--dependency-update",
				"-f", "chart/app-values.yaml",
			},
		},
		{
			name:            "absolute chart directory",
			appName:         "app",
			appNamespace:    "argocd",
			targetDirectory: "/abs/chart",
			valuesFile:      "values.yaml",
			wantValuesPath:  "/abs/chart/values.yaml",
			want: []string{
				"template", "app", "/abs/chart",
				"--namespace", "argocd",
				"--include-crds",
				"--dependency-update",
				"-f", "/abs/chart/values.yaml",
			},
		},
		{
			name:            "without kube version and api versions",
			appName:         "app",
			appNamespace:    "argocd",
			targetDirectory: ".",
			valuesFile:      "values.yaml",
			wantValuesPath:  "values.yaml",
			want: []string{
				"template", "app", ".",
				"--namespace", "argocd",
				"--include-crds",
				"--dependency-update",
				"-f", "values.yaml",
			},
		},
		{
			name:            "with kube version and api versions",
			appName:         "app",
			appNamespace:    "argocd",
			targetDirectory: ".",
			valuesFile:      "values.yaml",
			kubeVersion:     "1.29.0",
			apiVersions:     []string{"cert-manager.io/v1", "apps/v1"},
			wantValuesPath:  "values.yaml",
			want: []string{
				"template", "app", ".",
				"--namespace", "argocd",
				"--include-crds",
				"--dependency-update",
				"--kube-version", "1.29.0",
				"--api-versions", "cert-manager.io/v1",
				"--api-versions", "apps/v1",
				"-f", "values.yaml",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := helmTemplateArgs(tt.appName, tt.appNamespace, tt.targetDirectory, tt.valuesFile, tt.kubeVersion, tt.apiVersions)

			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("helmTemplateArgs() = %#v, want %#v", got, tt.want)
			}

			if len(got) >= 2 && got[len(got)-2] != "-f" {
				t.Fatalf("expected -f before values path, got %#v", got)
			}
			if got[len(got)-1] != tt.wantValuesPath {
				t.Fatalf("values path = %q, want %q", got[len(got)-1], tt.wantValuesPath)
			}
		})
	}
}

func TestNormalizeKubeVersion(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    string
	}{
		{name: "plain version", version: "1.29.0", want: "1.29.0"},
		{name: "leading v", version: "v1.29.0", want: "1.29.0"},
		{name: "build metadata", version: "1.29.0+", want: "1.29.0"},
		{name: "distribution suffix", version: "1.29.0-eks-1234", want: "1.29.0"},
		{name: "empty", version: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeKubeVersion(tt.version); got != tt.want {
				t.Errorf("normalizeKubeVersion(%q) = %q, want %q", tt.version, got, tt.want)
			}
		})
	}
}
