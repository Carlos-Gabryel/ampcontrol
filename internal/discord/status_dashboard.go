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

type statusDashboardState struct {
	MessageID string `json:"message_id"`
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

	if err := c.refreshStatusDashboard(ctx); err != nil {
		c.log.Error().
			Err(err).
			Msg("Não foi possível atualizar o painel fixo do Discord")
	}

	// A limpeza do canal nao depende da disponibilidade do AMP.
	c.cleanupExpiredChannelMessages()
}

func (c *Client) refreshStatusDashboard(ctx context.Context) error {
	c.statusRefreshMu.Lock()
	defer c.statusRefreshMu.Unlock()

	instances, err := c.discoverAMPInstances(ctx)
	if err != nil {
		return err
	}

	statuses := c.collectAMPInstanceStatuses(instances)
	embeds := buildAMPStatusEmbeds(statuses, time.Now())

	if err := c.upsertStatusDashboardMessage(embeds); err != nil {
		return err
	}

	return nil
}

func (c *Client) discoverAMPInstances(
	ctx context.Context,
) ([]amp.ManagedInstance, error) {
	instances, err := c.ampClient.DiscoverManagedInstances(
		ctx,
		c.adsURL,
	)
	if err == nil {
		c.applyGameOverrides(instances)
		return instances, nil
	}

	c.log.Warn().
		Err(err).
		Msg("Inventário pela API ADS falhou; usando ampinstmgr como fallback")

	fallback, fallbackErr := amp.DiscoverInstances(ctx)
	if fallbackErr != nil {
		return nil, fmt.Errorf(
			"inventário ADS falhou (%v) e o fallback ampinstmgr também falhou: %w",
			err,
			fallbackErr,
		)
	}

	for index := range fallback {
		if strings.EqualFold(fallback[index].Module, "Minecraft") {
			fallback[index].Game = "Minecraft"
		} else {
			fallback[index].Game = fallback[index].Module
		}
	}

	c.applyGameOverrides(fallback)

	return fallback, nil
}

func (c *Client) applyGameOverrides(instances []amp.ManagedInstance) {
	for index := range instances {
		for instanceName, game := range c.gameOverrides {
			if strings.EqualFold(instances[index].Name, instanceName) &&
				strings.TrimSpace(game) != "" {
				instances[index].Game = strings.TrimSpace(game)
				break
			}
		}
	}
}

func (c *Client) upsertStatusDashboardMessage(
	embeds []disgoDiscord.Embed,
) error {
	state, err := loadStatusDashboardState(c.statusStatePath)
	if err != nil {
		return err
	}

	if state.MessageID != "" {
		messageID, parseErr := snowflake.Parse(state.MessageID)
		if parseErr == nil && messageID != 0 {
			existing, getErr := c.channels.GetMessage(
				c.notificationChannelID,
				messageID,
			)
			if getErr == nil &&
				existing.Author.ID == c.bot.ID() {
				update := disgoDiscord.NewMessageUpdate().
					ClearContent().
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
		disgoDiscord.NewMessageCreate().WithEmbeds(embeds...),
	)
	if err != nil {
		return fmt.Errorf(
			"não foi possível criar a mensagem fixa: %w",
			err,
		)
	}

	if err := saveStatusDashboardState(
		c.statusStatePath,
		statusDashboardState{MessageID: created.ID.String()},
	); err != nil {
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

	return nil
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
			if !shouldDeleteChannelMessage(
				message.ID,
				dashboardID,
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
	createdAt time.Time,
	cutoff time.Time,
) bool {
	return messageID != 0 &&
		messageID != dashboardID &&
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
