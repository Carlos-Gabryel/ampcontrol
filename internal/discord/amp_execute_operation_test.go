package discord

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/Carlos-Gabryel/ampcontrol/internal/amp"
	"github.com/Carlos-Gabryel/ampcontrol/internal/operation"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"
)

// recordingInteractions registra cada atualização da resposta da interação.
type recordingInteractions struct {
	rest.Interactions

	mutex    sync.Mutex
	messages []string
}

func (r *recordingInteractions) UpdateInteractionResponse(
	_ snowflake.ID,
	_ string,
	update discord.MessageUpdate,
	_ ...rest.RequestOpt,
) (*discord.Message, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	content := ""
	if update.Content != nil {
		content = *update.Content
	}

	r.messages = append(r.messages, content)

	return &discord.Message{}, nil
}

func (r *recordingInteractions) all() []string {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	return append([]string(nil), r.messages...)
}

// fakePlayerCountResolver devolve uma contagem fixa de jogadores.
type fakePlayerCountResolver struct {
	count int
	err   error
}

func (f fakePlayerCountResolver) ResolvePlayerCount(
	context.Context,
	string,
) (int, bool, error) {
	return f.count, true, f.err
}

type executeOperationHarness struct {
	client       *Client
	interactions *recordingInteractions
	manager      *operation.Manager
}

func newExecuteOperationHarness(
	t *testing.T,
	withManager bool,
	resolver PlayerCountResolver,
) *executeOperationHarness {
	t.Helper()

	interactions := &recordingInteractions{}
	client := newAMPOperationsTestClient()
	client.interactions = interactions
	client.statusRefreshRequests = make(chan struct{}, 1)
	client.playerCountResolver = resolver

	manager := operation.NewManager()

	if withManager {
		if err := client.SetOperationManager(manager); err != nil {
			t.Fatalf("não foi possível configurar o gerenciador: %v", err)
		}

		t.Cleanup(func() { clearOperationManager(client) })
	}

	return &executeOperationHarness{
		client:       client,
		interactions: interactions,
		manager:      manager,
	}
}

func (h *executeOperationHarness) run(
	instance amp.ManagedInstance,
	commandOperation ampCommandOperation,
	bypass bool,
) {
	h.client.executeAMPControlOperation(
		snowflake.ID(1),
		"token-de-teste",
		instance,
		commandOperation,
		bypass,
	)
}

func (h *executeOperationHarness) assertLockReleased(t *testing.T) {
	t.Helper()

	result, err := h.manager.TryAcquire("Minecraft01", "verificação")
	if err != nil {
		t.Fatalf("TryAcquire retornou erro: %v", err)
	}

	if !result.Acquired() {
		t.Fatal("o bloqueio da instância deveria ter sido liberado")
	}

	result.Lease.Release()
}

func (h *executeOperationHarness) assertRefreshRequested(t *testing.T) {
	t.Helper()

	select {
	case <-h.client.statusRefreshRequests:
	default:
		t.Fatal("a atualização do painel deveria ter sido solicitada")
	}
}

func TestExecuteAMPControlOperationSuccess(t *testing.T) {
	// ConfigureRuntime altera estado global; sem t.Parallel.
	const (
		stateStopped = 0
		stateReady   = 20
	)

	tests := []struct {
		name       string
		operation  ampCommandOperation
		running    bool
		state      int
		wantPrefix string
		wantStarts int
		wantStops  int
		wantSudo   []string
	}{
		{
			name:       "start liga instância e jogo",
			operation:  ampCommandOperationStart,
			running:    false,
			state:      stateStopped,
			wantPrefix: "✅",
			wantStarts: 1,
			wantSudo:   []string{"-n -u amp /usr/local/bin/ampcontrol-amp start Minecraft01"},
		},
		{
			name:       "stop sem jogadores coloca em Idle",
			operation:  ampCommandOperationStop,
			running:    true,
			state:      stateReady,
			wantPrefix: "💤",
			wantStops:  1,
		},
		{
			name:       "restart com jogo em Idle não reinicia",
			operation:  ampCommandOperationRestart,
			running:    true,
			state:      stateStopped,
			wantPrefix: "💤",
		},
		{
			name:       "shutdown chama o wrapper stop",
			operation:  ampCommandOperationShutdown,
			running:    true,
			state:      stateReady,
			wantPrefix: "⚫",
			wantSudo:   []string{"-n -u amp /usr/local/bin/ampcontrol-amp stop Minecraft01"},
		},
		{
			name:       "update chama o wrapper update",
			operation:  ampCommandOperationUpdate,
			running:    true,
			state:      stateReady,
			wantPrefix: "✅",
			wantSudo:   []string{"-n -u amp /usr/local/bin/ampcontrol-amp update Minecraft01"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sudo := installFakeSudo(t)
			fake := newFakeAMPAPI(t, test.state)
			harness := newExecuteOperationHarness(
				t,
				true,
				fakePlayerCountResolver{count: 0},
			)

			harness.run(
				amp.ManagedInstance{
					Name:    "Minecraft01",
					APIURL:  fake.server.URL,
					Running: test.running,
				},
				test.operation,
				false,
			)

			messages := harness.interactions.all()
			if len(messages) != 1 {
				t.Fatalf("esperava 1 atualização, obteve %q", messages)
			}

			if !strings.HasPrefix(messages[0], test.wantPrefix) {
				t.Fatalf("mensagem %q deveria começar com %q", messages[0], test.wantPrefix)
			}

			if !strings.Contains(messages[0], "`Minecraft01`") {
				t.Fatalf("mensagem %q deveria citar a instância", messages[0])
			}

			if got := fake.count("Start"); got != test.wantStarts {
				t.Fatalf("chamadas a Core.Start = %d, esperado %d", got, test.wantStarts)
			}

			if got := fake.count("Stop"); got != test.wantStops {
				t.Fatalf("chamadas a Core.Stop = %d, esperado %d", got, test.wantStops)
			}

			sudo.assertCalls(t, test.wantSudo...)
			harness.assertLockReleased(t)
			harness.assertRefreshRequested(t)
		})
	}
}

