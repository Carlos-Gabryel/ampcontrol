package discord

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/alabamaamp/ampcontrol/internal/amp"
	disgoDiscord "github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
)

type playerCountResolverStub struct {
	count   int
	applies bool
	err     error
	calls   int
}

func (r *playerCountResolverStub) ResolvePlayerCount(
	context.Context,
	string,
) (int, bool, error) {
	r.calls++
	return r.count, r.applies, r.err
}

func TestAMPOperationProtectedFromPlayers(t *testing.T) {
	protected := []ampCommandOperation{
		ampCommandOperationStop,
		ampCommandOperationRestart,
		ampCommandOperationShutdown,
		ampCommandOperationUpdate,
	}
	for _, operation := range protected {
		if !ampOperationProtectedFromPlayers(operation) {
			t.Fatalf("operação %s deveria ser protegida", operation)
		}
	}

	if ampOperationProtectedFromPlayers(ampCommandOperationStart) {
		t.Fatal("o comando de início não deveria ser bloqueado pela proteção")
	}
}

func TestAMPCommandBypassesPlayerProtection(t *testing.T) {
	ownerID := snowflake.ID(100)
	regular := &disgoDiscord.ResolvedMember{}
	administrator := &disgoDiscord.ResolvedMember{
		Permissions: disgoDiscord.PermissionAdministrator,
	}

	if !ampCommandBypassesPlayerProtection(ownerID, regular, ownerID) {
		t.Fatal("o proprietário deveria ignorar a proteção")
	}
	if !ampCommandBypassesPlayerProtection(snowflake.ID(200), administrator, ownerID) {
		t.Fatal("um administrador deveria ignorar a proteção")
	}
	if ampCommandBypassesPlayerProtection(snowflake.ID(200), regular, ownerID) {
		t.Fatal("um usuário comum não deveria ignorar a proteção")
	}
}

func TestDecideAMPPlayerProtection(t *testing.T) {
	if decision := decideAMPPlayerProtection(0, nil, false); !decision.Allowed {
		t.Fatal("servidor vazio deveria permitir a operação")
	}
	if decision := decideAMPPlayerProtection(2, nil, false); decision.Allowed {
		t.Fatal("usuário comum não deveria interromper dois jogadores")
	}
	if decision := decideAMPPlayerProtection(0, errors.New("indisponível"), false); decision.Allowed {
		t.Fatal("falha de detecção deveria bloquear usuário comum")
	}

	decision := decideAMPPlayerProtection(2, nil, true)
	if !decision.Allowed || !decision.Bypassed {
		t.Fatalf("administrador deveria continuar com aviso: %+v", decision)
	}
}

func TestResolveAMPPlayerCountFromStatusUsesAPIPlayers(t *testing.T) {
	resolver := &playerCountResolverStub{count: 9, applies: true}
	client := &Client{playerCountResolver: resolver}
	status := amp.ApplicationStatus{
		State: amp.ApplicationStateReady,
		Metrics: map[string]amp.StatusMetric{
			"Active Users": {RawValue: 3, MaxValue: 32},
		},
	}

	count, err := client.resolveAMPPlayerCountFromStatus(
		context.Background(),
		amp.ManagedInstance{Name: "Valheim01"},
		status,
	)
	if err != nil || count != 3 {
		t.Fatalf("contagem da API inesperada: count=%d err=%v", count, err)
	}
	if resolver.calls != 0 {
		t.Fatal("fallback não deveria ser consultado quando a API encontra jogadores")
	}
}

func TestResolveAMPPlayerCountFromStatusConfirmsZeroWithFallback(t *testing.T) {
	resolver := &playerCountResolverStub{count: 2, applies: true}
	client := &Client{playerCountResolver: resolver}
	status := amp.ApplicationStatus{
		State: amp.ApplicationStateReady,
		Metrics: map[string]amp.StatusMetric{
			"Active Users": {RawValue: 0, MaxValue: 32},
		},
	}

	count, err := client.resolveAMPPlayerCountFromStatus(
		context.Background(),
		amp.ManagedInstance{Name: "TheWalkingRats01"},
		status,
	)
	if err != nil || count != 2 {
		t.Fatalf("fallback inesperado: count=%d err=%v", count, err)
	}
	if resolver.calls != 1 {
		t.Fatalf("fallback deveria ser consultado uma vez, chamadas=%d", resolver.calls)
	}
}

func TestResolveAMPPlayerCountFromStatusFailsClosed(t *testing.T) {
	client := &Client{}
	status := amp.ApplicationStatus{State: amp.ApplicationStateReady}

	_, err := client.resolveAMPPlayerCountFromStatus(
		context.Background(),
		amp.ManagedInstance{Name: "Servidor01"},
		status,
	)
	if err == nil || !strings.Contains(err.Error(), "confirmar") {
		t.Fatalf("métrica inválida deveria falhar de forma segura: %v", err)
	}
}

func TestResolveAMPPlayerCountFromStatusFailsClosedWhenFallbackFails(t *testing.T) {
	resolver := &playerCountResolverStub{
		applies: true,
		err:     errors.New("RCON indisponível"),
	}
	client := &Client{playerCountResolver: resolver}
	status := amp.ApplicationStatus{
		State: amp.ApplicationStateReady,
		Metrics: map[string]amp.StatusMetric{
			"Active Users": {RawValue: 0, MaxValue: 32},
		},
	}

	_, err := client.resolveAMPPlayerCountFromStatus(
		context.Background(),
		amp.ManagedInstance{Name: "TheWalkingRats01"},
		status,
	)
	if err == nil || !strings.Contains(err.Error(), "detector alternativo") {
		t.Fatalf("falha do fallback deveria bloquear por segurança: %v", err)
	}
}

func TestResolveAMPPlayerCountFromIdleStatusAllowsWithoutMetric(t *testing.T) {
	client := &Client{}
	status := amp.ApplicationStatus{State: amp.ApplicationStateSleeping}

	count, err := client.resolveAMPPlayerCountFromStatus(
		context.Background(),
		amp.ManagedInstance{Name: "Servidor01"},
		status,
	)
	if err != nil || count != 0 {
		t.Fatalf("aplicação em Idle deveria estar vazia: count=%d err=%v", count, err)
	}
}

func TestPlayerProtectionMessagesExplainRefusal(t *testing.T) {
	instance := amp.ManagedInstance{Name: "Valheim01", FriendlyName: "Valheim"}
	message := (ampPlayerProtectionDecision{PlayerCount: 1}).RefusalMessage(
		instance,
		ampCommandOperationRestart,
	)
	if !strings.Contains(message, "1 jogador conectado") ||
		!strings.Contains(message, "reiniciar") {
		t.Fatalf("mensagem de recusa incompleta: %q", message)
	}
}
