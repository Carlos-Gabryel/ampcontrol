package discord

import (
	"fmt"
	"sync"

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
		return fmt.Errorf(
			"o cliente Discord não foi inicializado",
		)
	}

	if manager == nil {
		return fmt.Errorf(
			"o gerenciador de operações não foi informado",
		)
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
		return nil, fmt.Errorf(
			"o cliente Discord não foi inicializado",
		)
	}

	value, exists := clientOperationManagers.Load(
		c,
	)
	if !exists {
		return nil, fmt.Errorf(
			"o gerenciador de operações do cliente Discord não foi configurado",
		)
	}

	manager, valid := value.(*operation.Manager)
	if !valid || manager == nil {
		return nil, fmt.Errorf(
			"o gerenciador de operações do cliente Discord é inválido",
		)
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
