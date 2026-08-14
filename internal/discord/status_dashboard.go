package discord

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/alabamaamp/ampcontrol/internal/amp"
	disgoDiscord "github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"
)

const statusDashboardTimeout = 45 * time.Second

const statusDashboardMessageOrderVersion = 1

type statusDashboardState struct {
	MessageID           string   `json:"message_id"`
	MessageIDs          []string `json:"message_ids,omitempty"`
	GuideMessageID      string   `json:"guide_message_id,omitempty"`
	MessageOrderVersion int      `json:"message_order_version,omitempty"`
}

func (c *Client) runStatusDashboard(ctx context.Context) {
	if c == nil || ctx == nil {
		return
	}

	interval := c.statusRefreshInterval
	if interval <= 0 {
		interval = time.Minute
	}

	c.refreshStatusDashboardWithTimeout(ctx)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
			c.refreshStatusDashboardWithTimeout(ctx)

		case <-c.statusRefreshRequests:
			c.refreshStatusDashboardWithTimeout(ctx)
		}
	}
}

func (c *Client) requestStatusRefresh() {
	if c == nil || c.statusRefreshRequests == nil {
		return
	}

	select {
	case c.statusRefreshRequests <- struct{}{}:
	default:
	}
}

func (c *Client) refreshStatusDashboardWithTimeout(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, statusDashboardTimeout)
	defer cancel()

	err := c.refreshStatusDashboard(ctx)
	c.recordDashboardRefresh(err)
	if err != nil {
		c.log.Error().
			Err(err).
			Msg("Não foi possível atualizar o painel fixo do Discord")
	}

	// A limpeza do canal nao depende da disponibilidade do AMP.
	c.cleanupExpiredChannelMessages()
}

func (c *Client) recordDashboardRefresh(refreshErr error) {
	c.diagnosticsMu.Lock()
	defer c.diagnosticsMu.Unlock()

	now := time.Now()
	if refreshErr == nil {
		c.lastDashboardSuccess = now
		c.lastDashboardError = ""
		return
	}

	c.lastDashboardFailure = now
	c.lastDashboardError = sanitizeAuditText(refreshErr.Error(), 500)
}

func (c *Client) refreshStatusDashboard(ctx context.Context) error {
	c.statusRefreshMu.Lock()
	defer c.statusRefreshMu.Unlock()

	instances, err := c.discoverAMPInstances(ctx)
	if err != nil {
		return err
	}
	c.refreshCommandsForInventory(ctx, instances)
	instances = c.visibleAMPInstances(instances)

	statuses := c.collectAMPInstanceStatuses(instances)
	pages := buildAMPStatusPages(statuses, time.Now(), c.gameServerAddress)

	// O guia precisa ser a primeira mensagem do canal. Em instalações
	// antigas, o upsert do painel abaixo faz uma migração única para que o
	// novo painel fique cronologicamente depois do guia.
	if err := c.upsertCommandGuideMessage(); err != nil {
		return err
	}

	if err := c.upsertStatusDashboardMessages(pages, statuses); err != nil {
		return err
	}

	return nil
}

func (c *Client) discoverAMPInstances(
	ctx context.Context,
) ([]amp.ManagedInstance, error) {
	instances, err := c.inventory.DiscoverInstances(ctx)
	if err != nil {
		return nil, err
	}
	c.applyGameOverrides(instances)
	return instances, nil
}

func (c *Client) applyGameOverrides(instances []amp.ManagedInstance) {
	gameOverrides := c.gameOverridesSnapshot()
	for index := range instances {
		for instanceName, game := range gameOverrides {
			if strings.EqualFold(instances[index].Name, instanceName) &&
				strings.TrimSpace(game) != "" {
				instances[index].Game = strings.TrimSpace(game)
				break
			}
		}
	}
	applyInstancePresentationOverrides(instances, c.instancePresentationSettingsSnapshot())
}

func (c *Client) gameOverridesSnapshot() map[string]string {
	c.gameOverridesMu.RLock()
	defer c.gameOverridesMu.RUnlock()
	return copyStringMap(c.gameOverrides)
}

