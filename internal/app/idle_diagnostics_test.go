package app

import (
	"errors"
	"testing"

	"github.com/Carlos-Gabryel/ampcontrol/internal/idle"
)

func TestIdleEventIsDiagnosticFailure(t *testing.T) {
	testErr := errors.New("falha")
	if !idleEventIsDiagnosticFailure(idle.Event{
		Type: idle.EventDetectorFailed,
		Err:  testErr,
	}) {
		t.Fatal("falha do detector deveria afetar o diagnóstico")
	}
	if idleEventIsDiagnosticFailure(idle.Event{
		Type: idle.EventPlayersPresent,
		Err:  testErr,
	}) {
		t.Fatal("evento saudável não deveria afetar o diagnóstico")
	}
	if idleEventIsDiagnosticFailure(idle.Event{
		Type: idle.EventStopFailed,
		Err:  errIdleObservationOnly,
	}) {
		t.Fatal("modo de observação não deveria ser tratado como falha")
	}
}

func TestIdleServerUsesRCON(t *testing.T) {
	if !idleServerUsesRCON(idle.Server{
		Detector:         idle.DetectorAMPPlayers,
		FallbackDetector: idle.DetectorProjectZomboidRCON,
	}) {
		t.Fatal("fallback RCON deveria ser detectado")
	}
	if idleServerUsesRCON(idle.Server{
		Detector: idle.DetectorAMPPlayers,
	}) {
		t.Fatal("servidor somente AMP não deveria ser contado como RCON")
	}
}
