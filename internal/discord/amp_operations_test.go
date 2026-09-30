package discord

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Carlos-Gabryel/ampcontrol/internal/amp"
)

// fakeAMPAPI simula a API do AMP e registra as chamadas por caminho.
type fakeAMPAPI struct {
	server *httptest.Server

	mutex        sync.Mutex
	calls        map[string]int
	state        int
	statusFailed bool
}

func newFakeAMPAPI(
	t *testing.T,
	initialState int,
) *fakeAMPAPI {
	t.Helper()

	fake := &fakeAMPAPI{
		calls: map[string]int{},
		state: initialState,
	}

	fake.server = httptest.NewServer(
		http.HandlerFunc(
			func(
				responseWriter http.ResponseWriter,
				request *http.Request,
			) {
				fake.mutex.Lock()
				defer fake.mutex.Unlock()

				fake.calls[request.URL.Path]++

				responseWriter.Header().Set(
					"Content-Type",
					"application/json",
				)

				switch request.URL.Path {
				case "/API/Core/Login":
					_, _ = responseWriter.Write([]byte(
						`{"result":0,"resultReason":"","success":true,"permissions":[],"sessionID":"sessao-de-teste"}`,
					))

				case "/API/Core/GetStatus":
					if fake.statusFailed {
						http.Error(
							responseWriter,
							"erro simulado",
							http.StatusInternalServerError,
						)

						return
					}

					_, _ = responseWriter.Write([]byte(
						`{"State":` + strconv.Itoa(fake.state) + `}`,
					))

				case "/API/Core/Start":
					fake.state = 20

					_, _ = responseWriter.Write(
						[]byte(`{"Status":true}`),
					)

				case "/API/Core/Stop":
					fake.state = 0

					_, _ = responseWriter.Write(
						[]byte(`{"Status":true}`),
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

	t.Cleanup(fake.server.Close)

	return fake
}

func (f *fakeAMPAPI) count(path string) int {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	return f.calls["/API/Core/"+path]
}

func (f *fakeAMPAPI) totalCalls() int {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	total := 0
	for _, count := range f.calls {
		total += count
	}

	return total
}

// fakeSudo cria um script no lugar do sudo, que registra os argumentos.
type fakeSudo struct {
	dir string
}

func installFakeSudo(t *testing.T) *fakeSudo {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("wrapper sudo falso requer shell POSIX")
	}

	dir := t.TempDir()
	logPath := filepath.Join(dir, "calls.log")
	failPath := filepath.Join(dir, "fail")
	scriptPath := filepath.Join(dir, "sudo")

	script := "#!/bin/sh\n" +
		"printf '%s\n' \"$*\" >> '" + logPath + "'\n" +
		"if [ -e '" + failPath + "' ]; then exit 1; fi\n" +
		"exit 0\n"

	if err := os.WriteFile(
		scriptPath,
		[]byte(script),
		0o755,
	); err != nil {
		t.Fatalf("não foi possível criar o sudo falso: %v", err)
	}

	err := amp.ConfigureRuntime(amp.RuntimeConfig{
		SystemUser:  "amp",
		ManagerPath: "/usr/bin/ampinstmgr",
		WrapperPath: "/usr/local/bin/ampcontrol-amp",
		SudoPath:    scriptPath,
	})
	if err != nil {
		t.Fatalf("ConfigureRuntime retornou erro: %v", err)
	}

	t.Cleanup(func() {
		_ = amp.ConfigureRuntime(amp.RuntimeConfig{
			SystemUser:  "amp",
			ManagerPath: "/usr/bin/ampinstmgr",
			WrapperPath: "/usr/local/bin/ampcontrol-amp",
			SudoPath:    "/usr/bin/sudo",
		})
	})

	return &fakeSudo{dir: dir}
}

func (s *fakeSudo) setFailing(t *testing.T) {
	t.Helper()

	if err := os.WriteFile(
		filepath.Join(s.dir, "fail"),
		nil,
		0o600,
	); err != nil {
		t.Fatalf("não foi possível criar o marcador de falha: %v", err)
	}
}

func (s *fakeSudo) lines(t *testing.T) []string {
	t.Helper()

	data, err := os.ReadFile(
		filepath.Join(s.dir, "calls.log"),
	)
	if os.IsNotExist(err) {
		return nil
	}

	if err != nil {
		t.Fatalf("não foi possível ler o log do sudo falso: %v", err)
	}

	text := strings.TrimSpace(string(data))
	if text == "" {
		return nil
	}

	return strings.Split(text, "\n")
}

func (s *fakeSudo) assertCalls(t *testing.T, want ...string) {
	t.Helper()

	got := s.lines(t)

	if len(got) != len(want) {
		t.Fatalf(
			"chamadas ao wrapper = %q, esperado %q",
			got,
			want,
		)
	}

	for index := range want {
		if got[index] != want[index] {
			t.Fatalf(
				"chamada %d ao wrapper = %q, esperado %q",
				index,
				got[index],
				want[index],
			)
		}
	}
}

func ampOperationsTestContext(
	t *testing.T,
) context.Context {
	t.Helper()

	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	t.Cleanup(cancel)

	return ctx
}

func newAMPOperationsTestClient() *Client {
	return &Client{
		ampClient: amp.NewAPIClient("usuario", "senha"),
	}
}

func TestPerformAMPControlOperationUnknown(t *testing.T) {
	t.Parallel()

	client := newAMPOperationsTestClient()

	_, err := client.performAMPControlOperation(
		ampOperationsTestContext(t),
		amp.ManagedInstance{Name: "Minecraft01"},
		ampCommandOperation("x"),
	)
	if err == nil {
		t.Fatal("operação desconhecida deveria retornar erro")
	}
}

func TestPerformAMPControlOperationApplication(t *testing.T) {
	// ConfigureRuntime altera estado global; sem t.Parallel.
	sudo := installFakeSudo(t)

	const (
		stateStopped   = 0
		stateStarting  = 10
		stateReady     = 20
		stateFailed    = 100
		stateSuspended = 200
		stateUnknown   = 999
	)

	tests := []struct {
		name        string
		operation   ampCommandOperation
		running     bool
		state       int
		wantPrefix  string
		wantErr     bool
		wantStarts  int
		wantStops   int
		wantSudo    []string
		wantNoAPI   bool
		statusFails bool
	}{
		// start
		{
			name:       "start instância desligada",
			operation:  ampCommandOperationStart,
			running:    false,
			state:      stateStopped,
			wantPrefix: "✅",
			wantStarts: 1,
			wantSudo:   []string{"-n -u amp /usr/local/bin/ampcontrol-amp start Minecraft01"},
		},
		{
			name:       "start jogo online",
			operation:  ampCommandOperationStart,
			running:    true,
			state:      stateReady,
			wantPrefix: "🟢",
		},
		{
			name:       "start jogo em transição",
			operation:  ampCommandOperationStart,
			running:    true,
			state:      stateStarting,
			wantPrefix: "🔵",
		},
		{
			name:       "start jogo suspenso",
			operation:  ampCommandOperationStart,
			running:    true,
			state:      stateSuspended,
			wantPrefix: "🟠",
		},
		{
			name:       "start jogo idle",
			operation:  ampCommandOperationStart,
			running:    true,
			state:      stateStopped,
			wantPrefix: "✅",
			wantStarts: 1,
		},
		{
			name:       "start jogo com falha",
			operation:  ampCommandOperationStart,
			running:    true,
			state:      stateFailed,
			wantPrefix: "✅",
			wantStarts: 1,
		},
		{
			name:       "start estado desconhecido",
			operation:  ampCommandOperationStart,
			running:    true,
			state:      stateUnknown,
			wantPrefix: "✅",
			wantStarts: 1,
		},
		{
			name:        "start com falha no GetStatus",
			operation:   ampCommandOperationStart,
			running:     true,
			statusFails: true,
			wantErr:     true,
		},
		// stop
		{
			name:       "stop instância desligada",
			operation:  ampCommandOperationStop,
			running:    false,
			wantPrefix: "⚫",
			wantNoAPI:  true,
		},
		{
			name:       "stop jogo idle",
			operation:  ampCommandOperationStop,
			running:    true,
			state:      stateStopped,
			wantPrefix: "💤",
		},
		{
			name:       "stop jogo em transição",
			operation:  ampCommandOperationStop,
			running:    true,
			state:      stateStarting,
			wantPrefix: "🔵",
		},
		{
			name:      "stop jogo com falha",
			operation: ampCommandOperationStop,
			running:   true,
			state:     stateFailed,
			wantErr:   true,
		},
		{
			name:      "stop jogo suspenso",
			operation: ampCommandOperationStop,
			running:   true,
			state:     stateSuspended,
			wantErr:   true,
		},
		{
			name:      "stop estado desconhecido",
			operation: ampCommandOperationStop,
			running:   true,
			state:     stateUnknown,
			wantErr:   true,
		},
		{
			name:       "stop jogo online",
			operation:  ampCommandOperationStop,
			running:    true,
			state:      stateReady,
			wantPrefix: "💤",
			wantStops:  1,
		},
		{
			name:        "stop com falha no GetStatus",
			operation:   ampCommandOperationStop,
			running:     true,
			statusFails: true,
			wantErr:     true,
		},
		// restart
		{
			name:       "restart instância desligada",
			operation:  ampCommandOperationRestart,
			running:    false,
			wantPrefix: "⚫",
			wantNoAPI:  true,
		},
		{
			name:       "restart jogo idle",
			operation:  ampCommandOperationRestart,
			running:    true,
			state:      stateStopped,
			wantPrefix: "💤",
		},
		{
			name:       "restart jogo em transição",
			operation:  ampCommandOperationRestart,
			running:    true,
			state:      stateStarting,
			wantPrefix: "🔵",
		},
		{
			name:      "restart jogo com falha",
			operation: ampCommandOperationRestart,
			running:   true,
			state:     stateFailed,
			wantErr:   true,
		},
		{
			name:      "restart jogo suspenso",
			operation: ampCommandOperationRestart,
			running:   true,
			state:     stateSuspended,
			wantErr:   true,
		},
		{
			name:      "restart estado desconhecido",
			operation: ampCommandOperationRestart,
			running:   true,
			state:     stateUnknown,
			wantErr:   true,
		},
		{
			name:       "restart jogo online",
			operation:  ampCommandOperationRestart,
			running:    true,
			state:      stateReady,
			wantPrefix: "✅",
			wantStops:  1,
			wantStarts: 1,
		},
		{
			name:        "restart com falha no GetStatus",
			operation:   ampCommandOperationRestart,
			running:     true,
			statusFails: true,
			wantErr:     true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Limpa o log do wrapper entre os casos.
			_ = os.Remove(filepath.Join(sudo.dir, "calls.log"))

			fake := newFakeAMPAPI(t, test.state)
			fake.statusFailed = test.statusFails

			instance := amp.ManagedInstance{
				Name:    "Minecraft01",
				APIURL:  fake.server.URL,
				Running: test.running,
			}

			message, err := newAMPOperationsTestClient().
				performAMPControlOperation(
					ampOperationsTestContext(t),
					instance,
					test.operation,
				)

			if test.wantErr {
				if err == nil {
					t.Fatalf("esperava erro, obteve mensagem %q", message)
				}
			} else {
				if err != nil {
					t.Fatalf("erro inesperado: %v", err)
				}

				if !strings.HasPrefix(message, test.wantPrefix) {
					t.Fatalf(
						"mensagem %q deveria começar com %q",
						message,
						test.wantPrefix,
					)
				}
			}

			if got := fake.count("Start"); got != test.wantStarts {
				t.Fatalf("chamadas a Core.Start = %d, esperado %d", got, test.wantStarts)
			}

			if got := fake.count("Stop"); got != test.wantStops {
				t.Fatalf("chamadas a Core.Stop = %d, esperado %d", got, test.wantStops)
			}

			if test.wantNoAPI && fake.totalCalls() != 0 {
				t.Fatalf("nenhuma chamada à API era esperada; total: %d", fake.totalCalls())
			}

			sudo.assertCalls(t, test.wantSudo...)
		})
	}
}

func TestStartAMPApplicationSudoFailureSkipsCoreStart(t *testing.T) {
	sudo := installFakeSudo(t)
	sudo.setFailing(t)

	fake := newFakeAMPAPI(t, 0)

	_, err := newAMPOperationsTestClient().performAMPControlOperation(
		ampOperationsTestContext(t),
		amp.ManagedInstance{
			Name:    "Minecraft01",
			APIURL:  fake.server.URL,
			Running: false,
		},
		ampCommandOperationStart,
	)
	if err == nil {
		t.Fatal("falha do wrapper deveria retornar erro")
	}

	if fake.count("Start") != 0 {
		t.Fatalf("Core.Start não deveria ser chamado; chamadas: %d", fake.count("Start"))
	}

	sudo.assertCalls(
		t,
		"-n -u amp /usr/local/bin/ampcontrol-amp start Minecraft01",
	)
}

func TestPerformAMPControlOperationShutdown(t *testing.T) {
	sudo := installFakeSudo(t)
	client := newAMPOperationsTestClient()

	t.Run("instância desligada não chama o wrapper", func(t *testing.T) {
		message, err := client.performAMPControlOperation(
			ampOperationsTestContext(t),
			amp.ManagedInstance{Name: "Minecraft01", Running: false},
			ampCommandOperationShutdown,
		)
		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}

		if !strings.HasPrefix(message, "⚫") {
			t.Fatalf("mensagem %q deveria começar com ⚫", message)
		}

		sudo.assertCalls(t)
	})

	t.Run("instância ligada chama o wrapper", func(t *testing.T) {
		message, err := client.performAMPControlOperation(
			ampOperationsTestContext(t),
			amp.ManagedInstance{Name: "Minecraft01", Running: true},
			ampCommandOperationShutdown,
		)
		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}

		if !strings.HasPrefix(message, "⚫") {
			t.Fatalf("mensagem %q deveria começar com ⚫", message)
		}

		sudo.assertCalls(
			t,
			"-n -u amp /usr/local/bin/ampcontrol-amp stop Minecraft01",
		)
	})

	t.Run("falha do wrapper retorna erro", func(t *testing.T) {
		sudo.setFailing(t)

		_, err := client.performAMPControlOperation(
			ampOperationsTestContext(t),
			amp.ManagedInstance{Name: "Minecraft01", Running: true},
			ampCommandOperationShutdown,
		)
		if err == nil {
			t.Fatal("falha do wrapper deveria retornar erro")
		}
	})
}

func TestPerformAMPControlOperationUpdate(t *testing.T) {
	sudo := installFakeSudo(t)
	client := newAMPOperationsTestClient()

	message, err := client.performAMPControlOperation(
		ampOperationsTestContext(t),
		amp.ManagedInstance{Name: "Minecraft01", Running: true},
		ampCommandOperationUpdate,
	)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	if !strings.HasPrefix(message, "✅") {
		t.Fatalf("mensagem %q deveria começar com ✅", message)
	}

	sudo.assertCalls(
		t,
		"-n -u amp /usr/local/bin/ampcontrol-amp update Minecraft01",
	)

	sudo.setFailing(t)

	if _, err := client.performAMPControlOperation(
		ampOperationsTestContext(t),
		amp.ManagedInstance{Name: "Minecraft01", Running: true},
		ampCommandOperationUpdate,
	); err == nil {
		t.Fatal("falha do wrapper deveria retornar erro")
	}
}
