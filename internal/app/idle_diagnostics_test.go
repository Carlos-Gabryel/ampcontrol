package app

import (
	"errors"
	"os"
	"path/filepath"
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

func TestRCONServerReadyUsesSystemdCredential(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(directory, "rcon_TestServer01"),
		[]byte("segredo-rcon\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CREDENTIALS_DIRECTORY", directory)

	server := idle.Server{
		Instance:       "TestServer01",
		RCONAddress:    "127.0.0.1:25575",
		RCONCredential: "rcon_TestServer01",
	}
	if !rconServerReady(server) {
		t.Fatal("credencial RCON do systemd deveria estar pronta")
	}
}

func TestRCONServerReadyRejectsMissingCredential(t *testing.T) {
	t.Setenv("CREDENTIALS_DIRECTORY", t.TempDir())
	if rconServerReady(idle.Server{
		Instance:       "TestServer01",
		RCONAddress:    "127.0.0.1:25575",
		RCONCredential: "rcon_TestServer01",
	}) {
		t.Fatal("credencial RCON ausente não deveria estar pronta")
	}
}
