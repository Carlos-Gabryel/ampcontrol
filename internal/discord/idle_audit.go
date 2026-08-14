package discord

import (
	"fmt"
	"time"

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
		UserName:  "Motor automático de Idle",
		ChannelID: c.notificationChannelID,
		Command:   "Idle automático",
		Server:    server.Instance,
		Options: []string{
			fmt.Sprintf(
				"**Limite de inatividade:** `%s`",
				formatCommandAuditDuration(server.IdleTimeout),
			),
		},
		Accepted:  true,
		Reason:    "Executado pelo motor automático",
		CreatedAt: startedAt,
	}

	presentation := commandAuditPresentation{
		Phase:      commandAuditPhaseCompleted,
		Result:     "O processo do jogo foi encerrado automaticamente por inatividade.",
		FinalState: "Idle",
		UpdatedAt:  time.Now(),
	}
	if operationErr != nil {
		presentation.Phase = commandAuditPhaseFailed
		presentation.Result = "Falha ao colocar o servidor em Idle: " +
			sanitizeAuditText(operationErr.Error(), 800)
		presentation.FinalState = "Não concluído"
	}

	go c.createDetachedCommandAudit(record, presentation)
}
