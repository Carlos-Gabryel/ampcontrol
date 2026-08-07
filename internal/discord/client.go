package discord

import (
	"context"
	"fmt"
	"time"

	"github.com/alabamaamp/palcontrol/internal/amp"
	"github.com/disgoorg/disgo"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/gateway"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"
	"github.com/rs/zerolog"
)

type managedServer struct {
	DisplayName  string
	APIURL       string
	RCONAddress  string
	RCONPassword string
	Instance     amp.Instance
}

type Client struct {
	bot                   *bot.Client
	ampClient             *amp.APIClient
	interactions          rest.Interactions
	channels              rest.Channels
	notificationChannelID snowflake.ID
	managedServers        []managedServer
	idleTimeout           time.Duration
	log                   zerolog.Logger
}

func New(
	token string,
	ampClient *amp.APIClient,
	alamamaRCONPassword string,
	kalagaRCONPassword string,
	notificationChannelID string,
	idleTimeout time.Duration,
	log zerolog.Logger,
) (*Client, error) {
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
		notificationChannelID: snowflake.MustParse(notificationChannelID),
		idleTimeout:           idleTimeout,
		log:                   log,
		managedServers: []managedServer{
			{
				DisplayName:  "Alamama",
				APIURL:       "http://127.0.0.1:8090",
				RCONAddress:  "127.0.0.1:25575",
				RCONPassword: alamamaRCONPassword,
				Instance: amp.Instance{
					Name:     "AlamamaPal01",
					GamePort: 8211,
				},
			},
			{
				DisplayName:  "Kalaga",
				APIURL:       "http://127.0.0.1:8088",
				RCONAddress:  "127.0.0.1:25576",
				RCONPassword: kalagaRCONPassword,
				Instance: amp.Instance{
					Name:     "KalagaPal01",
					GamePort: 8212,
				},
			},
		},
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

	err := RegisterCommands(
		context.Background(),
		c.bot.Rest,
		c.bot.ApplicationID,
	)
	if err != nil {
		c.log.Error().
			Err(err).
			Msg("Erro registrando comandos Discord")

		return
	}

	c.log.Info().
		Msg("Comandos Discord registrados")
}

func (c *Client) serverInstances() []amp.Instance {
	instances := make(
		[]amp.Instance,
		0,
		len(c.managedServers),
	)

	for _, server := range c.managedServers {
		instances = append(
			instances,
			server.Instance,
		)
	}

	return instances
}

func (c *Client) sendInteractionMessage(
	event *events.ApplicationCommandInteractionCreate,
	content string,
) {
	message := discord.NewMessageCreate().
		WithContent(content)

	err := event.CreateMessage(message)
	if err != nil {
		c.log.Error().
			Err(err).
			Msg("Erro enviando resposta ao Discord")

		return
	}

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

		return
	}

	c.log.Info().
		Msg("Resposta da interação atualizada")
}

func (c *Client) sendChannelMessage(
	content string,
) error {
	message := discord.NewMessageCreate().
		WithContent(content)

	_, err := c.channels.CreateMessage(
		c.notificationChannelID,
		message,
	)
	if err != nil {
		return fmt.Errorf(
			"não foi possível enviar mensagem ao canal: %w",
			err,
		)
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

	go c.runIdleMonitor(ctx)

	return nil
}

func (c *Client) Close(
	ctx context.Context,
) {
	c.log.Info().
		Msg("Encerrando conexão Discord")

	c.bot.Close(ctx)
}
