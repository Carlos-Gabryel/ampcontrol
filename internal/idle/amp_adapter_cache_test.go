package idle

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alabamaamp/ampcontrol/internal/amp"
)

func TestAMPAdapterCachesDiscoveryAcrossRuntimeStateCalls(
	t *testing.T,
) {
	t.Parallel()

	currentTime := time.Date(
		2026,
		time.August,
		7,
		3,
		50,
		0,
		0,
		time.UTC,
	)

	discoveryCalls := 0

	client := &fakeAMPApplicationClient{
		statuses: []amp.ApplicationStatus{
			{
				State: amp.ApplicationStateStopped,
			},
			{
				State: amp.ApplicationStateReady,
			},
		},
	}

	adapter, err := newAMPAdapter(
		client,
		func(
			context.Context,
		) ([]amp.ManagedInstance, error) {
			discoveryCalls++

			return []amp.ManagedInstance{
				{
					Name:    "AlamamaPal01",
					APIURL:  "http://127.0.0.1:8090/",
					Running: true,
				},
				{
					Name:    "KalagaPal01",
					APIURL:  "http://127.0.0.1:8088/",
					Running: true,
				},
			}, nil
		},
	)
	if err != nil {
		t.Fatalf(
			"não foi possível criar o adaptador: %v",
			err,
		)
	}

	adapter.cacheTTL = 5 * time.Second
	adapter.now = func() time.Time {
		return currentTime
	}

	alamamaState, err := adapter.RuntimeState(
		context.Background(),
		Server{
			Instance: "AlamamaPal01",
		},
	)
	if err != nil {
		t.Fatalf(
			"RuntimeState do Alamama retornou erro: %v",
			err,
		)
	}

	if alamamaState != RuntimeStateIdle {
		t.Fatalf(
			"estado inesperado do Alamama: %q",
			alamamaState,
		)
	}

	currentTime = currentTime.Add(
		1 * time.Second,
	)

	kalagaState, err := adapter.RuntimeState(
		context.Background(),
		Server{
			Instance: "KalagaPal01",
		},
	)
	if err != nil {
		t.Fatalf(
			"RuntimeState do Kalaga retornou erro: %v",
			err,
		)
	}

	if kalagaState != RuntimeStateOnline {
		t.Fatalf(
			"estado inesperado do Kalaga: %q",
			kalagaState,
		)
	}

	if discoveryCalls != 1 {
		t.Fatalf(
			"a descoberta deveria ser reutilizada no mesmo ciclo; chamadas: %d",
			discoveryCalls,
		)
	}
}

func TestAMPAdapterRefreshesDiscoveryAfterCacheExpires(
	t *testing.T,
) {
	t.Parallel()

	currentTime := time.Date(
		2026,
		time.August,
		7,
		3,
		50,
		0,
		0,
		time.UTC,
	)

	discoveryCalls := 0

	adapter, err := newAMPAdapter(
		&fakeAMPApplicationClient{},
		func(
			context.Context,
		) ([]amp.ManagedInstance, error) {
			discoveryCalls++

			return []amp.ManagedInstance{
				{
					Name:    "AlamamaPal01",
					APIURL:  "http://127.0.0.1:8090/",
					Running: false,
				},
			}, nil
		},
	)
	if err != nil {
		t.Fatalf(
			"não foi possível criar o adaptador: %v",
			err,
		)
	}

	adapter.cacheTTL = 5 * time.Second
	adapter.now = func() time.Time {
		return currentTime
	}

	_, err = adapter.RuntimeState(
		context.Background(),
		Server{
			Instance: "AlamamaPal01",
		},
	)
	if err != nil {
		t.Fatalf(
			"primeira consulta retornou erro: %v",
			err,
		)
	}

	currentTime = currentTime.Add(
		6 * time.Second,
	)

	_, err = adapter.RuntimeState(
		context.Background(),
		Server{
			Instance: "AlamamaPal01",
		},
	)
	if err != nil {
		t.Fatalf(
			"segunda consulta retornou erro: %v",
			err,
		)
	}

	if discoveryCalls != 2 {
		t.Fatalf(
			"o cache expirado deveria forçar nova descoberta; chamadas: %d",
			discoveryCalls,
		)
	}
}

func TestAMPAdapterDoesNotCacheDiscoveryErrors(
	t *testing.T,
) {
	t.Parallel()

	expectedErr := errors.New(
		"falha temporária simulada",
	)

	discoveryCalls := 0

	adapter, err := newAMPAdapter(
		&fakeAMPApplicationClient{},
		func(
			context.Context,
		) ([]amp.ManagedInstance, error) {
			discoveryCalls++

			if discoveryCalls == 1 {
				return nil, expectedErr
			}

			return []amp.ManagedInstance{
				{
					Name:    "AlamamaPal01",
					Running: false,
				},
			}, nil
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
			"a primeira descoberta deveria falhar",
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

	state, err := adapter.RuntimeState(
		context.Background(),
		Server{
			Instance: "AlamamaPal01",
		},
	)
	if err != nil {
		t.Fatalf(
			"a segunda descoberta deveria ser tentada novamente: %v",
			err,
		)
	}

	if state != RuntimeStateOffline {
		t.Fatalf(
			"estado inesperado após a recuperação: %q",
			state,
		)
	}

	if discoveryCalls != 2 {
		t.Fatalf(
			"erros de descoberta não devem ser armazenados em cache; chamadas: %d",
			discoveryCalls,
		)
	}
}

func TestAMPAdapterStopApplicationAlwaysRefreshesDiscovery(
	t *testing.T,
) {
	t.Parallel()

	currentTime := time.Date(
		2026,
		time.August,
		7,
		3,
		50,
		0,
		0,
		time.UTC,
	)

	discoveryCalls := 0

	client := &fakeAMPApplicationClient{
		statuses: []amp.ApplicationStatus{
			{
				State: amp.ApplicationStateReady,
			},
			{
				State: amp.ApplicationStateReady,
			},
		},
	}

	adapter, err := newAMPAdapter(
		client,
		func(
			context.Context,
		) ([]amp.ManagedInstance, error) {
			discoveryCalls++

			return []amp.ManagedInstance{
				{
					Name:    "AlamamaPal01",
					APIURL:  "http://127.0.0.1:8090/",
					Running: true,
				},
			}, nil
		},
	)
	if err != nil {
		t.Fatalf(
			"não foi possível criar o adaptador: %v",
			err,
		)
	}

	adapter.cacheTTL = 5 * time.Second
	adapter.now = func() time.Time {
		return currentTime
	}

	_, err = adapter.RuntimeState(
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

	currentTime = currentTime.Add(
		1 * time.Second,
	)

	err = adapter.StopApplication(
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

	if discoveryCalls != 2 {
		t.Fatalf(
			"StopApplication precisa ignorar o cache e redescobrir a instância; chamadas: %d",
			discoveryCalls,
		)
	}

	if len(client.stopCalls) != 1 {
		t.Fatalf(
			"Core.Stop deveria ter sido chamado uma vez; chamadas: %d",
			len(client.stopCalls),
		)
	}
}
