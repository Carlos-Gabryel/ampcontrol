package discord

import (
	"context"
	"fmt"
	"strings"
	"time"

	disgoDiscord "github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/snowflake/v2"
)

func (c *Client) handleAMPConfigCommand(
	event *events.ApplicationCommandInteractionCreate,
	data disgoDiscord.SlashCommandInteractionData,
) {
	if !ampConfigCommandAuthorized(
		event.User().ID,
		event.Member(),
		c.ownerUserID,
	) {
		c.log.Warn().
			Str("user_id", event.User().ID.String()).
			Msg("Comando administrativo do AmpControl recusado")
		c.sendInteractionMessage(
			event,
			"⛔ Somente o proprietário configurado pode alterar a visibilidade das instâncias.",
		)
		return
	}

	if data.SubCommandName == nil {
		c.sendInteractionMessage(
			event,
			"⚠️ Nenhum subcomando de configuração foi informado.",
		)
		return
	}

	switch *data.SubCommandName {
	case "ocultar":
		c.handleAMPInstanceVisibilityCommand(event, data, true)

	case "exibir":
		c.handleAMPInstanceVisibilityCommand(event, data, false)

	case "listar":
		c.handleAMPHiddenInstancesCommand(event)

	default:
		c.sendInteractionMessage(
			event,
			"⚠️ Subcomando de configuração não reconhecido.",
		)
	}
}

func ampConfigCommandAuthorized(
	userID snowflake.ID,
	member *disgoDiscord.ResolvedMember,
	ownerUserID snowflake.ID,
) bool {
	return ownerUserID != 0 &&
		userID == ownerUserID &&
		ampCommandAuthorized(member)
}

func (c *Client) handleAMPInstanceVisibilityCommand(
	event *events.ApplicationCommandInteractionCreate,
	data disgoDiscord.SlashCommandInteractionData,
	hidden bool,
) {
	if !c.deferAMPInteraction(event) {
		return
	}

	instanceName, exists := data.OptString("servidor")
	instanceName = strings.TrimSpace(instanceName)
	if !exists || instanceName == "" {
		c.updateInteractionMessage(
			event,
			"⚠️ A instância AMP não foi informada.",
		)
		return
	}

	if hidden {
		instance, err := c.resolveAMPInstance(instanceName)
		if err != nil {
			c.updateInteractionMessage(
				event,
				"⚠️ A instância selecionada não existe ou não pode ser controlada.",
			)
			return
		}
		instanceName = instance.Name
	} else {
		hiddenNames := hiddenInstanceMap(c.hiddenInstanceNames())
		storedName, isHidden := hiddenNames[normalizeInstanceVisibilityKey(instanceName)]
		if !isHidden {
			c.updateInteractionMessage(
				event,
				"ℹ️ Essa instância já está visível no Discord.",
			)
			return
		}
		instanceName = storedName
	}

	changed, err := c.setInstanceHidden(instanceName, hidden)
	if err != nil {
		c.log.Error().
			Err(err).
			Str("instance", instanceName).
			Msg("Não foi possível salvar a visibilidade da instância")
		c.updateInteractionMessage(
			event,
			"❌ Não foi possível salvar a preferência de visibilidade.",
		)
		return
	}

	if !changed {
		state := "visível"
		if hidden {
			state = "oculta"
		}
		c.updateInteractionMessage(
			event,
			fmt.Sprintf("ℹ️ **%s** já está %s no Discord.", instanceName, state),
		)
		return
	}

	c.requestStatusRefresh()
	ctx, cancel := context.WithTimeout(context.Background(), ampDiscoveryTimeout)
	defer cancel()
	registrationErr := c.registerCommands(ctx)

	action := "voltou a aparecer"
	if hidden {
		action = "foi ocultada"
	}
	message := fmt.Sprintf(
		"✅ **%s** %s no painel e nos comandos do Discord.",
		instanceName,
		action,
	)
	if registrationErr != nil {
		c.log.Error().
			Err(registrationErr).
			Msg("Preferência salva, mas os comandos não foram atualizados")
		message += "\n⚠️ A preferência foi salva, mas a lista dos comandos só será atualizada na próxima conexão do bot."
	}

	c.updateInteractionMessage(event, message)
	c.deleteInteractionResponseLater(event, 8*time.Second)
}

func (c *Client) handleAMPHiddenInstancesCommand(
	event *events.ApplicationCommandInteractionCreate,
) {
	if !c.deferAMPInteraction(event) {
		return
	}

	hiddenNames := c.hiddenInstanceNames()
	message := "ℹ️ Nenhuma instância está oculta no Discord."
	if len(hiddenNames) > 0 {
		message = "**Instâncias ocultas:**\n- `" +
			strings.Join(hiddenNames, "`\n- `") + "`"
	}

	c.updateInteractionMessage(event, message)
	c.deleteInteractionResponseLater(event, 12*time.Second)
}
