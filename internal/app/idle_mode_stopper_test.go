package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/alabamaamp/palcontrol/internal/idle"
)

type recordingIdleStopper struct {
	calls      int
	lastServer idle.Server
	err        error
}

func (s *recordingIdleStopper) StopApplication(
	_ context.Context,
	server idle.Server,
) error {
	s.calls++
	s.lastServer = server

	return s.err
}

func TestModeAwareStopperBlocksObserve(
	t *testing.T,
) {
	t.Parallel()

	activeStopper := &recordingIdleStopper{}

	stopper, err := newModeAwareStopper(
		activeStopper,
	)
	if err != nil {
		t.Fatalf(
			"newModeAwareStopper retornou erro: %v",
			err,
		)
	}

	err = stopper.StopApplication(
		context.Background(),
		idle.Server{
			Instance:    "AlamamaPal01",
			DisplayName: "Alamama",
			Enabled:     true,
			Mode:        idle.ServerModeObserve,
		},
	)

	if !errors.Is(
		err,
		errIdleObservationOnly,
	) {
		t.Fatalf(
			"modo observe deveria bloquear a parada; erro=%v",
			err,
		)
	}

	if activeStopper.calls != 0 {
		t.Fatalf(
			"stopper real não deveria ser chamado em observe; chamadas=%d",
			activeStopper.calls,
		)
	}
}

func TestModeAwareStopperTreatsEmptyModeAsObserve(
	t *testing.T,
) {
	t.Parallel()

	activeStopper := &recordingIdleStopper{}

	stopper, err := newModeAwareStopper(
		activeStopper,
	)
	if err != nil {
		t.Fatalf(
			"newModeAwareStopper retornou erro: %v",
			err,
		)
	}

	err = stopper.StopApplication(
		context.Background(),
		idle.Server{
			Instance:    "KalagaPal01",
			DisplayName: "Kalaga",
			Enabled:     true,
		},
	)

	if !errors.Is(
		err,
		errIdleObservationOnly,
	) {
		t.Fatalf(
			"mode vazio deveria ser tratado como observe; erro=%v",
			err,
		)
	}

	if activeStopper.calls != 0 {
		t.Fatalf(
			"stopper real não deveria ser chamado para mode vazio; chamadas=%d",
			activeStopper.calls,
		)
	}
}

func TestModeAwareStopperDelegatesActive(
	t *testing.T,
) {
	t.Parallel()

	activeStopper := &recordingIdleStopper{}

	stopper, err := newModeAwareStopper(
		activeStopper,
	)
	if err != nil {
		t.Fatalf(
			"newModeAwareStopper retornou erro: %v",
			err,
		)
	}

	server := idle.Server{
		Instance:    "AlamamaPal01",
		DisplayName: "Alamama",
		Enabled:     true,
		Mode:        idle.ServerModeActive,
	}

	err = stopper.StopApplication(
		context.Background(),
		server,
	)
	if err != nil {
		t.Fatalf(
			"modo active retornou erro inesperado: %v",
			err,
		)
	}

	if activeStopper.calls != 1 {
		t.Fatalf(
			"stopper real deveria ser chamado uma vez; chamadas=%d",
			activeStopper.calls,
		)
	}

	if activeStopper.lastServer.Instance != server.Instance {
		t.Fatalf(
			"instância encaminhada incorretamente: %s",
			activeStopper.lastServer.Instance,
		)
	}

	if activeStopper.lastServer.Mode != idle.ServerModeActive {
		t.Fatalf(
			"modo encaminhado incorretamente: %q",
			activeStopper.lastServer.Mode,
		)
	}
}

func TestModeAwareStopperPropagatesActiveError(
	t *testing.T,
) {
	t.Parallel()

	expectedErr := errors.New(
		"falha de teste no Core.Stop",
	)

	activeStopper := &recordingIdleStopper{
		err: expectedErr,
	}

	stopper, err := newModeAwareStopper(
		activeStopper,
	)
	if err != nil {
		t.Fatalf(
			"newModeAwareStopper retornou erro: %v",
			err,
		)
	}

	err = stopper.StopApplication(
		context.Background(),
		idle.Server{
			Instance:    "AlamamaPal01",
			DisplayName: "Alamama",
			Enabled:     true,
			Mode:        idle.ServerModeActive,
		},
	)

	if !errors.Is(
		err,
		expectedErr,
	) {
		t.Fatalf(
			"erro do stopper real deveria ser propagado; erro=%v",
			err,
		)
	}

	if activeStopper.calls != 1 {
		t.Fatalf(
			"stopper real deveria ser chamado uma vez; chamadas=%d",
			activeStopper.calls,
		)
	}
}

func TestModeAwareStopperRejectsUnknownMode(
	t *testing.T,
) {
	t.Parallel()

	activeStopper := &recordingIdleStopper{}

	stopper, err := newModeAwareStopper(
		activeStopper,
	)
	if err != nil {
		t.Fatalf(
			"newModeAwareStopper retornou erro: %v",
			err,
		)
	}

	err = stopper.StopApplication(
		context.Background(),
		idle.Server{
			Instance:    "AlamamaPal01",
			DisplayName: "Alamama",
			Enabled:     true,
			Mode:        idle.ServerMode("modo-invalido"),
		},
	)

	if err == nil {
		t.Fatal(
			"modo desconhecido deveria retornar erro",
		)
	}

	if !strings.Contains(
		err.Error(),
		"não reconhecido",
	) {
		t.Fatalf(
			"erro inesperado: %v",
			err,
		)
	}

	if activeStopper.calls != 0 {
		t.Fatalf(
			"stopper real não deveria ser chamado para modo desconhecido; chamadas=%d",
			activeStopper.calls,
		)
	}
}

func TestNewModeAwareStopperRejectsNilActiveStopper(
	t *testing.T,
) {
	t.Parallel()

	_, err := newModeAwareStopper(
		nil,
	)

	if err == nil {
		t.Fatal(
			"stopper active nulo deveria retornar erro",
		)
	}

	if !strings.Contains(
		err.Error(),
		"modo active",
	) {
		t.Fatalf(
			"erro inesperado: %v",
			err,
		)
	}
}
