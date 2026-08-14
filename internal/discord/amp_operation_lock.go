package discord

import (
	"fmt"

	"github.com/Carlos-Gabryel/ampcontrol/internal/i18n"
	"github.com/Carlos-Gabryel/ampcontrol/internal/operation"
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
		return i18n.Choose("comando /amp iniciar", "command /amp start")

	case ampCommandOperationStop:
		return i18n.Choose("comando /amp parar", "command /amp stop")

	case ampCommandOperationRestart:
		return i18n.Choose("comando /amp reiniciar", "command /amp restart")

	case ampCommandOperationShutdown:
		return i18n.Choose("comando /amp desligar", "command /amp shutdown")

	case ampCommandOperationUpdate:
		return i18n.Choose("comando /amp atualizar", "command /amp update")

	default:
		return fmt.Sprintf(
			i18n.Choose("comando /amp desconhecido (%s)", "unknown /amp command (%s)"),
			commandOperation,
		)
	}
}
