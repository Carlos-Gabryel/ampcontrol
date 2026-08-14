package app

import (
	"testing"

	"github.com/Carlos-Gabryel/ampcontrol/internal/idle"
)

func TestParseIdleDetectionMethod(t *testing.T) {
	tests := map[string]idle.Detector{
		"amp":                      "",
		"amp_palworld_rcon":        idle.DetectorPalworldRCON,
		"amp_project_zomboid_rcon": idle.DetectorProjectZomboidRCON,
	}
	for method, expectedFallback := range tests {
		detector, fallback, err := parseIdleDetectionMethod(method)
		if err != nil || detector != idle.DetectorAMPPlayers || fallback != expectedFallback {
			t.Fatalf("método %s inesperado: detector=%s fallback=%s err=%v", method, detector, fallback, err)
		}
	}
	if _, _, err := parseIdleDetectionMethod("desconhecido"); err == nil {
		t.Fatal("método desconhecido deveria ser rejeitado")
	}
}

func TestIdleDetectionMethodPresentation(t *testing.T) {
	server := idle.Server{Detector: idle.DetectorAMPPlayers, FallbackDetector: idle.DetectorProjectZomboidRCON}
	if actual := idleDetectionMethod(server); actual != "amp_project_zomboid_rcon" {
		t.Fatalf("método apresentado inesperado: %s", actual)
	}
}
