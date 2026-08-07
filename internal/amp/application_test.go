package amp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestStartApplicationUntilReadyRejectsEmptyURL(
	t *testing.T,
) {
	t.Parallel()

	client := NewAPIClient(
		"usuario",
		"senha",
	)

	err := client.StartApplicationUntilReady(
		context.Background(),
		"   ",
		time.Millisecond,
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

func TestRestartApplicationRejectsEmptyURL(
	t *testing.T,
) {
	t.Parallel()

	client := NewAPIClient(
		"usuario",
		"senha",
	)

	err := client.RestartApplication(
		context.Background(),
		"",
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

func TestStartApplicationUntilReadyHonorsCanceledContext(
	t *testing.T,
) {
	t.Parallel()

	client := NewAPIClient(
		"usuario",
		"senha",
	)

	ctx, cancel := context.WithCancel(
		context.Background(),
	)

	cancel()

	err := client.StartApplicationUntilReady(
		ctx,
		"http://127.0.0.1:65535",
		time.Millisecond,
	)

	if err == nil {
		t.Fatal(
			"era esperado um erro para contexto cancelado",
		)
	}

	if !strings.Contains(
		err.Error(),
		"context canceled",
	) {
		t.Fatalf(
			"erro inesperado: %v",
			err,
		)
	}
}

func TestStartApplicationUntilReadyWaitsForReady(
	t *testing.T,
) {
	t.Parallel()

	var startCalls atomic.Int32
	var statusCalls atomic.Int32

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
					writeApplicationTestLoginResponse(
						t,
						responseWriter,
					)

				case "/API/Core/Start":
					startCalls.Add(1)

					writeApplicationTestJSON(
						t,
						responseWriter,
						actionResponse{
							Status: true,
						},
					)

				case "/API/Core/GetStatus":
					call := statusCalls.Add(1)

					state := ApplicationStateStarting

					if call == 2 {
						state = ApplicationStateConfiguring
					}

					if call >= 3 {
						state = ApplicationStateReady
					}

					writeApplicationTestJSON(
						t,
						responseWriter,
						ApplicationStatus{
							State: state,
						},
					)

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

	ctx, cancel := context.WithTimeout(
		context.Background(),
		time.Second,
	)
	defer cancel()

	err := client.StartApplicationUntilReady(
		ctx,
		server.URL,
		5*time.Millisecond,
	)
	if err != nil {
		t.Fatalf(
			"StartApplicationUntilReady retornou erro: %v",
			err,
		)
	}

	if startCalls.Load() != 1 {
		t.Fatalf(
			"Core.Start deveria ser chamado apenas uma vez depois de aceito; chamadas: %d",
			startCalls.Load(),
		)
	}

	if statusCalls.Load() != 3 {
		t.Fatalf(
			"Core.GetStatus deveria aguardar até Ready; chamadas: %d",
			statusCalls.Load(),
		)
	}
}

func TestStartApplicationUntilReadyRetriesStartBeforeWaitingForReady(
	t *testing.T,
) {
	t.Parallel()

	var startCalls atomic.Int32
	var statusCalls atomic.Int32

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
					writeApplicationTestLoginResponse(
						t,
						responseWriter,
					)

				case "/API/Core/Start":
					call := startCalls.Add(1)

					if call == 1 {
						reason := "aplicação ainda indisponível"

						writeApplicationTestJSON(
							t,
							responseWriter,
							actionResponse{
								Status: false,
								Reason: &reason,
							},
						)

						return
					}

					writeApplicationTestJSON(
						t,
						responseWriter,
						actionResponse{
							Status: true,
						},
					)

				case "/API/Core/GetStatus":
					statusCalls.Add(1)

					writeApplicationTestJSON(
						t,
						responseWriter,
						ApplicationStatus{
							State: ApplicationStateReady,
						},
					)

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

	ctx, cancel := context.WithTimeout(
		context.Background(),
		time.Second,
	)
	defer cancel()

	err := client.StartApplicationUntilReady(
		ctx,
		server.URL,
		5*time.Millisecond,
	)
	if err != nil {
		t.Fatalf(
			"StartApplicationUntilReady retornou erro: %v",
			err,
		)
	}

	if startCalls.Load() != 2 {
		t.Fatalf(
			"Core.Start deveria ser repetido até ser aceito; chamadas: %d",
			startCalls.Load(),
		)
	}

	if statusCalls.Load() != 1 {
		t.Fatalf(
			"Core.GetStatus só deveria ser consultado depois de Core.Start ser aceito; chamadas: %d",
			statusCalls.Load(),
		)
	}
}

func TestStartApplicationUntilReadyStopsRetryingStartAfterAcceptance(
	t *testing.T,
) {
	t.Parallel()

	var startCalls atomic.Int32
	var statusCalls atomic.Int32

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
					writeApplicationTestLoginResponse(
						t,
						responseWriter,
					)

				case "/API/Core/Start":
					startCalls.Add(1)

					writeApplicationTestJSON(
						t,
						responseWriter,
						actionResponse{
							Status: true,
						},
					)

				case "/API/Core/GetStatus":
					call := statusCalls.Add(1)

					state := ApplicationStateStarting

					if call >= 4 {
						state = ApplicationStateReady
					}

					writeApplicationTestJSON(
						t,
						responseWriter,
						ApplicationStatus{
							State: state,
						},
					)

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

	ctx, cancel := context.WithTimeout(
		context.Background(),
		time.Second,
	)
	defer cancel()

	err := client.StartApplicationUntilReady(
		ctx,
		server.URL,
		5*time.Millisecond,
	)
	if err != nil {
		t.Fatalf(
			"StartApplicationUntilReady retornou erro: %v",
			err,
		)
	}

	if startCalls.Load() != 1 {
		t.Fatalf(
			"Core.Start não deve ser repetido enquanto a aplicação apenas aguarda Ready; chamadas: %d",
			startCalls.Load(),
		)
	}

	if statusCalls.Load() != 4 {
		t.Fatalf(
			"quantidade inesperada de consultas a Core.GetStatus: %d",
			statusCalls.Load(),
		)
	}
}

func TestStartApplicationUntilReadyRejectsFailedState(
	t *testing.T,
) {
	t.Parallel()

	var startCalls atomic.Int32
	var statusCalls atomic.Int32

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
					writeApplicationTestLoginResponse(
						t,
						responseWriter,
					)

				case "/API/Core/Start":
					startCalls.Add(1)

					writeApplicationTestJSON(
						t,
						responseWriter,
						actionResponse{
							Status: true,
						},
					)

				case "/API/Core/GetStatus":
					statusCalls.Add(1)

					writeApplicationTestJSON(
						t,
						responseWriter,
						ApplicationStatus{
							State: ApplicationStateFailed,
						},
					)

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

	ctx, cancel := context.WithTimeout(
		context.Background(),
		time.Second,
	)
	defer cancel()

	err := client.StartApplicationUntilReady(
		ctx,
		server.URL,
		5*time.Millisecond,
	)

	if err == nil {
		t.Fatal(
			"era esperado erro quando a aplicação entra em Failed",
		)
	}

	if !strings.Contains(
		err.Error(),
		"Failed",
	) {
		t.Fatalf(
			"o erro deveria informar o estado Failed: %v",
			err,
		)
	}

	if startCalls.Load() != 1 {
		t.Fatalf(
			"Core.Start deveria ter sido chamado uma vez; chamadas: %d",
			startCalls.Load(),
		)
	}

	if statusCalls.Load() != 1 {
		t.Fatalf(
			"Core.GetStatus deveria identificar Failed imediatamente; chamadas: %d",
			statusCalls.Load(),
		)
	}
}

func TestStartApplicationUntilReadyRejectsSuspendedState(
	t *testing.T,
) {
	t.Parallel()

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
					writeApplicationTestLoginResponse(
						t,
						responseWriter,
					)

				case "/API/Core/Start":
					writeApplicationTestJSON(
						t,
						responseWriter,
						actionResponse{
							Status: true,
						},
					)

				case "/API/Core/GetStatus":
					writeApplicationTestJSON(
						t,
						responseWriter,
						ApplicationStatus{
							State: ApplicationStateSuspended,
						},
					)

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

	ctx, cancel := context.WithTimeout(
		context.Background(),
		time.Second,
	)
	defer cancel()

	err := client.StartApplicationUntilReady(
		ctx,
		server.URL,
		5*time.Millisecond,
	)

	if err == nil {
		t.Fatal(
			"era esperado erro quando a aplicação entra em Suspended",
		)
	}

	if !strings.Contains(
		err.Error(),
		"Suspended",
	) {
		t.Fatalf(
			"o erro deveria informar o estado Suspended: %v",
			err,
		)
	}
}

func TestBuildApplicationRetryErrorIncludesLastError(
	t *testing.T,
) {
	t.Parallel()

	lastErr := context.DeadlineExceeded
	contextErr := context.Canceled

	err := buildApplicationRetryError(
		"http://127.0.0.1:8080",
		lastErr,
		contextErr,
	)

	if err == nil {
		t.Fatal(
			"era esperado um erro",
		)
	}

	if !strings.Contains(
		err.Error(),
		"último erro",
	) {
		t.Fatalf(
			"o erro não contém o último erro registrado: %v",
			err,
		)
	}

	if !strings.Contains(
		err.Error(),
		"context canceled",
	) {
		t.Fatalf(
			"o erro não contém o cancelamento do contexto: %v",
			err,
		)
	}
}

func writeApplicationTestLoginResponse(
	t *testing.T,
	responseWriter http.ResponseWriter,
) {
	t.Helper()

	writeApplicationTestJSON(
		t,
		responseWriter,
		loginResponse{
			Result:       0,
			ResultReason: "",
			Success:      true,
			Permissions:  []string{},
			SessionID:    "sessao-de-teste",
		},
	)
}

func writeApplicationTestJSON(
	t *testing.T,
	responseWriter http.ResponseWriter,
	value any,
) {
	t.Helper()

	if err := json.NewEncoder(
		responseWriter,
	).Encode(value); err != nil {
		t.Errorf(
			"não foi possível responder ao cliente de teste: %v",
			err,
		)
	}
}
