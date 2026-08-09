package idle

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/alabamaamp/ampcontrol/internal/amp"
)

type fakeAMPApplicationClient struct {
	statuses     []amp.ApplicationStatus
	statusErrors []error
	stopError    error
	statusCalls  []string
	stopCalls    []string
}

func (c *fakeAMPApplicationClient) GetApplicationStatus(
	_ context.Context,
	baseURL string,
) (amp.ApplicationStatus, error) {
	callIndex := len(
		c.statusCalls,
	)

	c.statusCalls = append(
		c.statusCalls,
		baseURL,
	)

	if callIndex < len(c.statusErrors) &&
		c.statusErrors[callIndex] != nil {
		return amp.ApplicationStatus{},
			c.statusErrors[callIndex]
	}

	if callIndex < len(c.statuses) {
		return c.statuses[callIndex], nil
	}

	return amp.ApplicationStatus{
		State: amp.ApplicationStateReady,
	}, nil
}

func (c *fakeAMPApplicationClient) StopApplication(
	_ context.Context,
	baseURL string,
) error {
	c.stopCalls = append(
		c.stopCalls,
		baseURL,
	)

	return c.stopError
}

func TestNewAMPAdapterRejectsNilClient(
	t *testing.T,
) {
	t.Parallel()

	_, err := NewAMPAdapter(
		nil,
	)

	if err == nil {
		t.Fatal(
			"era esperado um erro para cliente AMP nulo",
		)
	}

	if !strings.Contains(
		err.Error(),
		"cliente da API AMP",
	) {
		t.Fatalf(
			"erro inesperado: %v",
			err,
		)
	}
}

func TestNewAMPAdapterRejectsNilDiscovery(
	t *testing.T,
) {
	t.Parallel()

	_, err := newAMPAdapter(
		&fakeAMPApplicationClient{},
		nil,
	)

	if err == nil {
		t.Fatal(
			"era esperado um erro para descoberta nula",
		)
	}

	if !strings.Contains(
		err.Error(),
		"função de descoberta",
	) {
		t.Fatalf(
			"erro inesperado: %v",
			err,
		)
	}
}

func TestAMPAdapterReportsOfflineWithoutCallingAPI(
	t *testing.T,
) {
	t.Parallel()

	client := &fakeAMPApplicationClient{}

	adapter := newAMPAdapterForTest(
		t,
		client,
		[]amp.ManagedInstance{
			{
				Name:    "AlamamaPal01",
				APIURL:  "http://127.0.0.1:8090/",
				Running: false,
			},
		},
	)

	state, err := adapter.RuntimeState(
		context.Background(),
		Server{
			Instance: "AlamamaPal01",
		},
	)
	if err != nil {
		t.Fatalf(
			"RuntimeState retornou erro: %v",
			err,
		)
	}

	if state != RuntimeStateOffline {
		t.Fatalf(
			"estado inesperado: obtido %q, esperado %q",
			state,
			RuntimeStateOffline,
		)
	}

	if len(client.statusCalls) != 0 {
		t.Fatalf(
			"a API não deveria ser consultada para instância Offline; chamadas: %d",
			len(client.statusCalls),
		)
	}
}

