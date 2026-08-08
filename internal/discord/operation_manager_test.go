package discord

import (
	"strings"
	"testing"

	"github.com/alabamaamp/ampcontrol/internal/operation"
)

func TestSetOperationManagerRejectsNilClient(
	t *testing.T,
) {
	t.Parallel()

	var client *Client

	err := client.SetOperationManager(
		operation.NewManager(),
	)

	if err == nil {
		t.Fatal(
			"era esperado um erro para cliente Discord nulo",
		)
	}

	if !strings.Contains(
		err.Error(),
		"cliente Discord",
	) {
		t.Fatalf(
			"erro inesperado: %v",
			err,
		)
	}
}

func TestSetOperationManagerRejectsNilManager(
	t *testing.T,
) {
	t.Parallel()

	client := &Client{}

	defer clearOperationManager(
		client,
	)

	err := client.SetOperationManager(
		nil,
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

func TestOperationManagerRejectsUnconfiguredClient(
	t *testing.T,
) {
	t.Parallel()

	client := &Client{}

	defer clearOperationManager(
		client,
	)

	_, err := client.operationManager()

	if err == nil {
		t.Fatal(
			"era esperado um erro para cliente sem gerenciador configurado",
		)
	}

	if !strings.Contains(
		err.Error(),
		"não foi configurado",
	) {
		t.Fatalf(
			"erro inesperado: %v",
			err,
		)
	}
}

func TestClientKeepsSameOperationManager(
	t *testing.T,
) {
	t.Parallel()

	client := &Client{}

	defer clearOperationManager(
		client,
	)

	expectedManager := operation.NewManager()

	err := client.SetOperationManager(
		expectedManager,
	)
	if err != nil {
		t.Fatalf(
			"SetOperationManager retornou erro: %v",
			err,
		)
	}

	actualManager, err := client.operationManager()
	if err != nil {
		t.Fatalf(
			"operationManager retornou erro: %v",
			err,
		)
	}

	if actualManager != expectedManager {
		t.Fatal(
			"o Client não retornou a mesma instância do gerenciador configurado",
		)
	}
}

func TestDifferentClientsCanUseSameOperationManager(
	t *testing.T,
) {
	t.Parallel()

	firstClient := &Client{}
	secondClient := &Client{}

	defer clearOperationManager(
		firstClient,
	)

	defer clearOperationManager(
		secondClient,
	)

	manager := operation.NewManager()

	if err := firstClient.SetOperationManager(
		manager,
	); err != nil {
		t.Fatalf(
			"não foi possível configurar o primeiro cliente: %v",
			err,
		)
	}

	if err := secondClient.SetOperationManager(
		manager,
	); err != nil {
		t.Fatalf(
			"não foi possível configurar o segundo cliente: %v",
			err,
		)
	}

	firstManager, err := firstClient.operationManager()
	if err != nil {
		t.Fatalf(
			"não foi possível recuperar o gerenciador do primeiro cliente: %v",
			err,
		)
	}

	secondManager, err := secondClient.operationManager()
	if err != nil {
		t.Fatalf(
			"não foi possível recuperar o gerenciador do segundo cliente: %v",
			err,
		)
	}

	if firstManager != manager {
		t.Fatal(
			"o primeiro cliente não recebeu o gerenciador esperado",
		)
	}

	if secondManager != manager {
		t.Fatal(
			"o segundo cliente não recebeu o gerenciador esperado",
		)
	}

	if firstManager != secondManager {
		t.Fatal(
			"os clientes deveriam compartilhar a mesma instância",
		)
	}
}

func TestOperationManagerActuallySharesLocks(
	t *testing.T,
) {
	t.Parallel()

	client := &Client{}

	defer clearOperationManager(
		client,
	)

	manager := operation.NewManager()

	if err := client.SetOperationManager(
		manager,
	); err != nil {
		t.Fatalf(
			"não foi possível configurar o gerenciador: %v",
			err,
		)
	}

	clientManager, err := client.operationManager()
	if err != nil {
		t.Fatalf(
			"não foi possível recuperar o gerenciador: %v",
			err,
		)
	}

	first, err := manager.TryAcquire(
		"AlamamaPal01",
		"teste pela camada app",
	)
	if err != nil {
		t.Fatalf(
			"não foi possível adquirir o primeiro bloqueio: %v",
			err,
		)
	}

	defer first.Lease.Release()

	second, err := clientManager.TryAcquire(
		"alamamapal01",
		"teste pelo Discord",
	)
	if err != nil {
		t.Fatalf(
			"a segunda tentativa retornou erro: %v",
			err,
		)
	}

	if second.Acquired() {
		second.Lease.Release()

		t.Fatal(
			"o gerenciador recuperado pelo Discord não compartilhou o bloqueio",
		)
	}

	if second.Active == nil {
		t.Fatal(
			"a operação concorrente deveria ter sido informada",
		)
	}

	if second.Active.Operation != "teste pela camada app" {
		t.Fatalf(
			"operação concorrente inesperada: %q",
			second.Active.Operation,
		)
	}
}
