package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/alabamaamp/palcontrol/internal/amp"
	"github.com/alabamaamp/palcontrol/internal/idle"
	"github.com/alabamaamp/palcontrol/internal/operation"
	"github.com/rs/zerolog"
)

type noopIdleNotifier struct{}

func (
	noopIdleNotifier,
) SendNotification(
	string,
) error {
	return nil
}

func TestObservationOnlyStopperBlocksCoreStop(
	t *testing.T,
) {
	t.Parallel()

	stopper := observationOnlyStopper{}

	err := stopper.StopApplication(
		context.Background(),
		idle.Server{
			Instance:    "AlamamaPal01",
			DisplayName: "Alamama",
			Enabled:     true,
		},
	)

	if err == nil {
		t.Fatal(
			"era esperado o aviso interno do modo de observação",
		)
	}

	if !errors.Is(
		err,
		errIdleObservationOnly,
	) {
		t.Fatalf(
			"erro inesperado: %v",
			err,
		)
	}
}

func TestNewIdleObserverRejectsNilAMPClient(
	t *testing.T,
) {
	t.Parallel()

	_, err := newIdleObserver(
		nil,
		operation.NewManager(),
		noopIdleNotifier{},
		zerolog.Nop(),
	)

	if err == nil {
		t.Fatal(
			"era esperado um erro para cliente AMP nulo",
		)
	}

	if !strings.Contains(
		err.Error(),
		"cliente AMP",
	) {
		t.Fatalf(
			"erro inesperado: %v",
			err,
		)
	}
}

func TestNewIdleObserverRejectsNilOperationManager(
	t *testing.T,
) {
	t.Parallel()

	ampClient := amp.NewAPIClient(
		"usuario-teste",
		"senha-teste",
	)

	_, err := newIdleObserver(
		ampClient,
		nil,
		noopIdleNotifier{},
		zerolog.Nop(),
	)

	if err == nil {
		t.Fatal(
			"era esperado um erro para gerenciador de operações nulo",
		)
	}

	if !strings.Contains(
		err.Error(),
		"gerenciador de operações",
	) {
		t.Fatalf(
			"erro inesperado: %v",
			err,
		)
	}
}

func TestNewIdleObserverRejectsNilNotifier(
	t *testing.T,
) {
	t.Parallel()

	ampClient := amp.NewAPIClient(
		"usuario-teste",
		"senha-teste",
	)

	_, err := newIdleObserver(
		ampClient,
		operation.NewManager(),
		nil,
		zerolog.Nop(),
	)

	if err == nil {
		t.Fatal(
			"era esperado um erro para notificante nulo",
		)
	}

	if !strings.Contains(
		err.Error(),
		"notificante",
	) {
		t.Fatalf(
			"erro inesperado: %v",
			err,
		)
	}
}
