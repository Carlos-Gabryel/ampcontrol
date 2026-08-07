package amp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetApplicationStatusRejectsEmptyURL(
	t *testing.T,
) {
	t.Parallel()

	client := NewAPIClient(
		"usuario",
		"senha",
	)

	_, err := client.GetApplicationStatus(
		context.Background(),
		"   ",
	)

	if err == nil {
		t.Fatal(
			"era esperado um erro para URL vazia",
		)
	}

	if !strings.Contains(
		err.Error(),
		"URL da instância AMP não foi informada",
	) {
		t.Fatalf(
			"erro inesperado: %v",
			err,
		)
	}
}

func TestGetApplicationStatus(
	t *testing.T,
) {
	t.Parallel()

	const sessionID = "sessao-de-teste"

	server := httptest.NewServer(
		http.HandlerFunc(
			func(
				responseWriter http.ResponseWriter,
				request *http.Request,
			) {
				responseWriter.Header().Set(
					"Content-Type",
					"application/json",
				)

				switch request.URL.Path {
				case "/API/Core/Login":
					loginResult := loginResponse{
						Result:       0,
						ResultReason: "",
						Success:      true,
						Permissions:  []string{},
						SessionID:    sessionID,
					}

					if err := json.NewEncoder(
						responseWriter,
					).Encode(loginResult); err != nil {
						t.Errorf(
							"não foi possível responder ao login: %v",
							err,
						)
					}

				case "/API/Core/GetStatus":
					expectedAuthorization := "Bearer " + sessionID

					if request.Header.Get(
						"Authorization",
					) != expectedAuthorization {
						t.Errorf(
							"Authorization inesperado: %q",
							request.Header.Get("Authorization"),
						)
					}

					var requestBody authenticatedRequest

					if err := json.NewDecoder(
						request.Body,
					).Decode(&requestBody); err != nil {
						t.Errorf(
							"corpo inválido em GetStatus: %v",
							err,
						)
					}

					if requestBody.SessionID != sessionID {
						t.Errorf(
							"SESSIONID inesperada: %q",
							requestBody.SessionID,
						)
					}

					status := ApplicationStatus{
						State:  ApplicationStateReady,
						Uptime: "00:10:00",
					}

					if err := json.NewEncoder(
						responseWriter,
					).Encode(status); err != nil {
						t.Errorf(
							"não foi possível responder ao GetStatus: %v",
							err,
						)
					}

				default:
					http.NotFound(
						responseWriter,
						request,
					)
				}
			},
		),
	)
	defer server.Close()

	client := NewAPIClient(
		"usuario",
		"senha",
	)

	status, err := client.GetApplicationStatus(
		context.Background(),
		server.URL,
	)
	if err != nil {
		t.Fatalf(
			"GetApplicationStatus retornou erro: %v",
			err,
		)
	}

	if status.State != ApplicationStateReady {
		t.Fatalf(
			"estado inesperado: obtido %s, esperado Ready",
			status.State.String(),
		)
	}

	if status.Phase() != ApplicationPhaseOnline {
		t.Fatalf(
			"fase inesperada: obtida %q, esperada %q",
			status.Phase(),
			ApplicationPhaseOnline,
		)
	}

	if status.Uptime != "00:10:00" {
		t.Fatalf(
			"uptime inesperado: %q",
			status.Uptime,
		)
	}
}

func TestApplicationStatePhase(
	t *testing.T,
) {
	t.Parallel()

	tests := []struct {
		name     string
		state    ApplicationState
		expected ApplicationPhase
	}{
		{
			name:     "parada",
			state:    ApplicationStateStopped,
			expected: ApplicationPhaseIdle,
		},
		{
			name:     "dormindo",
			state:    ApplicationStateSleeping,
			expected: ApplicationPhaseIdle,
		},
		{
			name:     "pronta",
			state:    ApplicationStateReady,
			expected: ApplicationPhaseOnline,
		},
		{
			name:     "iniciando",
			state:    ApplicationStateStarting,
			expected: ApplicationPhaseBusy,
		},
		{
			name:     "reiniciando",
			state:    ApplicationStateRestarting,
			expected: ApplicationPhaseBusy,
		},
		{
			name:     "parando",
			state:    ApplicationStateStopping,
			expected: ApplicationPhaseBusy,
		},
		{
			name:     "atualizando",
			state:    ApplicationStateUpdating,
			expected: ApplicationPhaseBusy,
		},
		{
			name:     "falhou",
			state:    ApplicationStateFailed,
			expected: ApplicationPhaseFailed,
		},
		{
			name:     "suspensa",
			state:    ApplicationStateSuspended,
			expected: ApplicationPhaseSuspended,
		},
		{
			name:     "desconhecida",
			state:    ApplicationState(12345),
			expected: ApplicationPhaseUnknown,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(
			test.name,
			func(t *testing.T) {
				t.Parallel()

				result := test.state.Phase()

				if result != test.expected {
					t.Fatalf(
						"fase inesperada: obtida %q, esperada %q",
						result,
						test.expected,
					)
				}
			},
		)
	}
}

func TestApplicationStateString(
	t *testing.T,
) {
	t.Parallel()

	if ApplicationStateReady.String() != "Ready" {
		t.Fatalf(
			"nome inesperado para Ready: %q",
			ApplicationStateReady.String(),
		)
	}

	unknown := ApplicationState(54321).String()

	if unknown != "Unknown(54321)" {
		t.Fatalf(
			"nome inesperado para estado desconhecido: %q",
			unknown,
		)
	}
}