func TestAMPAdapterMapsApplicationStates(
	t *testing.T,
) {
	t.Parallel()

	tests := []struct {
		name             string
		applicationState amp.ApplicationState
		expected         RuntimeState
	}{
		{
			name:             "idle",
			applicationState: amp.ApplicationStateStopped,
			expected:         RuntimeStateIdle,
		},
		{
			name:             "online",
			applicationState: amp.ApplicationStateReady,
			expected:         RuntimeStateOnline,
		},
		{
			name:             "iniciando",
			applicationState: amp.ApplicationStateStarting,
			expected:         RuntimeStateBusy,
		},
		{
			name:             "atualizando",
			applicationState: amp.ApplicationStateUpdating,
			expected:         RuntimeStateBusy,
		},
		{
			name:             "falha",
			applicationState: amp.ApplicationStateFailed,
			expected:         RuntimeStateFailed,
		},
		{
			name:             "suspensa",
			applicationState: amp.ApplicationStateSuspended,
			expected:         RuntimeStateFailed,
		},
		{
			name:             "desconhecida",
			applicationState: amp.ApplicationState(54321),
			expected:         RuntimeStateUnknown,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(
			test.name,
			func(t *testing.T) {
				t.Parallel()

				client := &fakeAMPApplicationClient{
					statuses: []amp.ApplicationStatus{
						{
							State: test.applicationState,
						},
					},
				}

				adapter := newAMPAdapterForTest(
					t,
					client,
					[]amp.ManagedInstance{
						{
							Name:    "ServidorTeste01",
							APIURL:  "http://127.0.0.1:9000/",
							Running: true,
						},
					},
				)

				state, err := adapter.RuntimeState(
					context.Background(),
					Server{
						Instance: "ServidorTeste01",
					},
				)
				if err != nil {
					t.Fatalf(
						"RuntimeState retornou erro: %v",
						err,
					)
				}

				if state != test.expected {
					t.Fatalf(
						"estado inesperado: obtido %q, esperado %q",
						state,
						test.expected,
					)
				}
			},
		)
	}
}

func TestAMPAdapterWrapsDiscoveryError(
	t *testing.T,
) {
	t.Parallel()

	expectedErr := errors.New(
		"falha simulada no ampinstmgr",
	)

	adapter, err := newAMPAdapter(
		&fakeAMPApplicationClient{},
		func(
			context.Context,
		) ([]amp.ManagedInstance, error) {
			return nil, expectedErr
		},
	)
	if err != nil {
		t.Fatalf(
			"não foi possível criar o adaptador: %v",
			err,
		)
	}

	_, err = adapter.RuntimeState(
		context.Background(),
		Server{
			Instance: "AlamamaPal01",
		},
	)

	if err == nil {
		t.Fatal(
			"era esperado um erro de descoberta",
		)
	}

	if !errors.Is(
		err,
		expectedErr,
	) {
		t.Fatalf(
			"o erro original não foi preservado: %v",
			err,
		)
	}
}

func TestAMPAdapterRejectsUnknownInstance(
	t *testing.T,
) {
	t.Parallel()

	adapter := newAMPAdapterForTest(
		t,
		&fakeAMPApplicationClient{},
		[]amp.ManagedInstance{
			{
				Name:    "KalagaPal01",
				APIURL:  "http://127.0.0.1:8088/",
				Running: true,
			},
		},
	)

	_, err := adapter.RuntimeState(
		context.Background(),
		Server{
			Instance: "InstanciaInexistente01",
		},
	)

	if err == nil {
		t.Fatal(
			"era esperado um erro para instância inexistente",
		)
	}

	if !strings.Contains(
		err.Error(),
		"não foi encontrada",
	) {
		t.Fatalf(
			"erro inesperado: %v",
			err,
		)
	}
}

func TestAMPAdapterRejectsRunningInstanceWithoutAPIURL(
	t *testing.T,
) {
	t.Parallel()

	adapter := newAMPAdapterForTest(
		t,
		&fakeAMPApplicationClient{},
		[]amp.ManagedInstance{
			{
				Name:    "AlamamaPal01",
				Running: true,
			},
		},
	)

	_, err := adapter.RuntimeState(
		context.Background(),
		Server{
			Instance: "AlamamaPal01",
		},
	)

	if err == nil {
		t.Fatal(
			"era esperado um erro para URL vazia",
		)
	}

	if !strings.Contains(
		err.Error(),
		"não possui URL de API",
	) {
		t.Fatalf(
			"erro inesperado: %v",
			err,
		)
	}
}

func TestAMPAdapterStopApplicationCallsCoreStopWhenOnline(
	t *testing.T,
) {
	t.Parallel()

	client := &fakeAMPApplicationClient{
		statuses: []amp.ApplicationStatus{
			{
				State: amp.ApplicationStateReady,
			},
		},
	}

	adapter := newAMPAdapterForTest(
		t,
		client,
		[]amp.ManagedInstance{
			{
				Name:    "AlamamaPal01",
				APIURL:  "http://127.0.0.1:8090/",
				Running: true,
			},
		},
	)

	err := adapter.StopApplication(
		context.Background(),
		Server{
			Instance: "AlamamaPal01",
		},
	)
	if err != nil {
		t.Fatalf(
			"StopApplication retornou erro: %v",
			err,
		)
	}

	if len(client.statusCalls) != 1 {
		t.Fatalf(
			"quantidade inesperada de consultas de estado: %d",
			len(client.statusCalls),
		)
	}

	if len(client.stopCalls) != 1 {
		t.Fatalf(
			"Core.Stop deveria ter sido chamado uma vez, mas foi chamado %d vezes",
			len(client.stopCalls),
		)
	}

	if client.stopCalls[0] != "http://127.0.0.1:8090/" {
		t.Fatalf(
			"URL inesperada em Core.Stop: %q",
			client.stopCalls[0],
		)
	}
}

func TestAMPAdapterStopApplicationAcceptsAlreadyIdle(
	t *testing.T,
) {
	t.Parallel()

	client := &fakeAMPApplicationClient{
		statuses: []amp.ApplicationStatus{
			{
				State: amp.ApplicationStateStopped,
			},
		},
	}

	adapter := newAMPAdapterForTest(
		t,
		client,
		[]amp.ManagedInstance{
			{
				Name:    "AlamamaPal01",
				APIURL:  "http://127.0.0.1:8090/",
				Running: true,
			},
		},
	)

	err := adapter.StopApplication(
		context.Background(),
		Server{
			Instance: "AlamamaPal01",
		},
	)
	if err != nil {
		t.Fatalf(
			"uma aplicação já em Idle deveria ser aceita: %v",
			err,
		)
	}

	if len(client.stopCalls) != 0 {
		t.Fatalf(
			"Core.Stop não deveria ser repetido quando a aplicação já está Idle; chamadas: %d",
			len(client.stopCalls),
		)
	}
}

func TestAMPAdapterStopApplicationRejectsBusyState(
	t *testing.T,
) {
	t.Parallel()

	client := &fakeAMPApplicationClient{
		statuses: []amp.ApplicationStatus{
			{
				State: amp.ApplicationStateStarting,
			},
		},
	}

	adapter := newAMPAdapterForTest(
		t,
		client,
		[]amp.ManagedInstance{
			{
				Name:    "AlamamaPal01",
				APIURL:  "http://127.0.0.1:8090/",
				Running: true,
			},
		},
	)

	err := adapter.StopApplication(
		context.Background(),
		Server{
			Instance: "AlamamaPal01",
		},
	)

	if err == nil {
		t.Fatal(
			"era esperado um erro para aplicação em transição",
		)
	}

	if !strings.Contains(
		err.Error(),
		"entrou em transição",
	) {
		t.Fatalf(
			"erro inesperado: %v",
			err,
		)
	}

	if len(client.stopCalls) != 0 {
		t.Fatalf(
			"Core.Stop não deveria ser chamado durante uma transição; chamadas: %d",
			len(client.stopCalls),
		)
	}
}

func TestAMPAdapterStopApplicationRejectsOfflineInstance(
	t *testing.T,
) {
	t.Parallel()

	client := &fakeAMPApplicationClient{}

	adapter := newAMPAdapterForTest(
		t,
		client,
		[]amp.ManagedInstance{
			{
				Name:    "AlamamaPal01",
				APIURL:  "http://127.0.0.1:8090/",
				Running: false,
			},
		},
	)

	err := adapter.StopApplication(
		context.Background(),
		Server{
			Instance: "AlamamaPal01",
		},
	)

	if err == nil {
		t.Fatal(
			"era esperado um erro para instância Offline",
		)
	}

	if !strings.Contains(
		err.Error(),
		"ficou Offline",
	) {
		t.Fatalf(
			"erro inesperado: %v",
			err,
		)
	}

	if len(client.statusCalls) != 0 {
		t.Fatalf(
			"a API não deveria ser consultada para uma instância Offline; chamadas: %d",
			len(client.statusCalls),
		)
	}

	if len(client.stopCalls) != 0 {
		t.Fatalf(
			"Core.Stop não deveria ser chamado para uma instância Offline; chamadas: %d",
			len(client.stopCalls),
		)
	}
}

func TestAMPAdapterStopApplicationWrapsCoreStopError(
	t *testing.T,
) {
	t.Parallel()

	expectedErr := errors.New(
		"falha simulada em Core.Stop",
	)

	client := &fakeAMPApplicationClient{
		statuses: []amp.ApplicationStatus{
			{
				State: amp.ApplicationStateReady,
			},
		},
		stopError: expectedErr,
	}

	adapter := newAMPAdapterForTest(
		t,
		client,
		[]amp.ManagedInstance{
			{
				Name:    "KalagaPal01",
				APIURL:  "http://127.0.0.1:8088/",
				Running: true,
			},
		},
	)

	err := adapter.StopApplication(
		context.Background(),
		Server{
			Instance: "KalagaPal01",
		},
	)

	if err == nil {
		t.Fatal(
			"era esperado um erro de Core.Stop",
		)
	}

	if !errors.Is(
		err,
		expectedErr,
	) {
		t.Fatalf(
			"o erro original de Core.Stop não foi preservado: %v",
			err,
		)
	}
}

func newAMPAdapterForTest(
	t *testing.T,
	client AMPApplicationClient,
	instances []amp.ManagedInstance,
) *AMPAdapter {
	t.Helper()

	adapter, err := newAMPAdapter(
		client,
		func(
			context.Context,
		) ([]amp.ManagedInstance, error) {
			result := make(
				[]amp.ManagedInstance,
				len(instances),
			)

			copy(
				result,
				instances,
			)

			return result, nil
		},
	)
	if err != nil {
		t.Fatalf(
			"não foi possível criar o adaptador AMP de teste: %v",
			err,
		)
	}

	return adapter
}
