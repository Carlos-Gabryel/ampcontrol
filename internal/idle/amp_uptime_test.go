package idle

import (
	"testing"
	"time"
)

func TestParseAMPApplicationUptime(t *testing.T) {
	tests := map[string]time.Duration{
		"0:03:24:12": 3*time.Hour + 24*time.Minute + 12*time.Second,
		"03:24:12":   3*time.Hour + 24*time.Minute + 12*time.Second,
		"2.03:04:05": 2*24*time.Hour + 3*time.Hour + 4*time.Minute + 5*time.Second,
		"00:00:05.9": 5 * time.Second,
	}

	for raw, expected := range tests {
		actual, err := parseAMPApplicationUptime(raw)
		if err != nil {
			t.Fatalf("parseAMPApplicationUptime(%q) retornou erro: %v", raw, err)
		}
		if actual != expected {
			t.Fatalf(
				"parseAMPApplicationUptime(%q)=%s; esperado=%s",
				raw,
				actual,
				expected,
			)
		}
	}
}

func TestParseAMPApplicationUptimeRejectsInvalidValue(t *testing.T) {
	if _, err := parseAMPApplicationUptime("indisponível"); err == nil {
		t.Fatal("uptime inválido deveria retornar erro")
	}
}