func (c *Client) setGameOverride(instance string, game string) {
	instance = strings.TrimSpace(instance)
	game = strings.TrimSpace(game)
	if instance == "" || game == "" {
		return
	}

	c.gameOverridesMu.Lock()
	c.gameOverrides[instance] = game
	c.gameOverridesMu.Unlock()
}

func (c *Client) upsertStatusDashboardMessages(pages [][]disgoDiscord.LayoutComponent, statuses []ampInstanceStatusView) error {
	state, err := loadStatusDashboardState(c.statusStatePath)
	if err != nil {
		return err
	}
	oldIDs := append([]string(nil), state.MessageIDs...)
	if len(oldIDs) == 0 && strings.TrimSpace(state.MessageID) != "" {
		oldIDs = []string{state.MessageID}
	}
	newIDs := make([]string, 0, len(pages))
	usedOld := make(map[string]struct{})

	for index, page := range pages {
		game := ""
		logoFilename := ""
		if index < len(statuses) {
			game = dashboardGameName(statuses[index].Instance)
			logoFilename = gameIconFilename(game)
		}
		var messageID snowflake.ID
		if index < len(oldIDs) {
			parsed, parseErr := snowflake.Parse(oldIDs[index])
			if parseErr == nil {
				existing, getErr := c.channels.GetMessage(c.notificationChannelID, parsed)
				if getErr == nil && existing.Author.ID == c.bot.ID() && existing.Flags.Has(disgoDiscord.MessageFlagIsComponentsV2) {
					messageID = parsed
					update := disgoDiscord.NewMessageUpdateV2(page...)
					if !dashboardMessageHasLogo(existing.Attachments, logoFilename) {
						attachments := []disgoDiscord.AttachmentUpdate{}
						update.Attachments = &attachments
						logo, logoErr := gameIconFile(game)
						if logoErr != nil {
							return logoErr
						}
						if logo != nil {
							update = update.WithFiles(logo)
						}
					}
					_, err = c.channels.UpdateMessage(c.notificationChannelID, parsed, update)
					if err != nil {
						return fmt.Errorf("não foi possível atualizar o painel %d: %w", index+1, err)
					}
					usedOld[oldIDs[index]] = struct{}{}
				} else if getErr != nil && !isDiscordNotFound(getErr) {
					return fmt.Errorf("não foi possível consultar o painel %d: %w", index+1, getErr)
				}
			}
		}
		if messageID == 0 {
			create := disgoDiscord.NewMessageCreateV2(page...)
			logo, logoErr := gameIconFile(game)
			if logoErr != nil {
				return logoErr
			}
			if logo != nil {
				create = create.WithFiles(logo)
			}
			created, createErr := c.channels.CreateMessage(c.notificationChannelID, create)
			if createErr != nil {
				return fmt.Errorf("não foi possível criar o painel %d: %w", index+1, createErr)
			}
			messageID = created.ID
		}
		newIDs = append(newIDs, messageID.String())
		if pinErr := c.channels.PinMessage(c.notificationChannelID, messageID); pinErr != nil {
			c.log.Warn().Err(pinErr).Str("message_id", messageID.String()).Msg("Painel atualizado, mas não foi possível fixá-lo")
		}
	}

	for _, oldID := range oldIDs {
		if _, keep := usedOld[oldID]; keep {
			continue
		}
		parsed, parseErr := snowflake.Parse(oldID)
		if parseErr == nil && parsed != 0 {
			if deleteErr := c.channels.DeleteMessage(c.notificationChannelID, parsed); deleteErr != nil && !isDiscordNotFound(deleteErr) {
				c.log.Warn().Err(deleteErr).Str("message_id", oldID).Msg("Não foi possível remover um painel antigo")
			}
		}
	}

	state.MessageIDs = newIDs
	state.MessageID = ""
	if len(newIDs) > 0 {
		state.MessageID = newIDs[0]
	}
	state.MessageOrderVersion = statusDashboardMessageOrderVersion
	return saveStatusDashboardState(c.statusStatePath, state)
}

