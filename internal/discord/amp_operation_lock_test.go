package discord

import (
	"strings"
	"testing"

	"github.com/alabamaamp/palcontrol/internal/operation"
)

func TestAcquireAMPCommandOperationRejectsClientWithoutManager(
	t *testing.T,
) {
	t.Parallel()

	client := &Client{}

	defer clearOperationManager(
		client,
	)

	lease, active, err := client.acquireAMPCommandOperation(
		"AlamamaPal01",
		ampCommandOperationStart,
	)

	if err == nil {
		t.Fatal(
			"era esperado um erro para cliente sem gerenciador",
		)
	}

	if lease != nil {
		t.Fatal(
			"nenhum Lease deveria ser criado",
		)
	}

	if active != nil {
		t.Fatal(
			"nenhuma operação ativa deveria ser retornada",
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

func TestAcquireAMPCommandOperationAcquiresInstance(
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

	lease, active, err := client.acquireAMPCommandOperation(
		"AlamamaPal01",
		ampCommandOperationRestart,
	)
	if err != nil {
		t.Fatalf(
			"a aquisição retornou erro: %v",
			err,
		)
	}

	if lease == nil {
		t.Fatal(
			"o Lease deveria ter sido adquirido",
		)
	}

	defer lease.Release()

	if active != nil {
		t.Fatalf(
			"nenhuma operação concorrente era esperada: %#v",
			active,
		)
	}

	info, exists := manager.Current(
		"AlamamaPal01",
	)

	if !exists {
		t.Fatal(
			"a instância deveria possuir uma operação ativa",
		)
	}

	if info.Operation != "comando /amp reiniciar" {
		t.Fatalf(
			"descrição inesperada: %q",
			info.Operation,
		)
	}
}

func TestAcquireAMPCommandOperationBlocksSameInstance(
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

	firstLease, firstActive, err :=
		client.acquireAMPCommandOperation(
			"AlamamaPal01",
			ampCommandOperationRestart,
		)
	if err != nil {
		t.Fatalf(
			"a primeira aquisição retornou erro: %v",
			err,
		)
	}

	if firstLease == nil {
		t.Fatal(
			"a primeira operação deveria adquirir a instância",
		)
	}

	defer firstLease.Release()

	if firstActive != nil {
		t.Fatalf(
			"a primeira operação não deveria encontrar concorrência: %#v",
			firstActive,
		)
	}

	secondLease, secondActive, err :=
		client.acquireAMPCommandOperation(
			"alamamapal01",
			ampCommandOperationStop,
		)
	if err != nil {
		t.Fatalf(
			"a segunda aquisição retornou erro: %v",
			err,
		)
	}

	if secondLease != nil {
		secondLease.Release()

		t.Fatal(
			"a mesma instância não deveria aceitar a segunda operação",
		)
	}

	if secondActive == nil {
		t.Fatal(
			"a operação que ocupa a instância deveria ser informada",
		)
	}

	if secondActive.Operation != "comando /amp reiniciar" {
		t.Fatalf(
			"operação ativa inesperada: %q",
			secondActive.Operation,
		)
	}
}

func TestAcquireAMPCommandOperationAllowsDifferentInstances(
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

	alamamaLease, _, err :=
		client.acquireAMPCommandOperation(
			"AlamamaPal01",
			ampCommandOperationUpdate,
		)
	if err != nil {
		t.Fatalf(
			"não foi possível reservar Alamama: %v",
			err,
		)
	}

	if alamamaLease == nil {
		t.Fatal(
			"Alamama deveria ter sido reservada",
		)
	}

	defer alamamaLease.Release()

	kalagaLease, kalagaActive, err :=
		client.acquireAMPCommandOperation(
			"KalagaPal01",
			ampCommandOperationStop,
		)
	if err != nil {
		t.Fatalf(
			"não foi possível reservar Kalaga: %v",
			err,
		)
	}

	if kalagaLease == nil {
		t.Fatal(
			"Kalaga deveria poder executar uma operação simultaneamente",
		)
	}

	defer kalagaLease.Release()

	if kalagaActive != nil {
		t.Fatalf(
			"Kalaga não deveria encontrar operação concorrente: %#v",
			kalagaActive,
		)
	}
}

func TestAcquireAMPCommandOperationReleasesInstance(
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

	firstLease, _, err :=
		client.acquireAMPCommandOperation(
			"AlamamaPal01",
			ampCommandOperationStart,
		)
	if err != nil {
		t.Fatalf(
			"não foi possível adquirir a primeira operação: %v",
			err,
		)
	}

	if firstLease == nil {
		t.Fatal(
			"a primeira operação deveria adquirir a instância",
		)
	}

	firstLease.Release()

	secondLease, active, err :=
		client.acquireAMPCommandOperation(
			"AlamamaPal01",
			ampCommandOperationStop,
		)
	if err != nil {
		t.Fatalf(
			"não foi possível adquirir depois da liberação: %v",
			err,
		)
	}

	if secondLease == nil {
		t.Fatalf(
			"a instância deveria estar livre; operação encontrada: %#v",
			active,
		)
	}

	secondLease.Release()
}

func TestAMPCommandOperationDescription(
	t *testing.T,
) {
	t.Parallel()

	tests := []struct {
		name      string
		operation ampCommandOperation
		expected  string
	}{
		{
			name:      "iniciar",
			operation: ampCommandOperationStart,
			expected:  "comando /amp iniciar",
		},
		{
			name:      "parar",
			operation: ampCommandOperationStop,
			expected:  "comando /amp parar",
		},
		{
			name:      "reiniciar",
			operation: ampCommandOperationRestart,
			expected:  "comando /amp reiniciar",
		},
		{
			name:      "desligar",
			operation: ampCommandOperationShutdown,
			expected:  "comando /amp desligar",
		},
		{
			name:      "atualizar",
			operation: ampCommandOperationUpdate,
			expected:  "comando /amp atualizar",
		},
	}

	for _, test := range tests {
		test := test

		t.Run(
			test.name,
			func(t *testing.T) {
				t.Parallel()

				actual := ampCommandOperationDescription(
					test.operation,
				)

				if actual != test.expected {
					t.Fatalf(
						"descrição inesperada: obtido %q, esperado %q",
						actual,
						test.expected,
					)
				}
			},
		)
	}
}
