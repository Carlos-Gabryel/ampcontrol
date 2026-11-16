package discord

import (
	"errors"
	"fmt"
	"testing"

	"github.com/gorilla/websocket"
)

func TestIsAuthenticationFailure(t *testing.T) {
	closeErr := func(code int) error {
		return &websocket.CloseError{Code: code, Text: "Authentication failed."}
	}
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"4004", closeErr(4004), true},
		{"4004 envolvido", fmt.Errorf("falha: %w", closeErr(4004)), true},
		{"sentinela", fmt.Errorf("%w: x", ErrAuthenticationFailed), true},
		{"4014", closeErr(4014), false},
		{"1006", closeErr(1006), false},
		{"outro erro", errors.New("boom"), false},
		{"nil", nil, false},
	}
	for _, tc := range cases {
		if got := isAuthenticationFailure(tc.err); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}
