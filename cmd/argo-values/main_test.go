package main

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
)

func parse(t *testing.T, args []string) (*CLI, *kong.Context) {
	t.Helper()

	commandLine := &CLI{}
	parser, err := New(commandLine)
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}

	ctx, err := parser.Parse(args)
	if err != nil {
		t.Fatalf("Parse(%q) error = %v, want nil", args, err)
	}

	return commandLine, ctx
}

func TestCommandDispatch(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "init", args: []string{"init"}, want: "init"},
		{name: "generate", args: []string{"generate"}, want: "generate"},
		{name: "discover", args: []string{"discover"}, want: "discover"},
		{name: "watch", args: []string{"watch"}, want: "watch"},
		{name: "no args shows help", args: []string{}, want: "help"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ctx := parse(t, tt.args)
			if got := ctx.Command(); got != tt.want {
				t.Fatalf("ctx.Command() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGlobalFlagsBeforeAndAfterSubcommand(t *testing.T) {
	t.Run("before subcommand", func(t *testing.T) {
		commandLine, _ := parse(t, []string{"-d", "init", "--value-maps", "cm"})

		if !commandLine.Global.DryRun {
			t.Error("DryRun = false, want true")
		}
		if got, want := commandLine.Init.ValueConfigs, []string{"cm"}; !reflect.DeepEqual(got, want) {
			t.Errorf("ValueConfigs = %#v, want %#v", got, want)
		}
	})

	t.Run("after subcommand", func(t *testing.T) {
		commandLine, _ := parse(t, []string{"init", "--value-maps", "cm", "-d", "--file-target", "/tmp"})

		if !commandLine.Global.DryRun {
			t.Error("DryRun = false, want true")
		}
		if got, want := commandLine.Global.TargetDirectory, "/tmp"; got != want {
			t.Errorf("TargetDirectory = %q, want %q", got, want)
		}
		if got, want := commandLine.Init.ValueConfigs, []string{"cm"}; !reflect.DeepEqual(got, want) {
			t.Errorf("ValueConfigs = %#v, want %#v", got, want)
		}
	})
}

func TestShortFlags(t *testing.T) {
	commandLine, _ := parse(t, []string{"-l", "info", "-k", "/kc", "-v", "v.yaml", "-t", "/tmp", "-d", "init"})

	tests := []struct {
		name string
		got  any
		want any
	}{
		{name: "log level", got: commandLine.Global.LogLevel, want: "info"},
		{name: "kubeconfig", got: commandLine.Global.KubeConfigPath, want: "/kc"},
		{name: "values file", got: commandLine.Global.ValuesFileName, want: "v.yaml"},
		{name: "target directory", got: commandLine.Global.TargetDirectory, want: "/tmp"},
		{name: "dry run", got: commandLine.Global.DryRun, want: true},
	}

	for _, tt := range tests {
		if !reflect.DeepEqual(tt.got, tt.want) {
			t.Errorf("%s = %#v, want %#v", tt.name, tt.got, tt.want)
		}
	}
}

func TestInitListFlagsDoNotSplitCommas(t *testing.T) {
	commandLine, _ := parse(t, []string{
		"init",
		"--value-maps", "a,b",
		"--value-maps", "c",
		"--env-maps", "d,e",
		"--value-secrets", "f,g",
		"--env-secrets", "h,i",
	})

	tests := []struct {
		name string
		got  []string
		want []string
	}{
		{name: "value-maps", got: commandLine.Init.ValueConfigs, want: []string{"a,b", "c"}},
		{name: "env-maps", got: commandLine.Init.EnvConfigs, want: []string{"d,e"}},
		{name: "value-secrets", got: commandLine.Init.ValueSecrets, want: []string{"f,g"}},
		{name: "env-secrets", got: commandLine.Init.EnvSecrets, want: []string{"h,i"}},
	}

	for _, tt := range tests {
		if !reflect.DeepEqual(tt.got, tt.want) {
			t.Errorf("%s = %#v, want %#v", tt.name, tt.got, tt.want)
		}
	}
}

func TestEnvDefaults(t *testing.T) {
	t.Setenv("LOG_LEVEL", "warn")
	t.Setenv("LOG_FORMAT", "text")
	t.Setenv("ARGOCD_APP_NAME", "my-app")
	t.Setenv("ARGOCD_APP_NAMESPACE", "ns")

	commandLine, _ := parse(t, []string{"generate"})

	if got, want := commandLine.Global.LogLevel, "warn"; got != want {
		t.Errorf("LogLevel = %q, want %q", got, want)
	}
	if got, want := commandLine.Global.LogFormat, "text"; got != want {
		t.Errorf("LogFormat = %q, want %q", got, want)
	}
	if got, want := commandLine.Generate.AppName, "my-app"; got != want {
		t.Errorf("AppName = %q, want %q", got, want)
	}
	if got, want := commandLine.Generate.AppNamespace, "ns"; got != want {
		t.Errorf("AppNamespace = %q, want %q", got, want)
	}
}

func TestWatchEnvDefaults(t *testing.T) {
	t.Setenv("ARGOCD_VALUES_REFRESH_SECONDS", "5")
	t.Setenv("ARGOCD_VALUES_NAMESPACES", "a,b")

	commandLine, _ := parse(t, []string{"watch"})

	if got, want := commandLine.Watch.RefreshInterval, 5; got != want {
		t.Errorf("RefreshInterval = %d, want %d", got, want)
	}
	// Kong splits the env value on commas without trimming whitespace.
	if got, want := commandLine.Watch.Namespaces, []string{"a", "b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Namespaces = %#v, want %#v", got, want)
	}
}

func TestWatchRefreshIntervalFlag(t *testing.T) {
	commandLine, _ := parse(t, []string{"watch", "--refresh-interval", "7"})

	if got, want := commandLine.Watch.RefreshInterval, 7; got != want {
		t.Errorf("RefreshInterval = %d, want %d", got, want)
	}
}

func TestWatchEnvInvalidIntervalFails(t *testing.T) {
	t.Setenv("ARGOCD_VALUES_REFRESH_SECONDS", "abc")

	parser, err := New(&CLI{})
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}

	if _, err := parser.Parse([]string{"watch"}); err == nil {
		t.Fatal("Parse() error = nil, want error for invalid refresh interval")
	}
}

