package idle

import (
	"context"
	"fmt"
	"strings"

	"github.com/Carlos-Gabryel/ampcontrol/internal/operation"
)

const automaticIdleOperationName = "Idle automático"

// OperationBusyError informa que a parada automática não pôde adquirir
// o controle exclusivo da instância porque outra operação já está ativa.
type OperationBusyError struct {
	Instance string
	Active   operation.Info
}

func (e *OperationBusyError) Error() string {
	if e == nil {
		return "a instância possui outra operação em andamento"
	}

	instance := strings.TrimSpace(
		e.Instance,
	)

	if instance == "" {
		instance = "desconhecida"
	}

	activeOperation := strings.TrimSpace(
		e.Active.Operation,
	)

	if activeOperation == "" {
		return fmt.Sprintf(
			"a instância %s possui outra operação em andamento",
			instance,
		)
	}

	return fmt.Sprintf(
		"a instância %s já está sendo usada por %s",
		instance,
		activeOperation,
	)
}

// LockedStopper adiciona exclusão mútua por instância a qualquer
// ApplicationStopper.
//
// O bloqueio é adquirido imediatamente antes da parada e liberado
// independentemente de sucesso ou falha da operação seguinte.
type LockedStopper struct {
	manager *operation.Manager
	next    ApplicationStopper
}

// NewLockedStopper cria um ApplicationStopper protegido pelo mesmo
// gerenciador usado pelos comandos manuais.
func NewLockedStopper(
	manager *operation.Manager,
	next ApplicationStopper,
) (*LockedStopper, error) {
	if manager == nil {
		return nil, fmt.Errorf(
			"o gerenciador de operações do Idle não foi informado",
		)
	}

	if next == nil {
		return nil, fmt.Errorf(
			"o controlador de parada protegido não foi informado",
		)
	}

	return &LockedStopper{
		manager: manager,
		next:    next,
	}, nil
}

// StopApplication tenta adquirir exclusivamente a instância antes
// de permitir que o próximo ApplicationStopper seja executado.
//
// Caso uma operação manual esteja ativa, a parada automática é
// recusada imediatamente.
func (s *LockedStopper) StopApplication(
	ctx context.Context,
	server Server,
) error {
	if s == nil {
		return fmt.Errorf(
			"o controlador protegido de Idle não foi inicializado",
		)
	}

	if ctx == nil {
		return fmt.Errorf(
			"o contexto da parada automática não foi informado",
		)
	}

	if s.manager == nil {
		return fmt.Errorf(
			"o gerenciador de operações do Idle não foi configurado",
		)
	}

	if s.next == nil {
		return fmt.Errorf(
			"o controlador de parada protegido não foi configurado",
		)
	}

	result, err := s.manager.TryAcquire(
		server.Instance,
		automaticIdleOperationName,
	)
	if err != nil {
		return fmt.Errorf(
			"não foi possível reservar a instância para o Idle automático: %w",
			err,
		)
	}

	if !result.Acquired() {
		busyError := &OperationBusyError{
			Instance: server.Instance,
		}

		if result.Active != nil {
			busyError.Active = *result.Active
		}

		return busyError
	}

	defer result.Lease.Release()

	if err := s.next.StopApplication(
		ctx,
		server,
	); err != nil {
		return err
	}

	return nil
}