func dashboardMessageHasLogo(attachments []disgoDiscord.Attachment, filename string) bool {
	if filename == "" {
		return len(attachments) == 0
	}
	for _, attachment := range attachments {
		if attachment.Filename == filename {
			return true
		}
	}
	return false
}

func (c *Client) upsertStatusDashboardMessage(
	embeds []disgoDiscord.Embed,
) error {
	const dashboardContent = "— Estado dos servidores"

	state, err := loadStatusDashboardState(c.statusStatePath)
	if err != nil {
		return err
	}

	recreateForOrder := statusDashboardNeedsOrderMigration(state)
	previousMessageID, _ := snowflake.Parse(state.MessageID)

	if state.MessageID != "" && !recreateForOrder {
		messageID, parseErr := snowflake.Parse(state.MessageID)
		if parseErr == nil && messageID != 0 {
			existing, getErr := c.channels.GetMessage(
				c.notificationChannelID,
				messageID,
			)
			if getErr == nil &&
				existing.Author.ID == c.bot.ID() {
				update := disgoDiscord.NewMessageUpdate().
					WithContent(dashboardContent).
					WithEmbeds(embeds...)

				updated, updateErr := c.channels.UpdateMessage(
					c.notificationChannelID,
					messageID,
					update,
				)
				if updateErr != nil {
					return fmt.Errorf(
						"não foi possível editar a mensagem fixa: %w",
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
							Msg("Painel atualizado, mas não foi possível fixá-lo")
					}
				}

				return nil
			}

			if getErr != nil && !isDiscordNotFound(getErr) {
				return fmt.Errorf(
					"não foi possível consultar a mensagem fixa: %w",
					getErr,
				)
			}
		}
	}

	created, err := c.channels.CreateMessage(
		c.notificationChannelID,
		disgoDiscord.NewMessageCreate().
			WithContent(dashboardContent).
			WithEmbeds(embeds...),
	)
	if err != nil {
		return fmt.Errorf(
			"não foi possível criar a mensagem fixa: %w",
			err,
		)
	}

	state.MessageID = created.ID.String()
	if state.GuideMessageID != "" {
		state.MessageOrderVersion = statusDashboardMessageOrderVersion
	}
	if err := saveStatusDashboardState(c.statusStatePath, state); err != nil {
		_ = c.channels.DeleteMessage(c.notificationChannelID, created.ID)
		return err
	}

	if err := c.channels.PinMessage(
		c.notificationChannelID,
		created.ID,
	); err != nil {
		c.log.Warn().
			Err(err).
			Msg("Painel criado, mas não foi possível fixá-lo")
	}

	c.log.Info().
		Str("message_id", created.ID.String()).
		Msg("Painel fixo de status criado no Discord")

	if recreateForOrder && previousMessageID != 0 &&
		previousMessageID != created.ID {
		if err := c.channels.DeleteMessage(
			c.notificationChannelID,
			previousMessageID,
		); err != nil && !isDiscordNotFound(err) {
			c.log.Warn().
				Err(err).
				Str("message_id", previousMessageID.String()).
				Msg("Novo painel criado, mas o painel antigo não pôde ser apagado")
		} else {
			c.log.Info().
				Str("old_message_id", previousMessageID.String()).
				Str("new_message_id", created.ID.String()).
				Msg("Ordem das mensagens fixas migrada para guia seguido do painel")
		}
	}

	return nil
}

func statusDashboardNeedsOrderMigration(state statusDashboardState) bool {
	return state.GuideMessageID != "" &&
		state.MessageOrderVersion < statusDashboardMessageOrderVersion
}

func loadStatusDashboardState(path string) (statusDashboardState, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return statusDashboardState{}, fmt.Errorf(
			"o caminho do estado do painel não foi configurado",
		)
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return statusDashboardState{}, nil
	}
	if err != nil {
		return statusDashboardState{}, fmt.Errorf(
			"não foi possível ler %s: %w",
			path,
			err,
		)
	}

	var state statusDashboardState
	if err := json.Unmarshal(data, &state); err != nil {
		return statusDashboardState{}, fmt.Errorf(
			"o estado do painel em %s é inválido: %w",
			path,
			err,
		)
	}

	return state, nil
}