func TestExecuteAMPControlOperationFailureReportsErrorAndReleasesLock(t *testing.T) {
	sudo := installFakeSudo(t)
	sudo.setFailing(t)

	fake := newFakeAMPAPI(t, 0)
	harness := newExecuteOperationHarness(t, true, nil)

	harness.run(
		amp.ManagedInstance{
			Name:         "Minecraft01",
			FriendlyName: "Servidor Mine",
			APIURL:       fake.server.URL,
			Running:      false,
		},
		ampCommandOperationStart,
		false,
	)

	messages := harness.interactions.all()
	if len(messages) != 1 {
		t.Fatalf("esperava 1 atualização, obteve %q", messages)
	}

	for _, want := range []string{"❌", "Servidor Mine", "Erro:"} {
		if !strings.Contains(messages[0], want) {
			t.Fatalf("mensagem %q deveria conter %q", messages[0], want)
		}
	}

	if strings.Contains(messages[0], "`Minecraft01`") {
		t.Fatalf("mensagem de erro não deveria usar o formato de sucesso: %q", messages[0])
	}

	// A falha do wrapper interrompe a cadeia: o jogo nunca é iniciado.
	if fake.count("Start") != 0 {
		t.Fatalf("Core.Start não deveria ser chamado; chamadas: %d", fake.count("Start"))
	}

	harness.assertLockReleased(t)
	harness.assertRefreshRequested(t)
}

func TestExecuteAMPControlOperationStatusFailureStopsChain(t *testing.T) {
	sudo := installFakeSudo(t)

	fake := newFakeAMPAPI(t, 20)
	fake.statusFailed = true

	// Sem jogadores confirmados e sem resolvedor, a falha do estado
	// bloqueia a proteção; com bypass a operação segue e falha no estado.
	harness := newExecuteOperationHarness(t, true, nil)

	harness.run(
		amp.ManagedInstance{
			Name:    "Minecraft01",
			APIURL:  fake.server.URL,
			Running: true,
		},
		ampCommandOperationStop,
		true,
	)

	messages := harness.interactions.all()
	if len(messages) != 2 {
		t.Fatalf("esperava aviso de bypass e falha, obteve %q", messages)
	}

	if !strings.Contains(messages[0], "⚠️") {
		t.Fatalf("primeira mensagem %q deveria avisar o bypass", messages[0])
	}

	if !strings.HasPrefix(messages[1], "❌") {
		t.Fatalf("segunda mensagem %q deveria ser a falha", messages[1])
	}

	if fake.count("Stop") != 0 {
		t.Fatal("Core.Stop não deveria ser chamado após falha de estado")
	}

	sudo.assertCalls(t)
	harness.assertLockReleased(t)
}

func TestExecuteAMPControlOperationLockErrorWithoutManager(t *testing.T) {
	sudo := installFakeSudo(t)
	harness := newExecuteOperationHarness(t, false, nil)

	harness.run(
		amp.ManagedInstance{Name: "Minecraft01", Running: true},
		ampCommandOperationUpdate,
		false,
	)

	messages := harness.interactions.all()
	if len(messages) != 1 {
		t.Fatalf("esperava 1 atualização, obteve %q", messages)
	}

	for _, want := range []string{"❌", "Minecraft01", "Erro:"} {
		if !strings.Contains(messages[0], want) {
			t.Fatalf("mensagem %q deveria conter %q", messages[0], want)
		}
	}

	// Sem bloqueio, nada pode ser executado.
	sudo.assertCalls(t)
	harness.assertRefreshRequested(t)
}

