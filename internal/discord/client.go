package discord

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/alabamaamp/ampcontrol/internal/amp"
	"github.com/disgoorg/disgo"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/gateway"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"
	"github.com/rs/zerolog"
)

type Client struct {
	bot                   *bot.Client
	ampClient             *amp.APIClient
	interactions          rest.Interactions
	channels              rest.Channels
	notificationChannelID snowflake.ID
	auditChannelID        snowflake.ID
	ownerUserID           snowflake.ID
	notificationTTL       time.Duration
	statusRefreshInterval time.Duration
	adsURL                string
	statusStatePath       string
	statusRefreshRequests chan struct{}
	statusRefreshMu       sync.Mutex
	commandRegistrationMu sync.Mutex
	commandInventoryMu    sync.RWMutex
	commandInventory      string
	commandAudits         sync.Map
	preferencesMu         sync.RWMutex
	preferencesPath       string
	preferences           discordPreferences
	gameOverridesMu       sync.RWMutex
	idleRegistrationMu    sync.RWMutex
	idleServerRegistrar   IdleServerRegistrar
	idleRegistered        map[string]string
	gameOverrides         map[string]string
	playerCountResolver   PlayerCountResolver
	log                   zerolog.Logger
}

// PlayerCountResolver aplica os detectores específicos configurados para
// uma instância quando a telemetria genérica do AMP precisa de confirmação.
type PlayerCountResolver interface {
	ResolvePlayerCount(
		ctx context.Context,
		instance string,
	) (
		count int,
		applies bool,
		err error,
	)
}

type ClientConfig struct {
	NotificationChannelID   string
	AuditChannelID          string
	OwnerUserID             string
	NotificationTTL         time.Duration
	StatusRefreshInterval   time.Duration
	ADSURL                  string
	StatusStatePath         string
	PreferencesPath         string
	GameOverrides           map[string]string
	PlayerCountResolver     PlayerCountResolver
	IdleRegisteredInstances []string
}

func New(
	token string,
	ampClient *amp.APIClient,
	config ClientConfig,
	log zerolog.Logger,
) (*Client, error) {
	preferences, err := loadDiscordPreferences(config.PreferencesPath)
	if err != nil {
		return nil, err
	}

	disgoClient, err := disgo.New(
		token,
		bot.WithGatewayConfigOpts(
			gateway.WithIntents(
				gateway.IntentGuilds,
			),
		),
	)
	if err != nil {
		return nil, err
	}

	interactions := rest.NewInteractions(
		disgoClient.Rest,
		discord.AllowedMentions{},
	)

	channels := rest.NewChannels(
		disgoClient.Rest,
		discord.AllowedMentions{},
	)

	client := &Client{
		bot:                   disgoClient,
		ampClient:             ampClient,
		interactions:          interactions,
		channels:              channels,
		notificationChannelID: snowflake.MustParse(config.NotificationChannelID),
		auditChannelID:        snowflake.MustParse(config.AuditChannelID),
		ownerUserID:           snowflake.MustParse(config.OwnerUserID),
		notificationTTL:       config.NotificationTTL,
		statusRefreshInterval: config.StatusRefreshInterval,
		adsURL:                config.ADSURL,
		statusStatePath:       config.StatusStatePath,
		preferencesPath:       config.PreferencesPath,
		preferences:           preferences,
		idleRegistered:        instanceNameMap(config.IdleRegisteredInstances),
		statusRefreshRequests: make(chan struct{}, 1),
		gameOverrides:         copyStringMap(config.GameOverrides),
		playerCountResolver:   config.PlayerCountResolver,
		log:                   log,
	}

	disgoClient.AddEventListeners(
		bot.NewListenerFunc(
			func(event *events.Ready) {
				client.handleReadyEvent(event)
			},
		),
	)

	return client, nil
}