func TestWatchNamespacesOverrideAndCommaSplitting(t *testing.T) {
	t.Setenv("ARGOCD_VALUES_NAMESPACES", "env-ns")

	commandLine, _ := parse(t, []string{"watch", "--namespaces", "x", "--namespaces", "y,z"})

	if got, want := commandLine.Watch.Namespaces, []string{"x", "y", "z"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Namespaces = %#v, want %#v", got, want)
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "unknown flag", args: []string{"init", "--bogus"}},
		{name: "missing flag value", args: []string{"init", "--value-maps"}},
		{name: "unknown command", args: []string{"foo"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser, err := New(&CLI{})
			if err != nil {
				t.Fatalf("New() error = %v, want nil", err)
			}

			_, err = parser.Parse(tt.args)
			if err == nil {
				t.Fatalf("Parse(%q) error = nil, want error", tt.args)
			}

			var parseErr *kong.ParseError
			if !errors.As(err, &parseErr) {
				t.Fatalf("Parse(%q) error = %T, want *kong.ParseError", tt.args, err)
			}
			if got, want := parseErr.ExitCode(), 80; got != want {
				t.Errorf("ExitCode() = %d, want %d", got, want)
			}
		})
	}
}

func TestDefaultHelp(t *testing.T) {
	commandLine := &CLI{}
	parser, err := New(commandLine)
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}

	var buf bytes.Buffer
	parser.Stdout = &buf

	ctx, err := parser.Parse([]string{})
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}
	if got, want := ctx.Command(), "help"; got != want {
		t.Fatalf("ctx.Command() = %q, want %q", got, want)
	}

	if err := ctx.Run(&commandLine.Global); err != nil {
		t.Fatalf("ctx.Run() error = %v, want nil", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Usage: argo-values") {
		t.Errorf("help output does not contain %q:\n%s", "Usage: argo-values", output)
	}
	if !strings.Contains(output, "Commands:") {
		t.Errorf("help output does not contain %q:\n%s", "Commands:", output)
	}
	if strings.Contains(output, "argo-values help") {
		t.Errorf("help output unexpectedly mentions the internal help command:\n%s", output)
	}
}

func TestExpectedLongFlagsAccepted(t *testing.T) {
	commandLine, _ := parse(t, []string{
		"--log-level", "info",
		"--log-format", "text",
		"--log-output", "stdout",
		"--kubeconfig", "/kc",
		"--file-values", "v.yaml",
		"--file-target", "/tmp",
		"--dry-run",
		"init",
		"--value-maps", "cm",
		"--env-maps", "env",
		"--value-secrets", "vs",
		"--env-secrets", "es",
	})

	if got, want := commandLine.Global.LogLevel, "info"; got != want {
		t.Errorf("LogLevel = %q, want %q", got, want)
	}
	if got, want := commandLine.Global.LogFormat, "text"; got != want {
		t.Errorf("LogFormat = %q, want %q", got, want)
	}
	if got, want := commandLine.Global.LogOutput, "stdout"; got != want {
		t.Errorf("LogOutput = %q, want %q", got, want)
	}
	if got, want := commandLine.Global.KubeConfigPath, "/kc"; got != want {
		t.Errorf("KubeConfigPath = %q, want %q", got, want)
	}
	if got, want := commandLine.Global.ValuesFileName, "v.yaml"; got != want {
		t.Errorf("ValuesFileName = %q, want %q", got, want)
	}
	if got, want := commandLine.Global.TargetDirectory, "/tmp"; got != want {
		t.Errorf("TargetDirectory = %q, want %q", got, want)
	}
	if !commandLine.Global.DryRun {
		t.Error("DryRun = false, want true")
	}
	if got, want := commandLine.Init.ValueConfigs, []string{"cm"}; !reflect.DeepEqual(got, want) {
		t.Errorf("ValueConfigs = %#v, want %#v", got, want)
	}
	if got, want := commandLine.Init.EnvConfigs, []string{"env"}; !reflect.DeepEqual(got, want) {
		t.Errorf("EnvConfigs = %#v, want %#v", got, want)
	}
	if got, want := commandLine.Init.ValueSecrets, []string{"vs"}; !reflect.DeepEqual(got, want) {
		t.Errorf("ValueSecrets = %#v, want %#v", got, want)
	}
	if got, want := commandLine.Init.EnvSecrets, []string{"es"}; !reflect.DeepEqual(got, want) {
		t.Errorf("EnvSecrets = %#v, want %#v", got, want)
	}
}

func TestGenerateFlags(t *testing.T) {
	commandLine, _ := parse(t, []string{"generate", "--app-name", "app", "--app-namespace", "argocd"})

	if got, want := commandLine.Generate.AppName, "app"; got != want {
		t.Errorf("AppName = %q, want %q", got, want)
	}
	if got, want := commandLine.Generate.AppNamespace, "argocd"; got != want {
		t.Errorf("AppNamespace = %q, want %q", got, want)
	}
}
