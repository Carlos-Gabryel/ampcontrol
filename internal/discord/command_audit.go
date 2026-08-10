package discord

import (
	"fmt"
	"sort"
	"strings"
	"time"

	disgoDiscord "github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/snowflake/v2"
)

type commandAuditRecord struct {
	UserID    snowflake.ID
	UserName  string
	ChannelID snowflake.ID
	Command   string
	Options   []string
	Accepted  bool
	Reason    string
	CreatedAt time.Time
}

func (c *Client) validateCommandAuditChannel() error {
	guildID := snowflake.MustParse(discordGuildID)
	channel, err := c.channels.GetChannel(c.auditChannelID)
	if err != nil {
		return fmt.Errorf("não foi possível acessar o canal de auditoria: %w", err)
	}

	textChannel, ok := channel.(disgoDiscord.GuildTextChannel)
	if !ok || textChannel.GuildID() != guildID {
		return fmt.Errorf("o canal de auditoria configurado não é um canal de texto deste servidor")
	}

	if c.auditChannelID == c.notificationChannelID {
		return fmt.Errorf("o canal de auditoria precisa ser diferente do canal do painel")
	}

	return nil
}

func (c *Client) commandAuditDecision(
	event *events.ApplicationCommandInteractionCreate,
	data disgoDiscord.SlashCommandInteractionData,
	channelAllowed bool,
) (bool, string) {
	if !channelAllowed {
		return false, "Recusado: canal não autorizado"
	}

	if data.CommandName() == "ampconfig" && !ampConfigCommandAuthorized(
		event.User().ID,
		event.Member(),
		c.ownerUserID,
	) {
		return false, "Recusado: usuário sem permissão"
	}

	return true, "Recebido"
}

func newCommandAuditRecord(
	event *events.ApplicationCommandInteractionCreate,
	data disgoDiscord.SlashCommandInteractionData,
	accepted bool,
	reason string,
) commandAuditRecord {
	options := make([]string, 0, len(data.Options))
	optionNames := make([]string, 0, len(data.Options))
	for name := range data.Options {
		optionNames = append(optionNames, name)
	}
	sort.Strings(optionNames)
	for _, name := range optionNames {
		options = append(
			options,
			fmt.Sprintf("**%s:** `%s`", name, data.Options[name].String()),
		)
	}

	return commandAuditRecord{
		UserID:    event.User().ID,
		UserName:  event.User().EffectiveName(),
		ChannelID: event.Channel().ID(),
		Command:   formatCommandAuditPath(data.CommandPath()),
		Options:   options,
		Accepted:  accepted,
		Reason:    reason,
		CreatedAt: time.Now(),
	}
}

func formatCommandAuditPath(path string) string {
	parts := strings.FieldsFunc(path, func(r rune) bool {
		return r == '/'
	})
	if len(parts) == 0 {
		return "/"
	}
	return "/" + strings.Join(parts, " ")
}

func (c *Client) sendCommandAudit(record commandAuditRecord) {
	channelID := c.auditChannelID
	if channelID == 0 {
		c.log.Warn().
			Str("command", record.Command).
			Msg("Comando recebido sem canal de auditoria disponível")
		return
	}

	color := 0x57F287
	status := "✅ Comando recebido"
	if !record.Accepted {
		color = 0xED4245
		status = "⛔ Comando recusado"
	}

	embed := disgoDiscord.NewEmbed().
		WithTitle(status).
		AddField(
			"Usuário",
			fmt.Sprintf("<@%s> • %s\nID: `%s`", record.UserID, record.UserName, record.UserID),
			true,
		).
		AddField("Comando", fmt.Sprintf("`%s`", record.Command), true).
		AddField("Canal", fmt.Sprintf("<#%s>", record.ChannelID), true).
		AddField("Horário", fmt.Sprintf("<t:%d:F>", record.CreatedAt.Unix()), true).
		AddField("Situação", record.Reason, false).
		WithTimestamp(record.CreatedAt).
		WithColor(color)

	if len(record.Options) > 0 {
		embed = embed.AddField("Opções", strings.Join(record.Options, "\n"), false)
	}

	_, err := c.channels.CreateMessage(
		channelID,
		disgoDiscord.NewMessageCreate().WithEmbeds(embed),
	)
	if err != nil {
		c.log.Error().
			Err(err).
			Str("command", record.Command).
			Str("user_id", record.UserID.String()).
			Msg("Não foi possível registrar a auditoria do comando no Discord")
	}
}
