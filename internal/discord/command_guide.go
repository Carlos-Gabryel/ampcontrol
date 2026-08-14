package discord

import (
	"fmt"

	"github.com/Carlos-Gabryel/ampcontrol/internal/i18n"
	disgoDiscord "github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
)

const commandGuideContent = "— Como usar os comandos"

func buildAMPCommandGuideEmbeds(restricted bool) []disgoDiscord.Embed {
	l := i18n.Choose
	intro := l("Escolha o comando digitando `/` e selecione o servidor na lista.\n\n", "Type `/`, choose a command, and select the server from the list.\n\n")
	footer := l("Todos os comandos /amp estão disponíveis para os usuários.", "All /amp commands are available to users.")
	if restricted {
		intro = l("Escolha o comando digitando `/` neste canal e selecione o servidor na lista. Os comandos não funcionam em outros canais.\n\n", "Type `/` in this channel, choose a command, and select the server. Commands do not work in other channels.\n\n")
		footer = l("Todos os comandos /amp estão disponíveis para os usuários deste canal.", "All /amp commands are available to users in this channel.")
	}
	commands := []struct{ title, description string }{
		{l("/amp status", "/amp status"), l("Atualiza o painel fixo de servidores. Não inicia, reinicia ou para nenhuma instância. **Disponível para todos.**", "Refreshes the pinned server dashboard. It does not start, restart, or stop any instance. **Available to everyone.**")},
		{l("/amp iniciar servidor:<servidor>", "/amp start server:<server>"), l("Liga a instância AMP, se necessário, inicia o processo do jogo e aguarda o estado Online. O início é manual, pode levar alguns minutos e está **disponível para todos**.", "Starts the AMP instance if needed, starts the game process, and waits for Online state. Startup is manual, may take a few minutes, and is **available to everyone**.")},
		{l("/amp parar servidor:<servidor>", "/amp stop server:<server>"), l("Para somente o processo do jogo. A instância AMP permanece ligada e o painel passa a mostrar **Idle**. **Disponível para todos.**", "Stops only the game process. The AMP instance stays running and the dashboard shows **Idle**. **Available to everyone.**")},
		{l("/amp reiniciar servidor:<servidor>", "/amp restart server:<server>"), l("Reinicia somente o processo do jogo e aguarda que ele volte a ficar Online. **Disponível para todos.**", "Restarts only the game process and waits for it to return Online. **Available to everyone.**")},
		{l("/amp desligar servidor:<servidor> confirmar:sim", "/amp shutdown server:<server> confirm:yes"), l("Desliga completamente a instância AMP. Exige a confirmação oferecida pelo Discord e o painel passa a mostrar **Offline**. **Disponível para todos.**", "Shuts down the AMP instance completely. Requires Discord confirmation and the dashboard shows **Offline**. **Available to everyone.**")},
		{l("/amp atualizar servidor:<servidor> confirmar:sim", "/amp update server:<server> confirm:yes"), l("Atualiza a instância selecionada. Exige confirmação e reinicia a instância caso ela esteja ligada, interrompendo o servidor durante o processo. **Disponível para todos.**", "Updates the selected instance. Requires confirmation and restarts it if running, interrupting the server during the process. **Available to everyone.**")},
	}
	embeds := make([]disgoDiscord.Embed, 0, len(commands)+1)
	for index, command := range commands {
		description := command.description
		if index == 0 {
			description = intro + description
		}
		embeds = append(embeds, disgoDiscord.NewEmbed().WithTitle(command.title).WithDescription(description).WithColor(0x5865F2))
	}
	embeds = append(embeds, disgoDiscord.NewEmbed().
		WithTitle(l("🛡️ Proteção de partidas em andamento", "🛡️ Active game protection")).
		WithDescription(l("Usuários comuns não podem parar, reiniciar, desligar ou atualizar um servidor enquanto houver jogadores conectados. Se a quantidade de jogadores não puder ser confirmada, esses comandos também serão bloqueados por segurança.", "Regular users cannot stop, restart, shut down, or update a server while players are connected. If the player count cannot be confirmed, these commands are also blocked for safety.")).
		WithFooter(footer, "").
		WithColor(0x57F287))
	return embeds
}

func (c *Client) upsertCommandGuideMessage() error {
	state, err := loadStatusDashboardState(c.statusStatePath)
	if err != nil {
		return err
	}

	embeds := buildAMPCommandGuideEmbeds(c.restrictCommandChannel)
	if state.GuideMessageID != "" {
		messageID, parseErr := snowflake.Parse(state.GuideMessageID)
		if parseErr == nil && messageID != 0 {
			existing, getErr := c.channels.GetMessage(
				c.notificationChannelID,
				messageID,
			)
			if getErr == nil && existing.Author.ID == c.bot.ID() {
				update := disgoDiscord.NewMessageUpdate().
					WithContent(i18n.Choose(commandGuideContent, "— How to use the commands")).
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
			WithContent(i18n.Choose(commandGuideContent, "— How to use the commands")).
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
