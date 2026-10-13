package amp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetConsoleEntries(t *testing.T) {
	t.Parallel()

	const sessionID = "sessao-de-teste"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/API/Core/Login":
			_, _ = w.Write([]byte(`{"result":0,"success":true,"sessionID":"` + sessionID + `"}`))
		case "/API/Core/GetUpdates":
			if got := r.Header.Get("Authorization"); got != "Bearer "+sessionID {
				t.Errorf("Authorization inesperado: %q", got)
			}
			var body authenticatedRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.SessionID != sessionID {
				t.Errorf("corpo inesperado: %+v err=%v", body, err)
			}
			_, _ = w.Write([]byte(`{"Status":{},"ConsoleEntries":[` +
				`{"Timestamp":"2026-10-13T04:08:22.27Z","Source":"Console","Type":"Console","Contents":"Session \"A\" registered with join code 454389"}],"Other":1}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	entries, err := NewAPIClient("u", "p").GetConsoleEntries(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(entries) != 1 || ExtractValheimJoinCode(entries) != "454389" {
		t.Fatalf("entradas inesperadas: %+v", entries)
	}
}

func TestGetConsoleEntriesLoginFailure(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/API/Core/GetUpdates" {
			t.Error("GetUpdates não deveria ser chamado sem login")
		}
		_, _ = w.Write([]byte(`{"success":false,"resultReason":"negado"}`))
	}))
	t.Cleanup(server.Close)

	if _, err := NewAPIClient("u", "p").GetConsoleEntries(context.Background(), server.URL); err == nil {
		t.Fatal("era esperado erro de login")
	}
	if _, err := NewAPIClient("u", "p").GetConsoleEntries(context.Background(), " "); err == nil {
		t.Fatal("era esperado erro para URL vazia")
	}
}

func TestExtractValheimJoinCode(t *testing.T) {
	t.Parallel()

	line := func(s string) ConsoleEntry { return ConsoleEntry{Contents: s} }
	empty := line(`New session server "Alabama Valheim" that has join code , now 0 player(s)`)
	registered := line(`Session "Alabama Valheim" registered with join code 454389`)
	active := line(`Session "Alabama Valheim" with join code 454389 and IP 201.71.60.40:2456 is active with 0 player(s)`)

	tests := []struct {
		name    string
		entries []ConsoleEntry
		want    string
	}{
		{"linhas reais", []ConsoleEntry{empty, registered, active}, "454389"},
		{"somente código vazio", []ConsoleEntry{empty}, ""},
		{"último vence", []ConsoleEntry{registered, line("Session X with join code 111222 and IP"), line("JOIN CODE 999000")}, "999000"},
		{"sem correspondência", []ConsoleEntry{line("nada")}, ""},
		{"sem entradas", nil, ""},
	}
	for _, test := range tests {
		if got := ExtractValheimJoinCode(test.entries); got != test.want {
			t.Errorf("%s: got %q, want %q", test.name, got, test.want)
		}
	}
}
