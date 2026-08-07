package discord

import (
	"fmt"

	"github.com/alabamaamp/palcontrol/internal/operation"
)

// acquireAMPCommandOperation tenta reservar uma instância exclusivamente
// para uma operação manual executada pelo comando /amp.
//
// O método não espera outra operação terminar. Quando a instância já
// estiver ocupada, lease será nil e active descreverá a operação atual.
func (c *Client) acquireAMPCommandOperation(
	instanceName string,
	commandOperation ampCommandOperation,
) (
	*operation.Lease,
	*operation.Info,
	error,
) {
	manager, err := c.operationManager()
	if err != nil {
		return nil, nil, fmt.Errorf(
			"não foi possível acessar o gerenciador de operações: %w",
			err,
		)
	}

	result, err := manager.TryAcquire(
		instanceName,
		ampCommandOperationDescription(
			commandOperation,
		),
	)
	if err != nil {
		return nil, nil, fmt.Errorf(
			"não foi possível reservar a instância %s: %w",
			instanceName,
			err,
		)
	}

	if result.Acquired() {
		return result.Lease, nil, nil
	}

	return nil, result.Active, nil
}

// ampCommandOperationDescription gera o nome armazenado no gerenciador.
//
// Esses nomes também serão usados posteriormente pelo motor genérico de
// Idle para informar qual operação manual está impedindo uma parada
// automática.
func ampCommandOperationDescription(
	commandOperation ampCommandOperation,
) string {
	switch commandOperation {
	case ampCommandOperationStart:
		return "comando /amp iniciar"

	case ampCommandOperationStop:
		return "comando /amp parar"

	case ampCommandOperationRestart:
		return "comando /amp reiniciar"

	case ampCommandOperationShutdown:
		return "comando /amp desligar"

	case ampCommandOperationUpdate:
		return "comando /amp atualizar"

	default:
		return fmt.Sprintf(
			"comando /amp desconhecido (%s)",
			commandOperation,
		)
	}
}
