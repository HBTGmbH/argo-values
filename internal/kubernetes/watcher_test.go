package kubernetes

import (
	"strings"
	"testing"
	"time"
)

func TestNewResourceWatcherRejectsNonPositiveInterval(t *testing.T) {
	tests := []struct {
		name     string
		interval time.Duration
	}{
		{name: "zero", interval: 0},
		{name: "negative", interval: -1 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			watcher, err := NewResourceWatcher(nil, tt.interval, nil, nil)
			if err == nil {
				t.Fatalf("NewResourceWatcher(%v) returned no error, want error", tt.interval)
			}
			if watcher != nil {
				t.Errorf("NewResourceWatcher(%v) returned watcher %v, want nil", tt.interval, watcher)
			}
			if !strings.Contains(err.Error(), "refresh interval") {
				t.Errorf("error message %q does not contain %q", err.Error(), "refresh interval")
			}
		})
	}
}

func TestNewResourceWatcherAcceptsPositiveInterval(t *testing.T) {
	watcher, err := NewResourceWatcher(nil, time.Second, nil, nil)
	if err != nil {
		t.Fatalf("NewResourceWatcher(time.Second) returned error: %v", err)
	}
	if watcher == nil {
		t.Fatal("NewResourceWatcher(time.Second) returned nil watcher, want non-nil")
	}
}