func TestExecuteAMPControlOperationBusyInstance(t *testing.T) {
	sudo := installFakeSudo(t)
	fake := newFakeAMPAPI(t, 20)
	harness := newExecuteOperationHarness(t, true, nil)

	held, err := harness.manager.TryAcquire("Minecraft01", "comando /amp reiniciar")
	if err != nil || !held.Acquired() {
		t.Fatalf("não foi possível reservar a instância: %v", err)
	}

	harness.run(
		amp.ManagedInstance{
			Name:    "Minecraft01",
			APIURL:  fake.server.URL,
			Running: true,
		},
		ampCommandOperationUpdate,
		false,
	)

	messages := harness.interactions.all()
	if len(messages) != 1 {
		t.Fatalf("esperava 1 atualização, obteve %q", messages)
	}

	for _, want := range []string{"⏳", "comando /amp reiniciar"} {
		if !strings.Contains(messages[0], want) {
			t.Fatalf("mensagem %q deveria conter %q", messages[0], want)
		}
	}

	sudo.assertCalls(t)

	if fake.totalCalls() != 0 {
		t.Fatalf("nenhuma chamada à API era esperada; total: %d", fake.totalCalls())
	}

	// A operação recusada não pode liberar o bloqueio de outra operação.
	again, err := harness.manager.TryAcquire("Minecraft01", "outra")
	if err != nil {
		t.Fatalf("TryAcquire retornou erro: %v", err)
	}

	if again.Acquired() {
		t.Fatal("o bloqueio original deveria continuar ativo")
	}

	held.Lease.Release()
	harness.assertRefreshRequested(t)
}

func TestExecuteAMPControlOperationPlayerProtection(t *testing.T) {
	tests := []struct {
		name         string
		resolver     fakePlayerCountResolver
		bypass       bool
		wantMessages int
		wantRefusal  bool
		wantStops    int
	}{
		{
			name:         "jogadores conectados recusam",
			resolver:     fakePlayerCountResolver{count: 3},
			wantMessages: 1,
			wantRefusal:  true,
		},
		{
			name:         "detector indisponível recusa",
			resolver:     fakePlayerCountResolver{err: errors.New("detector fora do ar")},
			wantMessages: 1,
			wantRefusal:  true,
		},
		{
			name:         "privilegiado ignora a proteção e executa",
			resolver:     fakePlayerCountResolver{count: 3},
			bypass:       true,
			wantMessages: 2,
			wantStops:    1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sudo := installFakeSudo(t)
			fake := newFakeAMPAPI(t, 20)
			harness := newExecuteOperationHarness(t, true, test.resolver)

			harness.run(
				amp.ManagedInstance{
					Name:    "Minecraft01",
					APIURL:  fake.server.URL,
					Running: true,
				},
				ampCommandOperationStop,
				test.bypass,
			)

			messages := harness.interactions.all()
			if len(messages) != test.wantMessages {
				t.Fatalf("esperava %d atualizações, obteve %q", test.wantMessages, messages)
			}

			first := messages[0]
			if test.wantRefusal && !strings.HasPrefix(first, "⛔") {
				t.Fatalf("mensagem %q deveria ser uma recusa", first)
			}

			if !test.wantRefusal {
				if !strings.Contains(first, "⚠️") {
					t.Fatalf("mensagem %q deveria avisar o bypass", first)
				}

				last := messages[len(messages)-1]
				if !strings.HasPrefix(last, "💤") {
					t.Fatalf("mensagem final %q deveria indicar sucesso", last)
				}
			}

			if got := fake.count("Stop"); got != test.wantStops {
				t.Fatalf("chamadas a Core.Stop = %d, esperado %d", got, test.wantStops)
			}

			sudo.assertCalls(t)
			harness.assertLockReleased(t)
			harness.assertRefreshRequested(t)
		})
	}
}

func TestExecuteAMPControlOperationUnprotectedOperationSkipsPlayerCheck(t *testing.T) {
	installFakeSudo(t)
	fake := newFakeAMPAPI(t, 0)

	// Um resolvedor com jogadores não pode bloquear o start.
	harness := newExecuteOperationHarness(
		t,
		true,
		fakePlayerCountResolver{count: 5},
	)

	harness.run(
		amp.ManagedInstance{
			Name:    "Minecraft01",
			APIURL:  fake.server.URL,
			Running: false,
		},
		ampCommandOperationStart,
		false,
	)

	messages := harness.interactions.all()
	if len(messages) != 1 || !strings.HasPrefix(messages[0], "✅") {
		t.Fatalf("start deveria concluir normalmente, obteve %q", messages)
	}
}

func TestExecuteAMPControlOperationUnknownOperationFails(t *testing.T) {
	installFakeSudo(t)
	harness := newExecuteOperationHarness(t, true, nil)

	harness.run(
		amp.ManagedInstance{Name: "Minecraft01", Running: true},
		ampCommandOperation("x"),
		false,
	)

	messages := harness.interactions.all()
	if len(messages) != 1 || !strings.HasPrefix(messages[0], "❌") {
		t.Fatalf("operação desconhecida deveria falhar, obteve %q", messages)
	}

	harness.assertLockReleased(t)
}
