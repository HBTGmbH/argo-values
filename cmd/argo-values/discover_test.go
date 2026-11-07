package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestWriteDiscoveryResult(t *testing.T) {
	t.Run("supported writes token", func(t *testing.T) {
		var out bytes.Buffer
		writeDiscoveryResult(&out, true)
		if got, want := out.String(), "argo-values\n"; got != want {
			t.Fatalf("writeDiscoveryResult(true) = %q, want %q", got, want)
		}
	})

	t.Run("unsupported writes nothing", func(t *testing.T) {
		var out bytes.Buffer
		writeDiscoveryResult(&out, false)
		if got := out.String(); got != "" {
			t.Fatalf("writeDiscoveryResult(false) = %q, want empty", got)
		}
	})
}

func TestDiscoverApplication(t *testing.T) {
	t.Run("parameter present is supported", func(t *testing.T) {
		t.Setenv("ARGOCD_APP_PARAMETERS", `[{"name":"valueConfigs","array":["cm"]}]`)

		supported, err := discoverApplication()
		if err != nil {
			t.Fatalf("discoverApplication() error = %v, want nil", err)
		}
		if !supported {
			t.Fatal("discoverApplication() = false, want true")
		}
	})

	t.Run("empty parameters is unsupported", func(t *testing.T) {
		for _, env := range os.Environ() {
			if strings.HasPrefix(env, "ARGOCD_ENV_") {
				t.Skipf("test environment contains %q, which would make the application supported", strings.SplitN(env, "=", 2)[0])
			}
		}
		t.Setenv("ARGOCD_APP_PARAMETERS", `[]`)

		supported, err := discoverApplication()
		if err != nil {
			t.Fatalf("discoverApplication() error = %v, want nil", err)
		}
		if supported {
			t.Fatal("discoverApplication() = true, want false")
		}
	})

	t.Run("broken parameters returns error", func(t *testing.T) {
		t.Setenv("ARGOCD_APP_PARAMETERS", `{broken`)

		if _, err := discoverApplication(); err == nil {
			t.Fatal("discoverApplication() error = nil, want error")
		}
	})
}