func saveStatusDashboardState(
	path string,
	state statusDashboardState,
) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf(
			"não foi possível criar a pasta de estado do painel: %w",
			err,
		)
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf(
			"não foi possível serializar o estado do painel: %w",
			err,
		)
	}
	data = append(data, '\n')

	temporaryPath := path + ".tmp"
	if err := os.WriteFile(temporaryPath, data, 0o600); err != nil {
		return fmt.Errorf(
			"não foi possível gravar o estado temporário do painel: %w",
			err,
		)
	}

	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf(
			"não foi possível publicar o estado do painel: %w",
			err,
		)
	}

	return nil
}

func isDiscordNotFound(err error) bool {
	var restError *rest.Error
	return errors.As(err, &restError) &&
		restError.Response != nil &&
		restError.Response.StatusCode == http.StatusNotFound
}

func (c *Client) cleanupExpiredChannelMessages() {
	if c.notificationTTL <= 0 {
		return
	}

	state, err := loadStatusDashboardState(c.statusStatePath)
	if err != nil {
		c.log.Warn().
			Err(err).
			Msg("Não foi possível carregar o painel durante a limpeza do canal")
		return
	}

	dashboardID, _ := snowflake.Parse(state.MessageID)
	dashboardIDs := make(map[snowflake.ID]struct{}, len(state.MessageIDs))
	for _, rawID := range state.MessageIDs {
		if parsed, parseErr := snowflake.Parse(rawID); parseErr == nil && parsed != 0 {
			dashboardIDs[parsed] = struct{}{}
		}
	}
	guideID, _ := snowflake.Parse(state.GuideMessageID)
	cutoff := time.Now().Add(-c.notificationTTL)
	before := snowflake.ID(0)

	for {
		messages, listErr := c.channels.GetMessages(
			c.notificationChannelID,
			0,
			before,
			0,
			100,
		)
		if listErr != nil {
			c.log.Warn().
				Err(listErr).
				Msg("Não foi possível listar mensagens para limpar o canal AmpControl")
			return
		}
		if len(messages) == 0 {
			return
		}

		for _, message := range messages {
			if _, protected := dashboardIDs[message.ID]; protected {
				continue
			}
			if !shouldDeleteChannelMessage(
				message.ID,
				dashboardID,
				guideID,
				message.CreatedAt,
				cutoff,
			) {
				continue
			}

			if deleteErr := c.channels.DeleteMessage(
				c.notificationChannelID,
				message.ID,
			); deleteErr != nil && !isDiscordNotFound(deleteErr) {
				c.log.Warn().
					Err(deleteErr).
					Str("message_id", message.ID.String()).
					Str("author_id", message.Author.ID.String()).
					Msg("Não foi possível apagar uma mensagem expirada do canal AmpControl")
			}
		}

		if len(messages) < 100 {
			return
		}

		before = messages[len(messages)-1].ID
	}
}

func shouldDeleteChannelMessage(
	messageID snowflake.ID,
	dashboardID snowflake.ID,
	guideID snowflake.ID,
	createdAt time.Time,
	cutoff time.Time,
) bool {
	return messageID != 0 &&
		messageID != dashboardID &&
		messageID != guideID &&
		createdAt.Before(cutoff)
}

func formatAMPUptime(raw string) string {
	parts := strings.Split(strings.TrimSpace(raw), ":")
	if len(parts) != 4 {
		return "indisponível"
	}

	values := make([]int, len(parts))
	for index, part := range parts {
		value, err := strconv.Atoi(part)
		if err != nil || value < 0 {
			return "indisponível"
		}
		values[index] = value
	}

	days, hours, minutes, seconds :=
		values[0], values[1], values[2], values[3]

	switch {
	case days > 0:
		return fmt.Sprintf("%dd %dh", days, hours)

	case hours > 0:
		return fmt.Sprintf("%dh %02dmin", hours, minutes)

	case minutes > 0:
		return fmt.Sprintf("%d min", minutes)

	case seconds > 0:
		return "<1 min"

	default:
		return "0 min"
	}
}
