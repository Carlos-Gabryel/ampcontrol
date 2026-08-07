package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/alabamaamp/palcontrol/internal/idle"
	"github.com/rs/zerolog"
)

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
