package discord

import (
	"fmt"

	disgoDiscord "github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
)

const commandGuideContent = "— Como usar os comandos"

func buildAMPCommandGuideEmbeds() []disgoDiscord.Embed {
	operations := disgoDiscord.NewEmbed().
		WithDescription(
			"Escolha o comando digitando `/` neste canal e selecione o servidor na lista. "+
				"Os comandos não funcionam em outros canais.",
		).
		AddField(
			"### /amp status",
			"Atualiza o painel fixo de servidores. Não inicia, reinicia ou para nenhuma instância. **Disponível para todos.**",
			false,
		).
		AddField(
			"### /amp iniciar servidor:<servidor>",
			"Liga a instância AMP, se necessário, inicia o processo do jogo e aguarda o estado Online. O início é manual, pode levar alguns minutos e está **disponível para todos**.",
			false,
		).
		AddField(
			"### /amp parar servidor:<servidor>",
			"Para somente o processo do jogo. A instância AMP permanece ligada e o painel mostra a bolinha amarela de **Idle**. **Disponível para todos.**",
			false,
		).
		AddField(
			"### /amp reiniciar servidor:<servidor>",
			"Reinicia somente o processo do jogo e aguarda que ele volte a ficar Online. **Disponível para todos.**",
			false,
		).
		AddField(
			"### /amp desligar servidor:<servidor> confirmar:sim",
			"Desliga completamente a instância AMP. Exige a confirmação oferecida pelo Discord e o painel passa a mostrar **Offline**. **Disponível para todos.**",
			false,
		).
		AddField(
			"### /amp atualizar servidor:<servidor> confirmar:sim",
			"Atualiza a instância selecionada. Exige confirmação e reinicia a instância caso ela esteja ligada, interrompendo o servidor durante o processo. **Disponível para todos.**",
			false,
		).
		AddField(
			"🛡️ Proteção de partidas em andamento",
			"Usuários comuns não podem parar, reiniciar, desligar ou atualizar um servidor enquanto houver jogadores conectados. Se a quantidade de jogadores não puder ser confirmada, esses comandos também serão bloqueados por segurança.",
			false,
		).
		WithFooter(
			"Todos os comandos /amp estão disponíveis para os usuários deste canal.",
			"",
		).
		WithColor(0x5865F2)

	return []disgoDiscord.Embed{operations}
}

func (c *Client) upsertCommandGuideMessage() error {
	state, err := loadStatusDashboardState(c.statusStatePath)
	if err != nil {
		return err
	}

	embeds := buildAMPCommandGuideEmbeds()
	if state.GuideMessageID != "" {
		messageID, parseErr := snowflake.Parse(state.GuideMessageID)
		if parseErr == nil && messageID != 0 {
			existing, getErr := c.channels.GetMessage(
				c.notificationChannelID,
				messageID,
			)
			if getErr == nil && existing.Author.ID == c.bot.ID() {
				update := disgoDiscord.NewMessageUpdate().
					WithContent(commandGuideContent).
					WithEmbeds(embeds...)

				updated, updateErr := c.channels.UpdateMessage(
					c.notificationChannelID,
					messageID,
					update,
				)
				if updateErr != nil {
					return fmt.Errorf(
						"não foi possível editar o guia fixo de comandos: %w",
						updateErr,
					)
				}

				if !updated.Pinned {
					if pinErr := c.channels.PinMessage(
						c.notificationChannelID,
						messageID,
					); pinErr != nil {
						c.log.Warn().
							Err(pinErr).
							Msg("Guia atualizado, mas não foi possível fixá-lo")
					}
				}

				return nil
			}

			if getErr != nil && !isDiscordNotFound(getErr) {
				return fmt.Errorf(
					"não foi possível consultar o guia fixo de comandos: %w",
					getErr,
				)
			}
		}
	}

	created, err := c.channels.CreateMessage(
		c.notificationChannelID,
		disgoDiscord.NewMessageCreate().
			WithContent(commandGuideContent).
			WithEmbeds(embeds...),
	)
	if err != nil {
		return fmt.Errorf("não foi possível criar o guia fixo de comandos: %w", err)
	}

	state.GuideMessageID = created.ID.String()
	if err := saveStatusDashboardState(c.statusStatePath, state); err != nil {
		return err
	}

	if err := c.channels.PinMessage(
		c.notificationChannelID,
		created.ID,
	); err != nil {
		c.log.Warn().
			Err(err).
			Msg("Guia criado, mas não foi possível fixá-lo")
	}

	c.log.Info().
		Str("message_id", created.ID.String()).
		Msg("Guia fixo de comandos criado no Discord")

	return nil
}
