package logger

import (
	"os"
	"testing"

	"github.com/sirupsen/logrus"
)

// preserveLogState restores the global logger configuration after a test.
func preserveLogState(t *testing.T) {
	t.Helper()
	originalOut := Log.Out
	originalLevel := Log.Level
	originalFormatter := Log.Formatter
	t.Cleanup(func() {
		Log.Out = originalOut
		Log.Level = originalLevel
		Log.Formatter = originalFormatter
	})
}

func TestInitRoutesOutputToStderr(t *testing.T) {
	preserveLogState(t)

	Init("debug", "json", "stderr")

	if Log.Out != os.Stderr {
		t.Fatalf("expected Log.Out to be os.Stderr, got %T", Log.Out)
	}
	if Log.Level != logrus.DebugLevel {
		t.Fatalf("expected debug level, got %v", Log.Level)
	}
	if _, ok := Log.Formatter.(*logrus.JSONFormatter); !ok {
		t.Fatalf("expected JSON formatter, got %T", Log.Formatter)
	}
}

func TestInitRoutesOutputToStdout(t *testing.T) {
	preserveLogState(t)

	Init("info", "text", "stdout")

	if Log.Out != os.Stdout {
		t.Fatalf("expected Log.Out to be os.Stdout, got %T", Log.Out)
	}
	if Log.Level != logrus.InfoLevel {
		t.Fatalf("expected info level, got %v", Log.Level)
	}
	if _, ok := Log.Formatter.(*logrus.TextFormatter); !ok {
		t.Fatalf("expected text formatter, got %T", Log.Formatter)
	}
}
