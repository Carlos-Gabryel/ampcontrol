package discord

import (
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/disgoorg/snowflake/v2"
)

func TestAMPCommandCooldownBlocksSameUser(t *testing.T) {
	now := time.Date(2026, 8, 9, 22, 0, 0, 0, time.UTC)
	cooldowns := newAMPCommandCooldowns(5*time.Second, 15*time.Second)
	cooldowns.now = func() time.Time { return now }

	if decision := cooldowns.reserve(snowflake.ID(10), "Valheim01"); !decision.Allowed {
		t.Fatal("primeiro comando deveria ser aceito")
	}

	now = now.Add(2 * time.Second)
	decision := cooldowns.reserve(snowflake.ID(10), "OutroServidor")
	if decision.Allowed || decision.Scope != ampCommandCooldownScopeUser {
		t.Fatalf("cooldown do usuário não foi aplicado: %+v", decision)
	}
	if seconds := cooldownSeconds(decision.Remaining); seconds != 3 {
		t.Fatalf("tempo restante inesperado: %d", seconds)
	}
}

func TestAMPCommandCooldownAllowsOnlyOneConcurrentServerReservation(t *testing.T) {
	now := time.Date(2026, 8, 9, 22, 0, 0, 0, time.UTC)
	cooldowns := newAMPCommandCooldowns(0, 15*time.Second)
	cooldowns.now = func() time.Time { return now }

	const attempts = 20
	var allowed atomic.Int32
	var waitGroup sync.WaitGroup
	for index := 0; index < attempts; index++ {
		waitGroup.Add(1)
		go func(user int) {
			defer waitGroup.Done()
			if cooldowns.reserve(snowflake.ID(user+1), "Valheim01").Allowed {
				allowed.Add(1)
			}
		}(index)
	}
	waitGroup.Wait()

	if actual := allowed.Load(); actual != 1 {
		t.Fatalf("esperava uma única reserva concorrente, obtido %d", actual)
	}
}

func TestAMPCommandCooldownBlocksSameServerForAnotherUser(t *testing.T) {
	now := time.Date(2026, 8, 9, 22, 0, 0, 0, time.UTC)
	cooldowns := newAMPCommandCooldowns(5*time.Second, 15*time.Second)
	cooldowns.now = func() time.Time { return now }

	cooldowns.reserve(snowflake.ID(10), "Valheim01")
	now = now.Add(6 * time.Second)

	decision := cooldowns.reserve(snowflake.ID(20), "valheim01")
	if decision.Allowed || decision.Scope != ampCommandCooldownScopeServer {
		t.Fatalf("cooldown da instância não foi aplicado: %+v", decision)
	}
	if seconds := cooldownSeconds(decision.Remaining); seconds != 9 {
		t.Fatalf("tempo restante inesperado: %d", seconds)
	}
	if !strings.Contains(decision.UserMessage(), "valheim01") {
		t.Fatalf("mensagem deveria identificar a instância: %q", decision.UserMessage())
	}
}

func TestAMPCommandCooldownExpiresAndCanBeDisabled(t *testing.T) {
	now := time.Date(2026, 8, 9, 22, 0, 0, 0, time.UTC)
	cooldowns := newAMPCommandCooldowns(5*time.Second, 15*time.Second)
	cooldowns.now = func() time.Time { return now }

	cooldowns.reserve(snowflake.ID(10), "Valheim01")
	now = now.Add(16 * time.Second)
	if decision := cooldowns.reserve(snowflake.ID(10), "Valheim01"); !decision.Allowed {
		t.Fatalf("cooldown expirado deveria permitir o comando: %+v", decision)
	}

	disabled := newAMPCommandCooldowns(0, 0)
	disabled.now = func() time.Time { return now }
	if !disabled.reserve(snowflake.ID(30), "Valheim01").Allowed ||
		!disabled.reserve(snowflake.ID(30), "Valheim01").Allowed {
		t.Fatal("cooldowns em zero deveriam ficar desativados")
	}
}

func TestAMPCommandCooldownDoesNotConsumeUserWindowWhenServerBlocks(t *testing.T) {
	now := time.Date(2026, 8, 9, 22, 0, 0, 0, time.UTC)
	cooldowns := newAMPCommandCooldowns(5*time.Second, 15*time.Second)
	cooldowns.now = func() time.Time { return now }

	cooldowns.reserve(snowflake.ID(10), "Valheim01")
	now = now.Add(6 * time.Second)
	if cooldowns.reserve(snowflake.ID(20), "Valheim01").Allowed {
		t.Fatal("a instância ainda deveria estar em cooldown")
	}

	if decision := cooldowns.reserve(snowflake.ID(20), "ProjectZomboid01"); !decision.Allowed {
		t.Fatalf("recusa por servidor não deveria consumir cooldown do usuário: %+v", decision)
	}
}
