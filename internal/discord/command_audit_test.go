package discord

import (
	"testing"
)

func TestFormatCommandAuditPath(t *testing.T) {
	tests := map[string]string{
		"/amp/iniciar":              "/amp iniciar",
		"/ampconfig/idle-adicionar": "/ampconfig idle-adicionar",
		"amp/status":                "/amp status",
		"":                          "/",
	}

	for input, expected := range tests {
		if actual := formatCommandAuditPath(input); actual != expected {
			t.Fatalf("formatação de %q: obtido %q, esperado %q", input, actual, expected)
		}
	}
}
