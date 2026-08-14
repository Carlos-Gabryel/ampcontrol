package discord

import (
	"context"
	"fmt"

	"github.com/Carlos-Gabryel/ampcontrol/internal/amp"
	disgoDiscord "github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
)

type ampPlayerProtectionDecision struct {
	Allowed     bool
	Bypassed    bool
	PlayerCount int
	Err         error
}

func ampCommandBypassesPlayerProtection(
	userID snowflake.ID,
	member *disgoDiscord.ResolvedMember,
	ownerUserID snowflake.ID,
	adminRoleIDs map[snowflake.ID]struct{},
	allowAdministrators bool,
) bool {
	return ampCommandPrivileged(userID, member, ownerUserID, adminRoleIDs, allowAdministrators)
}

func ampOperationProtectedFromPlayers(
	operation ampCommandOperation,
) bool {
	switch operation {
	case ampCommandOperationStop,
		ampCommandOperationRestart,
		ampCommandOperationShutdown,
		ampCommandOperationUpdate:
		return true
	default:
		return false
	}
}

func (c *Client) checkAMPPlayerProtection(
	ctx context.Context,
	instance amp.ManagedInstance,
	operation ampCommandOperation,
	bypass bool,
) ampPlayerProtectionDecision {
	if !ampOperationProtectedFromPlayers(operation) {
		return ampPlayerProtectionDecision{Allowed: true}
	}

	count, err := c.resolveAMPPlayerCountForProtection(ctx, instance)
	return decideAMPPlayerProtection(count, err, bypass)
}

func decideAMPPlayerProtection(
	playerCount int,
	err error,
	bypass bool,
) ampPlayerProtectionDecision {
	decision := ampPlayerProtectionDecision{
		PlayerCount: playerCount,
		Err:         err,
	}

	if bypass {
		decision.Allowed = true
		decision.Bypassed = playerCount > 0 || err != nil
		return decision
	}

	decision.Allowed = err == nil && playerCount == 0
	return decision
}

func (c *Client) resolveAMPPlayerCountForProtection(
	ctx context.Context,
	instance amp.ManagedInstance,
) (int, error) {
	if !instance.Running {
		return 0, nil
	}

	status, err := c.getAMPApplicationStatus(ctx, instance)
	if err != nil {
		return 0, err
	}

	return c.resolveAMPPlayerCountFromStatus(ctx, instance, status)
}

func (c *Client) resolveAMPPlayerCountFromStatus(
	ctx context.Context,
	instance amp.ManagedInstance,
	status amp.ApplicationStatus,
) (int, error) {
	counts, countsErr := status.PlayerCounts()
	if countsErr == nil && counts.Current > 0 {
		return counts.Current, nil
	}

	if status.Phase() == amp.ApplicationPhaseOnline &&
		c.playerCountResolver != nil {
		resolverCtx, cancel := context.WithTimeout(ctx, ampApplicationTimeout)
		resolvedCount, applies, resolverErr :=
			c.playerCountResolver.ResolvePlayerCount(
				resolverCtx,
				instance.Name,
			)
		cancel()

		if applies {
			if resolverErr != nil {
				return 0, fmt.Errorf(
					"não foi possível confirmar os jogadores pelo detector alternativo: %w",
					resolverErr,
				)
			}
			if resolvedCount < 0 {
				return 0, fmt.Errorf(
					"o detector alternativo retornou uma quantidade negativa de jogadores: %d",
					resolvedCount,
				)
			}
			return resolvedCount, nil
		}
	}

	if countsErr != nil {
		switch status.Phase() {
		case amp.ApplicationPhaseIdle,
			amp.ApplicationPhaseFailed,
			amp.ApplicationPhaseSuspended:
			return 0, nil
		default:
			return 0, fmt.Errorf(
				"não foi possível confirmar a quantidade de jogadores: %w",
				countsErr,
			)
		}
	}

	return counts.Current, nil
}

func (d ampPlayerProtectionDecision) RefusalMessage(
	instance amp.ManagedInstance,
	operation ampCommandOperation,
) string {
	displayName := ampInstanceDisplayName(instance)
	if d.Err != nil {
		return fmt.Sprintf(
			"⛔ Não foi possível confirmar se existem jogadores em **%s**.\n"+
				"Por segurança, usuários comuns não podem %s enquanto essa verificação estiver indisponível.",
			displayName,
			ampProtectedOperationVerb(operation),
		)
	}

	return fmt.Sprintf(
		"⛔ **%s** possui **%s**.\n"+
			"Para proteger a partida, usuários comuns não podem %s enquanto houver pessoas jogando.",
		displayName,
		formatConnectedPlayers(d.PlayerCount),
		ampProtectedOperationVerb(operation),
	)
}

func (d ampPlayerProtectionDecision) BypassMessage(
	instance amp.ManagedInstance,
	operation ampCommandOperation,
) string {
	message := fmt.Sprintf(
		"%s **%s**\nInstância: `%s`",
		ampOperationProgressMessage(operation),
		ampInstanceDisplayName(instance),
		instance.Name,
	)

	if d.Err != nil {
		return message +
			"\n⚠️ Não foi possível confirmar a quantidade de jogadores; " +
			"seu privilégio de proprietário/administrador permitiu continuar."
	}

	return fmt.Sprintf(
		"%s\n⚠️ Proteção ignorada por privilégio de proprietário/administrador: **%s**.",
		message,
		formatConnectedPlayers(d.PlayerCount),
	)
}

func ampProtectedOperationVerb(operation ampCommandOperation) string {
	switch operation {
	case ampCommandOperationStop:
		return "colocar o servidor em Idle"
	case ampCommandOperationRestart:
		return "reiniciar o servidor"
	case ampCommandOperationShutdown:
		return "desligar o servidor"
	case ampCommandOperationUpdate:
		return "atualizar a instância"
	default:
		return "executar essa operação"
	}
}

func formatConnectedPlayers(count int) string {
	if count == 1 {
		return "1 jogador conectado"
	}
	return fmt.Sprintf("%d jogadores conectados", count)
}
