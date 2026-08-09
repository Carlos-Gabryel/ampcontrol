package discord

import (
	"strings"
	"testing"

	"github.com/alabamaamp/ampcontrol/internal/operation"
)

func TestAcquireSharedOperationRejectsClientWithoutManager(
	t *testing.T,
) {
	t.Parallel()

	client := &Client{}

	defer clearOperationManager(
		client,
	)

	release, activeOperation, acquired, err :=
		client.acquireSharedOperation(
			"AlamamaPal01",
			"monitor Idle antigo",
		)

	if err == nil {
		t.Fatal(
			"era esperado um erro para cliente sem gerenciador",
		)
	}

	if release != nil {
		t.Fatal(
			"nenhuma função de liberação deveria ser criada",
		)
	}

	if activeOperation != "" {
		t.Fatalf(
			"operação ativa inesperada: %q",
			activeOperation,
		)
	}

	if acquired {
		t.Fatal(
			"a instância não deveria ter sido adquirida",
		)
	}

	if !strings.Contains(
		err.Error(),
		"gerenciador compartilhado",
	) {
		t.Fatalf(
			"erro inesperado: %v",
			err,
		)
	}
}

func TestAcquireSharedOperationAcquiresAndReleases(
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

	release, activeOperation, acquired, err :=
		client.acquireSharedOperation(
			"AlamamaPal01",
			"monitor Idle antigo",
		)
	if err != nil {
		t.Fatalf(
			"a aquisição retornou erro: %v",
			err,
		)
	}

	if !acquired {
		t.Fatal(
			"a instância deveria ter sido adquirida",
		)
	}

	if release == nil {
		t.Fatal(
			"a função de liberação deveria ter sido retornada",
		)
	}

	if activeOperation != "" {
		t.Fatalf(
			"nenhuma operação concorrente era esperada: %q",
			activeOperation,
		)
	}

	info, exists := manager.Current(
		"AlamamaPal01",
	)

	if !exists {
		t.Fatal(
			"a operação adquirida não foi encontrada",
		)
	}

	if info.Operation != "monitor Idle antigo" {
		t.Fatalf(
			"descrição inesperada: %q",
			info.Operation,
		)
	}

	release()

	if _, exists := manager.Current(
		"AlamamaPal01",
	); exists {
		t.Fatal(
			"a instância deveria estar livre após release",
		)
	}
}

func TestAcquireSharedOperationReportsActiveOperation(
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

	manualOperation, err := manager.TryAcquire(
		"AlamamaPal01",
		"comando /amp reiniciar",
	)
	if err != nil {
		t.Fatalf(
			"não foi possível criar a operação concorrente: %v",
			err,
		)
	}

	if !manualOperation.Acquired() {
		t.Fatal(
			"a operação manual deveria adquirir a instância",
		)
	}

	defer manualOperation.Lease.Release()

	release, activeOperation, acquired, err :=
		client.acquireSharedOperation(
			"alamamapal01",
			"monitor Idle antigo",
		)
	if err != nil {
		t.Fatalf(
			"a tentativa concorrente retornou erro: %v",
			err,
		)
	}

	if acquired {
		if release != nil {
			release()
		}

		t.Fatal(
			"o monitor não deveria adquirir uma instância ocupada",
		)
	}

	if release != nil {
		t.Fatal(
			"não deveria existir função de liberação",
		)
	}

	if activeOperation != "comando /amp reiniciar" {
		t.Fatalf(
			"operação ativa inesperada: %q",
			activeOperation,
		)
	}
}

func TestAcquireSharedOperationAllowsDifferentInstances(
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

	alamama, err := manager.TryAcquire(
		"AlamamaPal01",
		"comando /amp atualizar",
	)
	if err != nil {
		t.Fatalf(
			"não foi possível reservar Alamama: %v",
			err,
		)
	}

	if !alamama.Acquired() {
		t.Fatal(
			"Alamama deveria ter sido adquirida",
		)
	}

	defer alamama.Lease.Release()

	release, activeOperation, acquired, err :=
		client.acquireSharedOperation(
			"KalagaPal01",
			"monitor Idle antigo",
		)
	if err != nil {
		t.Fatalf(
			"não foi possível reservar Kalaga: %v",
			err,
		)
	}

	if !acquired {
		t.Fatalf(
			"Kalaga deveria ser independente; operação encontrada: %q",
			activeOperation,
		)
	}

	if release == nil {
		t.Fatal(
			"a função de liberação de Kalaga deveria existir",
		)
	}

	release()
}
