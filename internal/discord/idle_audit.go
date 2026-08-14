package discord

import (
	"fmt"
	"time"

	"github.com/Carlos-Gabryel/ampcontrol/internal/i18n"
	"github.com/Carlos-Gabryel/ampcontrol/internal/idle"
)

// RecordAutomaticIdle registra no canal de auditoria a conclusão da parada
// automática causada por inatividade. O registro é separado das notificações
// temporárias publicadas no canal operacional.
func (c *Client) RecordAutomaticIdle(
	server idle.Server,
	startedAt time.Time,
	operationErr error,
) {
	if c == nil || c.auditChannelID == 0 {
		return
	}

	record := commandAuditRecord{
		UserID:    c.bot.ID(),
		UserName:  i18n.Choose("Motor automático de Idle", "Automatic Idle engine"),
		ChannelID: c.notificationChannelID,
		Command:   i18n.Choose("Idle automático", "Automatic Idle"),
		Server:    server.Instance,
		Options: []string{
			fmt.Sprintf(
				i18n.Choose("**Limite de inatividade:** `%s`", "**Inactivity limit:** `%s`"),
				formatCommandAuditDuration(server.IdleTimeout),
			),
		},
		Accepted:  true,
		Reason:    i18n.Choose("Executado pelo motor automático", "Executed by the automatic engine"),
		CreatedAt: startedAt,
	}

	presentation := commandAuditPresentation{
		Phase:      commandAuditPhaseCompleted,
		Result:     i18n.Choose("O processo do jogo foi encerrado automaticamente por inatividade.", "The game process was stopped automatically due to inactivity."),
		FinalState: "Idle",
		UpdatedAt:  time.Now(),
	}
	if operationErr != nil {
		presentation.Phase = commandAuditPhaseFailed
		presentation.Result = i18n.Choose("Falha ao colocar o servidor em Idle: ", "Failed to place the server in Idle: ") +
			sanitizeAuditText(operationErr.Error(), 800)
		presentation.FinalState = i18n.Choose("Não concluído", "Not completed")
	}

	go c.createDetachedCommandAudit(record, presentation)
}
