package idle

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadIdleTimeoutOverrideMissingFile(t *testing.T) {
	minutes, err := LoadIdleTimeoutOverride(filepath.Join(t.TempDir(), "x.json"))
	if err != nil || minutes != 0 {
		t.Fatalf("got %d, %v", minutes, err)
	}
}

func TestIdleTimeoutOverrideRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "x.json")
	if err := SaveIdleTimeoutOverride(path, 30); err != nil {
		t.Fatal(err)
	}
	minutes, err := LoadIdleTimeoutOverride(path)
	if err != nil || minutes != 30 {
		t.Fatalf("got %d, %v", minutes, err)
	}
}

func TestIdleTimeoutOverrideRejectsOutOfRange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.json")
	for _, minutes := range []int{0, -1, MaxIdleTimeoutMinutes + 1} {
		if err := SaveIdleTimeoutOverride(path, minutes); err == nil {
			t.Fatalf("expected error for %d", minutes)
		}
	}
	if err := os.WriteFile(path, []byte(`{"idle_timeout_minutes": 5000}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadIdleTimeoutOverride(path); err == nil {
		t.Fatal("expected error loading out-of-range file")
	}
}

func TestApplyIdleTimeoutOverride(t *testing.T) {
	config := Config{Servers: []Server{
		{Instance: "a", IdleTimeout: 15 * time.Minute},
		{Instance: "b", IdleTimeout: 20 * time.Minute},
	}}
	got := ApplyIdleTimeoutOverride(config, 7)
	for _, server := range got.Servers {
		if server.IdleTimeout != 7*time.Minute {
			t.Fatalf("%s: %v", server.Instance, server.IdleTimeout)
		}
	}
	if config.Servers[0].IdleTimeout != 15*time.Minute {
		t.Fatal("original mutated")
	}
	same := ApplyIdleTimeoutOverride(config, 0)
	if same.Servers[1].IdleTimeout != 20*time.Minute {
		t.Fatal("zero should be a no-op")
	}
}
