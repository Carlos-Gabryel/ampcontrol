package discord

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	disgoDiscord "github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/snowflake/v2"
)

type commandAuditPhase string

const (
	commandAuditPhaseReceived  commandAuditPhase = "received"
	commandAuditPhaseRunning   commandAuditPhase = "running"
	commandAuditPhaseCompleted commandAuditPhase = "completed"
	commandAuditPhaseFailed    commandAuditPhase = "failed"
	commandAuditPhaseRefused   commandAuditPhase = "refused"
)

type commandAuditRecord struct {
	UserID    snowflake.ID
	UserName  string
	ChannelID snowflake.ID
	Command   string
	Server    string
	Options   []string
	Accepted  bool
	Reason    string
	CreatedAt time.Time
}

type commandAuditPresentation struct {
	Phase      commandAuditPhase
	Result     string
	FinalState string
	UpdatedAt  time.Time
}

type commandAuditSession struct {
	mu           sync.Mutex
	record       commandAuditRecord
	presentation commandAuditPresentation
	messageID    snowflake.ID
	terminal     bool
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

	server := ""
	for _, name := range optionNames {
		value := data.Options[name].String()
		if name == "servidor" {
			server = value
			continue
		}
		options = append(
			options,
			fmt.Sprintf("**%s:** `%s`", name, sanitizeAuditText(value, 300)),
		)
	}

	return commandAuditRecord{
		UserID:    event.User().ID,
		UserName:  event.User().EffectiveName(),
		ChannelID: event.Channel().ID(),
		Command:   formatCommandAuditPath(data.CommandPath()),
		Server:    server,
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

func (c *Client) beginCommandAudit(
	interactionToken string,
	record commandAuditRecord,
) {
	if c.auditChannelID == 0 {
		return
	}

	if !record.Accepted {
		go c.createDetachedCommandAudit(
			record,
			commandAuditPresentation{
				Phase:      commandAuditPhaseRefused,
				Result:     record.Reason,
				FinalState: "Não executado",
				UpdatedAt:  time.Now(),
			},
		)
		return
	}

	session := &commandAuditSession{
		record: record,
		presentation: commandAuditPresentation{
			Phase:     commandAuditPhaseReceived,
			Result:    record.Reason,
			UpdatedAt: time.Now(),
		},
	}
	if _, loaded := c.commandAudits.LoadOrStore(interactionToken, session); loaded {
		c.log.Warn().
			Str("command", record.Command).
			Msg("Uma auditoria já existe para esta interação")
		return
	}

	go c.createCommandAuditSession(interactionToken, session)
	time.AfterFunc(ampControlTimeout+time.Minute, func() {
		c.finishCommandAudit(
			interactionToken,
			commandAuditPhaseFailed,
			"A auditoria excedeu o tempo máximo de acompanhamento da operação.",
			"Tempo limite excedido",
		)
	})
}

func (c *Client) createDetachedCommandAudit(
	record commandAuditRecord,
	presentation commandAuditPresentation,
) {
	_, err := c.createCommandAuditMessage(record, presentation)
	if err != nil {
		c.log.Error().
			Err(err).
			Str("command", record.Command).
			Str("user_id", record.UserID.String()).
			Msg("Não foi possível registrar a auditoria do comando no Discord")
	}
}

func (c *Client) createCommandAuditSession(
	interactionToken string,
	session *commandAuditSession,
) {
	session.mu.Lock()
	record := session.record
	initialPresentation := session.presentation
	session.mu.Unlock()

	messageID, err := c.createCommandAuditMessage(record, initialPresentation)
	if err != nil {
		c.commandAudits.Delete(interactionToken)
		c.log.Error().
			Err(err).
			Str("command", record.Command).
			Str("user_id", record.UserID.String()).
			Msg("Não foi possível iniciar a auditoria do comando no Discord")
		return
	}

	session.mu.Lock()
	session.messageID = messageID
	presentation := session.presentation
	terminal := session.terminal
	changed := presentation.Phase != initialPresentation.Phase ||
		presentation.Result != initialPresentation.Result ||
		presentation.FinalState != initialPresentation.FinalState
	session.mu.Unlock()

	if changed {
		c.updateCommandAuditMessage(messageID, record, presentation)
	}
	if terminal {
		c.commandAudits.Delete(interactionToken)
	}
}

func (c *Client) createCommandAuditMessage(
	record commandAuditRecord,
	presentation commandAuditPresentation,
) (snowflake.ID, error) {
	created, err := c.channels.CreateMessage(
		c.auditChannelID,
		disgoDiscord.NewMessageCreate().WithEmbeds(
			buildCommandAuditEmbed(record, presentation),
		),
	)
	if err != nil {
		return 0, err
	}
	return created.ID, nil
}

func (c *Client) updateCommandAuditMessage(
	messageID snowflake.ID,
	record commandAuditRecord,
	presentation commandAuditPresentation,
) {
	_, err := c.channels.UpdateMessage(
		c.auditChannelID,
		messageID,
		disgoDiscord.NewMessageUpdate().WithEmbeds(
			buildCommandAuditEmbed(record, presentation),
		),
	)
	if err != nil {
		c.log.Error().
			Err(err).
			Str("message_id", messageID.String()).
			Str("command", record.Command).
			Msg("Não foi possível atualizar a auditoria do comando")
	}
}

func (c *Client) recordCommandAuditResponse(
	interactionToken string,
	content string,
) {
	phase, terminal := classifyCommandAuditResponse(content)
	if terminal {
		c.finishCommandAudit(
			interactionToken,
			phase,
			content,
			"",
		)
		return
	}

	c.transitionCommandAudit(
		interactionToken,
		phase,
		content,
		"",
		false,
	)
}

func (c *Client) finishCommandAudit(
	interactionToken string,
	phase commandAuditPhase,
	result string,
	finalState string,
) {
	c.transitionCommandAudit(
		interactionToken,
		phase,
		result,
		finalState,
		true,
	)
}

func (c *Client) transitionCommandAudit(
	interactionToken string,
	phase commandAuditPhase,
	result string,
	finalState string,
	terminal bool,
) {
	value, exists := c.commandAudits.Load(interactionToken)
	if !exists {
		return
	}
	session, ok := value.(*commandAuditSession)
	if !ok || session == nil {
		c.commandAudits.Delete(interactionToken)
		return
	}

	session.mu.Lock()
	if session.terminal {
		session.mu.Unlock()
		return
	}

	if finalState == "" && terminal {
		finalState = inferCommandAuditFinalState(
			session.record.Command,
			result,
			phase,
		)
	}
	session.presentation = commandAuditPresentation{
		Phase:      phase,
		Result:     sanitizeAuditText(result, 950),
		FinalState: finalState,
		UpdatedAt:  time.Now(),
	}
	session.terminal = terminal
	messageID := session.messageID
	record := session.record
	presentation := session.presentation
	session.mu.Unlock()

	if messageID == 0 {
		return
	}

	c.updateCommandAuditMessage(messageID, record, presentation)
	if terminal {
		c.commandAudits.Delete(interactionToken)
	}
}

func classifyCommandAuditResponse(
	content string,
) (commandAuditPhase, bool) {
	trimmed := strings.TrimSpace(content)
	lower := strings.ToLower(trimmed)

	if strings.Contains(lower, "já possui uma operação em andamento") {
		return commandAuditPhaseFailed, true
	}

	progressPrefixes := []string{
		"⏳ Iniciando",
		"⏳ Colocando",
		"🔄 Reiniciando",
		"⚫ Desligando",
		"⬆️ Atualizando",
		"⏳ Executando",
	}
	for _, prefix := range progressPrefixes {
		if strings.HasPrefix(trimmed, prefix) {
			return commandAuditPhaseRunning, false
		}
	}

	if strings.HasPrefix(trimmed, "⛔") {
		return commandAuditPhaseRefused, true
	}
	if strings.HasPrefix(trimmed, "❌") ||
		strings.HasPrefix(trimmed, "⚠️") {
		return commandAuditPhaseFailed, true
	}

	return commandAuditPhaseCompleted, true
}

func inferCommandAuditFinalState(
	command string,
	result string,
	phase commandAuditPhase,
) string {
	if phase == commandAuditPhaseFailed ||
		phase == commandAuditPhaseRefused {
		return "Não concluído"
	}

	lower := strings.ToLower(result)
	if strings.Contains(lower, "offline") {
		return "Offline"
	}
	if strings.Contains(lower, "modo idle") ||
		strings.Contains(lower, "em idle") {
		return "Idle"
	}
	if strings.Contains(lower, "transição") {
		return "Em transição"
	}
	if strings.Contains(lower, "suspens") {
		return "Suspenso"
	}

	switch command {
	case "/amp iniciar", "/amp reiniciar":
		return "Online"
	case "/amp parar":
		return "Idle"
	case "/amp desligar":
		return "Offline"
	case "/amp atualizar":
		return "Atualização concluída"
	case "/amp status":
		return "Painel atualizado"
	default:
		return "Concluído"
	}
}

func buildCommandAuditEmbed(
	record commandAuditRecord,
	presentation commandAuditPresentation,
) disgoDiscord.Embed {
	title, color, status := describeCommandAuditPhase(presentation.Phase)
	embed := disgoDiscord.NewEmbed().
		WithTitle(title).
		AddField(
			"Usuário",
			fmt.Sprintf("<@%s> • %s\nID: `%s`", record.UserID, record.UserName, record.UserID),
			true,
		).
		AddField("Comando", fmt.Sprintf("`%s`", record.Command), true).
		AddField("Canal", fmt.Sprintf("<#%s>", record.ChannelID), true).
		AddField("Solicitado", fmt.Sprintf("<t:%d:F>", record.CreatedAt.Unix()), true).
		AddField("Situação", status, true).
		WithTimestamp(presentation.UpdatedAt).
		WithColor(color)

	if strings.TrimSpace(record.Server) != "" {
		embed = embed.AddField("Servidor", fmt.Sprintf("`%s`", sanitizeAuditText(record.Server, 300)), true)
	}
	if len(record.Options) > 0 {
		embed = embed.AddField("Opções", strings.Join(record.Options, "\n"), false)
	}
	if strings.TrimSpace(presentation.FinalState) != "" {
		embed = embed.AddField("Estado final", presentation.FinalState, true)
	}
	if presentation.Phase != commandAuditPhaseReceived &&
		strings.TrimSpace(presentation.Result) != "" {
		embed = embed.AddField("Resultado", presentation.Result, false)
	}
	if presentation.Phase == commandAuditPhaseRunning ||
		presentation.Phase == commandAuditPhaseCompleted ||
		presentation.Phase == commandAuditPhaseFailed ||
		presentation.Phase == commandAuditPhaseRefused {
		embed = embed.AddField(
			"Duração",
			formatCommandAuditDuration(presentation.UpdatedAt.Sub(record.CreatedAt)),
			true,
		)
	}

	return embed
}

func describeCommandAuditPhase(
	phase commandAuditPhase,
) (string, int, string) {
	switch phase {
	case commandAuditPhaseRunning:
		return "🔵 Comando em execução", 0x3498DB, "Em execução"
	case commandAuditPhaseCompleted:
		return "✅ Comando concluído", 0x57F287, "Concluído"
	case commandAuditPhaseFailed:
		return "❌ Comando falhou", 0xED4245, "Falhou"
	case commandAuditPhaseRefused:
		return "⛔ Comando recusado", 0xED4245, "Recusado"
	default:
		return "🟡 Comando recebido", 0xFEE75C, "Aguardando processamento"
	}
}

func formatCommandAuditDuration(duration time.Duration) string {
	if duration < 0 {
		duration = 0
	}
	duration = duration.Round(time.Second)
	if duration < time.Second {
		return "menos de 1 s"
	}

	hours := int(duration / time.Hour)
	minutes := int(duration%time.Hour) / int(time.Minute)
	seconds := int(duration%time.Minute) / int(time.Second)
	parts := make([]string, 0, 3)
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%d h", hours))
	}
	if minutes > 0 {
		parts = append(parts, fmt.Sprintf("%d min", minutes))
	}
	if seconds > 0 || len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("%d s", seconds))
	}
	return strings.Join(parts, " ")
}

func sanitizeAuditText(value string, maximum int) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "`", "'")
	characters := []rune(value)
	if maximum > 0 && len(characters) > maximum {
		value = string(characters[:maximum])
	}
	return value
}
