package discord

import (
	"errors"
	"sync"

	"github.com/Carlos-Gabryel/ampcontrol/internal/i18n"

	"github.com/Carlos-Gabryel/ampcontrol/internal/operation"
)

// clientOperationManagers mantém a associação entre cada Client do Discord
// e o gerenciador de operações criado pela camada app.
//
// O Client já existia antes da introdução do bloqueio por instância.
// Manter essa associação em um arquivo separado permite adicionar a
// coordenação sem alterar a estrutura principal do Client nesta etapa.
var clientOperationManagers sync.Map

// SetOperationManager associa ao Client o gerenciador compartilhado
// responsável por coordenar operações exclusivas nas instâncias AMP.
//
// O mesmo Manager deverá posteriormente ser usado pelo motor genérico
// de Idle.
func (c *Client) SetOperationManager(
	manager *operation.Manager,
) error {
	if c == nil {
		return errors.New(i18n.Choose("o cliente Discord não foi inicializado", "the Discord client was not initialized"))
	}

	if manager == nil {
		return errors.New(i18n.Choose("o gerenciador de operações não foi informado", "the operation manager was not provided"))
	}

	clientOperationManagers.Store(
		c,
		manager,
	)

	return nil
}

// operationManager retorna o gerenciador associado ao Client.
//
// Os comandos /amp usarão esse método quando o bloqueio for ativado
// na próxima etapa.
func (c *Client) operationManager() (
	*operation.Manager,
	error,
) {
	if c == nil {
		return nil, errors.New(i18n.Choose("o cliente Discord não foi inicializado", "the Discord client was not initialized"))
	}

	value, exists := clientOperationManagers.Load(
		c,
	)
	if !exists {
		return nil, errors.New(i18n.Choose("o gerenciador de operações do cliente Discord não foi configurado", "the Discord client operation manager was not configured"))
	}

	manager, valid := value.(*operation.Manager)
	if !valid || manager == nil {
		return nil, errors.New(i18n.Choose("o gerenciador de operações do cliente Discord é inválido", "the Discord client operation manager is invalid"))
	}

	return manager, nil
}

// clearOperationManager remove a associação de um Client.
//
// Atualmente ele é usado pelos testes para que cada caso permaneça
// completamente isolado.
func clearOperationManager(
	client *Client,
) {
	if client == nil {
		return
	}

	clientOperationManagers.Delete(
		client,
	)
}
