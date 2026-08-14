package logger

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/Carlos-Gabryel/ampcontrol/internal/i18n"
)

func TestLocalizedJSONWriterTranslatesHumanFields(t *testing.T) {
	previous := i18n.Default()
	t.Cleanup(func() { i18n.SetDefault(previous) })
	i18n.SetDefault(i18n.EnglishUS)

	var output bytes.Buffer
	input := []byte(`{"level":"error","error":"não foi possível acessar a instância","message":"Discord conectado"}` + "\n")
	if _, err := (localizedJSONWriter{target: &output}).Write(input); err != nil {
		t.Fatal(err)
	}
	var event map[string]any
	if err := json.Unmarshal(output.Bytes(), &event); err != nil {
		t.Fatalf("output is not JSON: %v", err)
	}
	if event["message"] != "Discord connected" {
		t.Fatalf("message = %q", event["message"])
	}
	if event["error"] == "não foi possível acessar a instância" {
		t.Fatalf("error was not translated: %q", event["error"])
	}
}
