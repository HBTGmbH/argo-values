package config

import (
	"os"
	"strings"
	"testing"
)

func TestChartParameterNamesMatchCode(t *testing.T) {
	chartPath := "../../charts/argo-values/templates/configmap.yaml"

	content, err := os.ReadFile(chartPath)
	if err != nil {
		t.Fatalf("failed to read chart file %q: %v", chartPath, err)
	}

	chart := string(content)

	for _, parameter := range []string{"valueConfigs", "envConfigs", "valueSecrets", "envSecrets", "helmValues"} {
		expected := "- name: " + parameter
		if !strings.Contains(chart, expected) {
			t.Fatalf("chart file %q does not contain parameter %q (expected %q)", chartPath, parameter, expected)
		}
	}

	if strings.Contains(chart, "valuesConfigs") {
		t.Fatalf("chart file %q still contains obsolete parameter name %q", chartPath, "valuesConfigs")
	}
}
