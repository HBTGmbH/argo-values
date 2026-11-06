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
		wantValuesPath  string
	}{
		{
			name:            "current directory",
			appName:         "app",
			appNamespace:    "argocd",
			targetDirectory: ".",
			valuesFile:      "app-values.yaml",
			wantValuesPath:  "app-values.yaml",
		},
		{
			name:            "relative chart directory",
			appName:         "app",
			appNamespace:    "argocd",
			targetDirectory: "./chart",
			valuesFile:      "app-values.yaml",
			wantValuesPath:  "chart/app-values.yaml",
		},
		{
			name:            "absolute chart directory",
			appName:         "app",
			appNamespace:    "argocd",
			targetDirectory: "/abs/chart",
			valuesFile:      "values.yaml",
			wantValuesPath:  "/abs/chart/values.yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := helmTemplateArgs(tt.appName, tt.appNamespace, tt.targetDirectory, tt.valuesFile)
			want := []string{
				"template",
				tt.appName,
				tt.targetDirectory,
				"--namespace",
				tt.appNamespace,
				"--include-crds",
				"-f",
				tt.wantValuesPath,
			}

			if !reflect.DeepEqual(got, want) {
				t.Fatalf("helmTemplateArgs() = %#v, want %#v", got, want)
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
