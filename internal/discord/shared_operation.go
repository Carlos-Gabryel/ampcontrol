package discord

import (
	"fmt"
	"strings"

	"github.com/Carlos-Gabryel/ampcontrol/internal/i18n"
)

// acquireSharedOperation tenta reservar uma instância usando o mesmo
// gerenciador compartilhado pelos comandos /amp e pelo motor genérico
// de Idle.
//
// O método nunca espera outra operação terminar. Quando a instância
// estiver ocupada, acquired será false e activeOperation identificará
// a operação atual.
//
// Quando acquired for true, release deve obrigatoriamente ser chamado
// ao final da operação.
func (c *Client) acquireSharedOperation(
	instanceName string,
	operationName string,
) (
	release func(),
	activeOperation string,
	acquired bool,
	err error,
) {
	manager, err := c.operationManager()
	if err != nil {
		return nil, "", false, fmt.Errorf(
			i18n.Choose("não foi possível acessar o gerenciador compartilhado de operações: %w", "could not access the shared operation manager: %w"),
			err,
		)
	}

	result, err := manager.TryAcquire(
		instanceName,
		operationName,
	)
	if err != nil {
		return nil, "", false, fmt.Errorf(
			"não foi possível reservar a instância %s: %w",
			instanceName,
			err,
		)
	}

	if result.Acquired() {
		return result.Lease.Release, "", true, nil
	}

	activeOperation = "outra operação"

	if result.Active != nil {
		description := strings.TrimSpace(
			result.Active.Operation,
		)

		if description != "" {
			activeOperation = description
		}
	}

	return nil, activeOperation, false, nil
}
