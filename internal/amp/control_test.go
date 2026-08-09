package amp

import (
	"context"
	"strings"
	"testing"
)

func TestIsValidInstanceOperation(
	t *testing.T,
) {
	t.Parallel()

	tests := []struct {
		name      string
		operation InstanceOperation
		expected  bool
	}{
		{
			name:      "iniciar",
			operation: InstanceOperationStart,
			expected:  true,
		},
		{
			name:      "parar",
			operation: InstanceOperationStop,
			expected:  true,
		},
		{
			name:      "reiniciar",
			operation: InstanceOperationRestart,
			expected:  true,
		},
		{
			name:      "atualizar",
			operation: InstanceOperationUpdate,
			expected:  true,
		},
		{
			name:      "operação desconhecida",
			operation: InstanceOperation("delete"),
			expected:  false,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(
			test.name,
			func(t *testing.T) {
				t.Parallel()

				result := isValidInstanceOperation(
					test.operation,
				)

				if result != test.expected {
					t.Fatalf(
						"resultado inesperado: obtido %t, esperado %t",
						result,
						test.expected,
					)
				}
			},
		)
	}
}

func TestControlInstanceRejectsEmptyName(
	t *testing.T,
) {
	t.Parallel()

	err := ControlInstance(
		context.Background(),
		InstanceOperationStart,
		"   ",
	)

	if err == nil {
		t.Fatal(
			"era esperado um erro para nome vazio",
		)
	}

	if !strings.Contains(
		err.Error(),
		"nome da instância não foi informado",
	) {
		t.Fatalf(
			"erro inesperado: %v",
			err,
		)
	}
}

func TestControlInstanceRejectsInvalidOperation(
	t *testing.T,
) {
	t.Parallel()

	err := ControlInstance(
		context.Background(),
		InstanceOperation("delete"),
		"Valheim01",
	)

	if err == nil {
		t.Fatal(
			"era esperado um erro para operação inválida",
		)
	}

	if !strings.Contains(
		err.Error(),
		"operação AMP inválida",
	) {
		t.Fatalf(
			"erro inesperado: %v",
			err,
		)
	}
}

func TestControlInstanceProtectsADS(
	t *testing.T,
) {
	t.Parallel()

	err := StopInstance(
		context.Background(),
		"ADS01",
	)

	if err == nil {
		t.Fatal(
			"era esperado que o ADS01 fosse protegido",
		)
	}

	if !strings.Contains(
		err.Error(),
		"ADS01 é protegida",
	) {
		t.Fatalf(
			"erro inesperado: %v",
			err,
		)
	}
}

func TestControlInstanceRejectsUnknownInstance(
	t *testing.T,
) {
	t.Parallel()

	err := RestartInstance(
		context.Background(),
		"ServidorQueNaoExiste",
	)

	if err == nil {
		t.Fatal(
			"era esperado erro para instância inexistente",
		)
	}

	if !strings.Contains(
		err.Error(),
		"não existe",
	) {
		t.Fatalf(
			"erro inesperado: %v",
			err,
		)
	}
}
