package discord

import (
	"testing"

	"github.com/alabamaamp/palcontrol/internal/amp"
	"github.com/alabamaamp/palcontrol/internal/operation"
)

func TestAcquirePalStartOperationUsesExpectedDescription(
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

	server := managedServer{
		DisplayName: "Alamama",
		Instance: amp.Instance{
			Name: "AlamamaPal01",
		},
	}

	release, activeOperation, acquired, err :=
		client.acquirePalStartOperation(
			server,
		)
	if err != nil {
		t.Fatalf(
			"a aquisição retornou erro: %v",
			err,
		)
	}

	if !acquired {
		t.Fatal(
			"/pal iniciar deveria adquirir a instância",
		)
	}

	if release == nil {
		t.Fatal(
			"a função de liberação deveria existir",
		)
	}

	defer release()

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
			"a operação de /pal iniciar não foi encontrada",
		)
	}

	if info.Operation != palStartOperationName {
		t.Fatalf(
			"descrição inesperada: %q",
			info.Operation,
		)
	}
}

func TestAcquirePalStartOperationBlocksAMPCommand(
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

	server := managedServer{
		DisplayName: "Alamama",
		Instance: amp.Instance{
			Name: "AlamamaPal01",
		},
	}

	palRelease, activeOperation, acquired, err :=
		client.acquirePalStartOperation(
			server,
		)
	if err != nil {
		t.Fatalf(
			"/pal iniciar retornou erro: %v",
			err,
		)
	}

	if !acquired {
		t.Fatalf(
			"/pal iniciar deveria adquirir a instância; operação=%q",
			activeOperation,
		)
	}

	defer palRelease()

	ampLease, ampActive, err :=
		client.acquireAMPCommandOperation(
			"AlamamaPal01",
			ampCommandOperationStop,
		)
	if err != nil {
		t.Fatalf(
			"a tentativa do comando /amp retornou erro: %v",
			err,
		)
	}

	if ampLease != nil {
		ampLease.Release()

		t.Fatal(
			"/amp parar não deveria adquirir uma instância ocupada por /pal iniciar",
		)
	}

	if ampActive == nil {
		t.Fatal(
			"/amp deveria receber a operação que ocupa a instância",
		)
	}

	if ampActive.Operation != palStartOperationName {
		t.Fatalf(
			"operação concorrente inesperada: %q",
			ampActive.Operation,
		)
	}
}

func TestAcquirePalStartOperationIsBlockedByAMPCommand(
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

	ampOperation, err := manager.TryAcquire(
		"AlamamaPal01",
		"comando /amp reiniciar",
	)
	if err != nil {
		t.Fatalf(
			"não foi possível criar a operação AMP: %v",
			err,
		)
	}

	if !ampOperation.Acquired() {
		t.Fatal(
			"a operação AMP deveria adquirir a instância",
		)
	}

	defer ampOperation.Lease.Release()

	server := managedServer{
		DisplayName: "Alamama",
		Instance: amp.Instance{
			Name: "AlamamaPal01",
		},
	}

	release, activeOperation, acquired, err :=
		client.acquirePalStartOperation(
			server,
		)
	if err != nil {
		t.Fatalf(
			"/pal iniciar retornou erro: %v",
			err,
		)
	}

	if acquired {
		if release != nil {
			release()
		}

		t.Fatal(
			"/pal iniciar não deveria adquirir a instância",
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

func TestAcquirePalStartOperationAllowsDifferentServer(
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
		"monitor Idle antigo",
	)
	if err != nil {
		t.Fatalf(
			"não foi possível adquirir Alamama: %v",
			err,
		)
	}

	if !alamama.Acquired() {
		t.Fatal(
			"Alamama deveria estar adquirida",
		)
	}

	defer alamama.Lease.Release()

	kalaga := managedServer{
		DisplayName: "Kalaga",
		Instance: amp.Instance{
			Name: "KalagaPal01",
		},
	}

	release, activeOperation, acquired, err :=
		client.acquirePalStartOperation(
			kalaga,
		)
	if err != nil {
		t.Fatalf(
			"não foi possível adquirir Kalaga: %v",
			err,
		)
	}

	if !acquired {
		t.Fatalf(
			"Kalaga deveria ser independente; operação=%q",
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
