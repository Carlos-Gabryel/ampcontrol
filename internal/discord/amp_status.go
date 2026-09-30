package discord

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/Carlos-Gabryel/ampcontrol/internal/amp"
	"github.com/Carlos-Gabryel/ampcontrol/internal/i18n"
)

type ampInstanceStatusView struct {
	Instance          amp.ManagedInstance
	ApplicationStatus *amp.ApplicationStatus
	ApplicationError  error
	PlayerCounts      *amp.PlayerCounts
	PlayerError       error
	PlayerMaxOverride int
	Address           string
}

func (c *Client) collectAMPInstanceStatuses(
	instances []amp.ManagedInstance,
) []ampInstanceStatusView {
	statuses := make(
		[]ampInstanceStatusView,
		len(instances),
	)

	var waitGroup sync.WaitGroup

	for index, instance := range instances {
		statuses[index].Instance = instance
		if presentation, exists := c.instancePresentationSettings(instance.Name); exists {
			statuses[index].PlayerMaxOverride = presentation.MaximumPlayers
			statuses[index].Address = strings.TrimSpace(presentation.Address)
		}

		if !instance.Running {
			continue
		}

		waitGroup.Add(1)

		go func(
			statusIndex int,
			currentInstance amp.ManagedInstance,
		) {
			defer waitGroup.Done()

			if strings.TrimSpace(currentInstance.Game) == "" ||
				strings.EqualFold(currentInstance.Game, "GenericModule") {
				moduleCtx, moduleCancel := context.WithTimeout(
					context.Background(),
					ampApplicationTimeout,
				)
				moduleInfo, moduleErr := c.ampClient.GetModuleInfo(
					moduleCtx,
					currentInstance.APIURL,
				)
				moduleCancel()

				if moduleErr == nil &&
					strings.TrimSpace(moduleInfo.Application) != "" {
					statuses[statusIndex].Instance.Game =
						moduleInfo.Application
				}
			}

			ctx, cancel := context.WithTimeout(
				context.Background(),
				ampApplicationTimeout,
			)
			defer cancel()

			status, err := c.ampClient.GetApplicationStatus(
				ctx,
				currentInstance.APIURL,
			)
			if err != nil {
				statuses[statusIndex].ApplicationError = err

				c.log.Warn().
					Err(err).
					Str("instance", currentInstance.Name).
					Msg("Não foi possível consultar o estado da aplicação")

				return
			}

			statusCopy := status

			statuses[statusIndex].ApplicationStatus =
				&statusCopy

			counts, countsErr := status.PlayerCounts()
			if countsErr != nil {
				statuses[statusIndex].PlayerError = countsErr
				return
			}

			if status.Phase() == amp.ApplicationPhaseOnline &&
				c.playerCountResolver != nil {
				resolverCtx, resolverCancel := context.WithTimeout(
					context.Background(),
					ampApplicationTimeout,
				)
				resolvedCount, applies, resolverErr :=
					c.playerCountResolver.ResolvePlayerCount(
						resolverCtx,
						currentInstance.Name,
					)
				resolverCancel()

				if applies {
					if resolverErr != nil {
						statuses[statusIndex].PlayerError = resolverErr

						c.log.Warn().
							Err(resolverErr).
							Str("instance", currentInstance.Name).
							Msg("Fallback de jogadores do painel falhou")

						return
					}

					counts.Current = resolvedCount
				}
			}

			countsCopy := counts
			if statuses[statusIndex].PlayerMaxOverride > 0 {
				countsCopy.Maximum = statuses[statusIndex].PlayerMaxOverride
			}
			statuses[statusIndex].PlayerCounts = &countsCopy
		}(
			index,
			instance,
		)
	}

	waitGroup.Wait()

	return statuses
}

func describeAMPInstanceStatus(
	statusView ampInstanceStatusView,
) (string, string) {
	if !statusView.Instance.Running {
		return "🔴", "Offline"
	}

	if statusView.ApplicationError != nil {
		return "🔴", i18n.Choose("Offline — estado indisponível", "Offline — state unavailable")
	}

	if statusView.ApplicationStatus == nil {
		return "🔴", i18n.Choose("Offline — estado indisponível", "Offline — state unavailable")
	}

	status := *statusView.ApplicationStatus

	switch status.Phase() {
	case amp.ApplicationPhaseIdle:
		return "🟡", "Idle"

	case amp.ApplicationPhaseOnline:
		return "🟢", "Online"

	case amp.ApplicationPhaseBusy:
		return "🟢", fmt.Sprintf(
			"Online — %s",
			status.State.String(),
		)

	case amp.ApplicationPhaseFailed:
		return "🔴", i18n.Choose("Offline — falha", "Offline — failure")

	case amp.ApplicationPhaseSuspended:
		return "🔴", i18n.Choose("Offline — suspenso", "Offline — suspended")

	default:
		return "🔴", fmt.Sprintf(
			"Offline — %s",
			status.State.String(),
		)
	}
}

func ampInstanceDisplayName(
	instance amp.ManagedInstance,
) string {
	displayName := strings.TrimSpace(
		instance.FriendlyName,
	)

	if displayName == "" {
		return instance.Name
	}

	return displayName
}
