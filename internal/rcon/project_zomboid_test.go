package rcon

import (
	"strings"
	"testing"
)

func TestParseProjectZomboidPlayerCount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		output string
		want   int
	}{
		{
			name:   "nenhum jogador",
			output: "Players connected (0):",
			want:   0,
		},
		{
			name:   "jogadores conectados",
			output: "Players connected (2):\n-player-one\n-player-two",
			want:   2,
		},
		{
			name:   "espaços e caixa diferentes",
			output: "  PLAYERS connected ( 3 ) :  ",
			want:   3,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseProjectZomboidPlayerCount(test.output)
			if err != nil {
				t.Fatalf("não foi possível interpretar a resposta: %v", err)
			}
			if got != test.want {
				t.Fatalf("contagem inesperada: obtida=%d esperada=%d", got, test.want)
			}
		})
	}
}

func TestParseProjectZomboidPlayerCountRejectsUnknownOutput(t *testing.T) {
	t.Parallel()

	_, err := parseProjectZomboidPlayerCount("comando desconhecido")
	if err == nil {
		t.Fatal("era esperado um erro para uma resposta desconhecida")
	}
	if !strings.Contains(err.Error(), "interpretar") {
		t.Fatalf("erro inesperado: %v", err)
	}
}