func copyStringMap(source map[string]string) map[string]string {
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func (c *Client) handleReadyEvent(
	event *events.Ready,
) {
	c.log.Info().
		Str("username", event.User.Username).
		Msg("Discord conectado")

	c.log.Info().
		Str(
			"application_id",
			c.bot.ApplicationID.String(),
		).
		Msg("Application ID")

	if err := c.validateCommandAuditChannel(); err != nil {
		c.log.Error().
			Err(err).
			Msg("Não foi possível preparar o canal de auditoria do Discord")
	} else {
		c.log.Info().
			Str("channel_id", c.auditChannelID.String()).
			Msg("Canal de auditoria do Discord pronto")
	}

	err := c.registerCommands(context.Background())
	if err != nil {
		c.log.Error().
			Err(err).
			Msg("Erro registrando comandos Discord")

		return
	}

	c.log.Info().
		Msg("Comandos Discord registrados")
}

func (c *Client) sendInteractionMessage(
	event *events.ApplicationCommandInteractionCreate,
	content string,
) {
	message := discord.NewMessageCreate().
		WithContent(content).
		WithEphemeral(true)

	err := event.CreateMessage(message)
	if err != nil {
		c.log.Error().
			Err(err).
			Msg("Erro enviando resposta ao Discord")

		c.finishCommandAudit(
			event.Token(),
			commandAuditPhaseFailed,
			"Não foi possível responder ao comando no Discord.",
			"",
		)
		return
	}

	c.recordCommandAuditResponse(event.Token(), content)

	c.log.Info().
		Msg("Resposta enviada ao Discord")
}

func (c *Client) updateInteractionMessage(
	event *events.ApplicationCommandInteractionCreate,
	content string,
) {
	c.updateInteractionMessageByToken(
		event.ApplicationID(),
		event.Token(),
		content,
	)
}

func (c *Client) updateInteractionMessageByToken(
	applicationID snowflake.ID,
	interactionToken string,
	content string,
) {
	message := discord.NewMessageUpdate().
		WithContent(content)

	_, err := c.interactions.UpdateInteractionResponse(
		applicationID,
		interactionToken,
		message,
	)
	if err != nil {
		c.log.Error().
			Err(err).
			Msg("Erro atualizando resposta da interação")

		c.finishCommandAudit(
			interactionToken,
			commandAuditPhaseFailed,
			"Não foi possível atualizar a resposta do comando no Discord.",
			"",
		)
		return
	}

	c.recordCommandAuditResponse(interactionToken, content)

	c.log.Info().
		Msg("Resposta da interação atualizada")
}

func (c *Client) deleteInteractionResponseLater(
	event *events.ApplicationCommandInteractionCreate,
	delay time.Duration,
) {
	if event == nil || delay <= 0 {
		return
	}

	applicationID := event.ApplicationID()
	interactionToken := event.Token()
	time.AfterFunc(delay, func() {
		if err := c.interactions.DeleteInteractionResponse(
			applicationID,
			interactionToken,
		); err != nil && !isDiscordNotFound(err) {
			c.log.Warn().
				Err(err).
				Msg("Não foi possível apagar a confirmação temporária")
		}
	})
}

func (c *Client) sendChannelMessage(
	content string,
) error {
	message := discord.NewMessageCreate().
		WithContent(content)

	created, err := c.channels.CreateMessage(
		c.notificationChannelID,
		message,
	)
	if err != nil {
		return fmt.Errorf(
			"não foi possível enviar mensagem ao canal: %w",
			err,
		)
	}

	if c.notificationTTL > 0 {
		time.AfterFunc(c.notificationTTL, func() {
			if err := c.channels.DeleteMessage(
				c.notificationChannelID,
				created.ID,
			); err != nil && !isDiscordNotFound(err) {
				c.log.Warn().
					Err(err).
					Str("message_id", created.ID.String()).
					Msg("Não foi possível apagar uma notificação temporária do AmpControl")
			}
		})
	}

	return nil
}

func (c *Client) Start(
	ctx context.Context,
) error {
	c.log.Info().
		Msg("Iniciando conexão Discord")

	if err := c.bot.OpenGateway(ctx); err != nil {
		return err
	}

	go c.runStatusDashboard(ctx)

	return nil
}

func (c *Client) Close(
	ctx context.Context,
) {
	c.log.Info().
		Msg("Encerrando conexão Discord")

	c.bot.Close(ctx)
}
