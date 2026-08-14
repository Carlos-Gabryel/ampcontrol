package idle

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Carlos-Gabryel/ampcontrol/internal/operation"
)

type lockedStopperTestDouble struct {
	calls int
	err   error
}

func (s *lockedStopperTestDouble) StopApplication(
	_ context.Context,
	_ Server,
) error {
	s.calls++

	return s.err
}

func TestNewLockedStopperRejectsNilManager(
	t *testing.T,
) {
	t.Parallel()

	next := &lockedStopperTestDouble{}

	_, err := NewLockedStopper(
		nil,
		next,
	)

	if err == nil {
		t.Fatal(
			"era esperado um erro para gerenciador nulo",
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

func TestNewLockedStopperRejectsNilNextStopper(
	t *testing.T,
) {
	t.Parallel()

	_, err := NewLockedStopper(
		operation.NewManager(),
		nil,
	)

	if err == nil {
		t.Fatal(
			"era esperado um erro para controlador nulo",
		)
	}

	if !strings.Contains(
		err.Error(),
		"controlador de parada",
	) {
		t.Fatalf(
			"erro inesperado: %v",
			err,
		)
	}
}

func TestLockedStopperAcquiresAndReleasesInstance(
	t *testing.T,
) {
	t.Parallel()

	manager := operation.NewManager()

	next := &lockedStopperTestDouble{}

	stopper, err := NewLockedStopper(
		manager,
		next,
	)
	if err != nil {
		t.Fatalf(
			"NewLockedStopper retornou erro: %v",
			err,
		)
	}

	server := Server{
		Instance:    "AlamamaPal01",
		DisplayName: "Alamama",
		Enabled:     true,
	}

	err = stopper.StopApplication(
		context.Background(),
		server,
	)
	if err != nil {
		t.Fatalf(
			"StopApplication retornou erro: %v",
			err,
		)
	}

	if next.calls != 1 {
		t.Fatalf(
			"o controlador seguinte deveria ser chamado uma vez; chamadas=%d",
			next.calls,
		)
	}

	if _, exists := manager.Current(
		server.Instance,
	); exists {
		t.Fatal(
			"o bloqueio deveria ter sido liberado após a parada",
		)
	}
}

func TestLockedStopperBlocksWhenManualOperationIsActive(
	t *testing.T,
) {
	t.Parallel()

	manager := operation.NewManager()

	manualOperation, err := manager.TryAcquire(
		"AlamamaPal01",
		"comando /amp reiniciar",
	)
	if err != nil {
		t.Fatalf(
			"não foi possível criar a operação manual: %v",
			err,
		)
	}

	if !manualOperation.Acquired() {
		t.Fatal(
			"a operação manual deveria adquirir a instância",
		)
	}

	defer manualOperation.Lease.Release()

	next := &lockedStopperTestDouble{}

	stopper, err := NewLockedStopper(
		manager,
		next,
	)
	if err != nil {
		t.Fatalf(
			"NewLockedStopper retornou erro: %v",
			err,
		)
	}

	err = stopper.StopApplication(
		context.Background(),
		Server{
			Instance:    "alamamapal01",
			DisplayName: "Alamama",
			Enabled:     true,
		},
	)

	if err == nil {
		t.Fatal(
			"era esperado um erro informando a operação concorrente",
		)
	}

	var busyError *OperationBusyError

	if !errors.As(
		err,
		&busyError,
	) {
		t.Fatalf(
			"era esperado OperationBusyError; recebido: %T: %v",
			err,
			err,
		)
	}

	if busyError.Active.Operation != "comando /amp reiniciar" {
		t.Fatalf(
			"operação ativa inesperada: %q",
			busyError.Active.Operation,
		)
	}

	if next.calls != 0 {
		t.Fatalf(
			"o controlador real não deveria ser chamado; chamadas=%d",
			next.calls,
		)
	}
}

func TestLockedStopperAllowsDifferentInstance(
	t *testing.T,
) {
	t.Parallel()

	manager := operation.NewManager()

	manualOperation, err := manager.TryAcquire(
		"AlamamaPal01",
		"comando /amp atualizar",
	)
	if err != nil {
		t.Fatalf(
			"não foi possível adquirir Alamama: %v",
			err,
		)
	}

	if !manualOperation.Acquired() {
		t.Fatal(
			"Alamama deveria ter sido adquirida",
		)
	}

	defer manualOperation.Lease.Release()

	next := &lockedStopperTestDouble{}

	stopper, err := NewLockedStopper(
		manager,
		next,
	)
	if err != nil {
		t.Fatalf(
			"NewLockedStopper retornou erro: %v",
			err,
		)
	}

	err = stopper.StopApplication(
		context.Background(),
		Server{
			Instance:    "KalagaPal01",
			DisplayName: "Kalaga",
			Enabled:     true,
		},
	)
	if err != nil {
		t.Fatalf(
			"Kalaga deveria ser independente de Alamama: %v",
			err,
		)
	}

	if next.calls != 1 {
		t.Fatalf(
			"o controlador de Kalaga deveria ser chamado; chamadas=%d",
			next.calls,
		)
	}
}

func TestLockedStopperPropagatesStopErrorAndReleasesLock(
	t *testing.T,
) {
	t.Parallel()

	manager := operation.NewManager()

	expectedError := errors.New(
		"falha simulada no Core.Stop",
	)

	next := &lockedStopperTestDouble{
		err: expectedError,
	}

	stopper, err := NewLockedStopper(
		manager,
		next,
	)
	if err != nil {
		t.Fatalf(
			"NewLockedStopper retornou erro: %v",
			err,
		)
	}

	server := Server{
		Instance:    "AlamamaPal01",
		DisplayName: "Alamama",
		Enabled:     true,
	}

	err = stopper.StopApplication(
		context.Background(),
		server,
	)

	if !errors.Is(
		err,
		expectedError,
	) {
		t.Fatalf(
			"erro inesperado: %v",
			err,
		)
	}

	if _, exists := manager.Current(
		server.Instance,
	); exists {
		t.Fatal(
			"o bloqueio deveria ser liberado mesmo após erro",
		)
	}
}

func TestLockedStopperRejectsNilContext(
	t *testing.T,
) {
	t.Parallel()

	stopper, err := NewLockedStopper(
		operation.NewManager(),
		&lockedStopperTestDouble{},
	)
	if err != nil {
		t.Fatalf(
			"NewLockedStopper retornou erro: %v",
			err,
		)
	}

	err = stopper.StopApplication(
		nil,
		Server{
			Instance: "AlamamaPal01",
			Enabled:  true,
		},
	)

	if err == nil {
		t.Fatal(
			"era esperado um erro para contexto nulo",
		)
	}

	if !strings.Contains(
		err.Error(),
		"contexto",
	) {
		t.Fatalf(
			"erro inesperado: %v",
			err,
		)
	}
}
