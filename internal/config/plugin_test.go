package config

import (
	"os"
	"testing"
)

func TestHasEnvParameter(t *testing.T) {
	t.Run("present with non-empty value", func(t *testing.T) {
		t.Setenv("ARGO_VALUES_TEST_PRESENT", "value")
		if !HasEnvParameter("ARGO_VALUES_TEST_PRESENT") {
			t.Error("expected parameter to be found")
		}
	})

	t.Run("present with empty value", func(t *testing.T) {
		t.Setenv("ARGO_VALUES_TEST_EMPTY", "")
		if !HasEnvParameter("ARGO_VALUES_TEST_EMPTY") {
			t.Error("expected empty parameter to count as present")
		}
	})

	t.Run("dash and underscore are equivalent", func(t *testing.T) {
		t.Setenv("ARGO_VALUES_TEST-DASH", "x")
		if !HasEnvParameter("ARGO_VALUES_TEST-DASH") {
			t.Error("expected dash spelling to be found")
		}
		if !HasEnvParameter("ARGO_VALUES_TEST_DASH") {
			t.Error("expected underscore spelling to be found")
		}
	})

	t.Run("case insensitive", func(t *testing.T) {
		t.Setenv("argo_values_test_lower", "x")
		if !HasEnvParameter("ARGO_VALUES_TEST_LOWER") {
			t.Error("expected lower-case parameter to be found")
		}
	})

	t.Run("not set", func(t *testing.T) {
		const name = "ARGO_VALUES_TEST_DEFINITELY_NOT_SET_98765"
		if _, exists := os.LookupEnv(name); exists {
			t.Skipf("%s is already set in the test environment", name)
		}
		if HasEnvParameter(name) {
			t.Error("expected parameter to be absent")
		}
	})
}

func TestHasEnvParameters(t *testing.T) {
	names := []string{
		"ARGOCD_ENV_VALUE_CONFIGS",
		"ARGOCD_ENV_ENV_CONFIGS",
		"ARGOCD_ENV_VALUE_SECRETS",
		"ARGOCD_ENV_ENV_SECRETS",
	}
	for _, name := range names {
		if _, exists := os.LookupEnv(name); exists {
			t.Skipf("%s is already set in the test environment", name)
		}
	}

	if HasEnvParameters() {
		t.Error("expected no env parameters to be present")
	}

	t.Setenv("ARGOCD_ENV_ENV_SECRETS", "x")
	if !HasEnvParameters() {
		t.Error("expected env parameters to be present")
	}
}

func TestGetStringFromEnv(t *testing.T) {
	t.Run("present", func(t *testing.T) {
		t.Setenv("ARGO_VALUES_TEST_GET", "value")
		if got := GetStringFromEnv("ARGO_VALUES_TEST_GET", "default"); got != "value" {
			t.Errorf("expected %q, got %q", "value", got)
		}
	})

	t.Run("default fallback", func(t *testing.T) {
		const name = "ARGO_VALUES_TEST_GET_DEFINITELY_NOT_SET_98765"
		if _, exists := os.LookupEnv(name); exists {
			t.Skipf("%s is already set in the test environment", name)
		}
		if got := GetStringFromEnv(name, "default"); got != "default" {
			t.Errorf("expected %q, got %q", "default", got)
		}
	})

	t.Run("case and dash matching", func(t *testing.T) {
		t.Setenv("argo_values_test_get-dash", "value")
		if got := GetStringFromEnv("ARGO_VALUES_TEST_GET_DASH", "default"); got != "value" {
			t.Errorf("expected %q, got %q", "value", got)
		}
	})
}
