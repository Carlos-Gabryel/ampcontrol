package discord

import (
	"context"

	"github.com/disgoorg/disgo"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/gateway"
	"github.com/rs/zerolog"
)

type Client struct {
	bot *bot.Client
	log zerolog.Logger
}

func New(token string, log zerolog.Logger) (*Client, error) {

	client, err := disgo.New(
		token,
		bot.WithGatewayConfigOpts(
			gateway.WithIntents(
				gateway.IntentGuilds,
			),
		),
		bot.WithEventListenerFunc(
			func(event *events.Ready) {

				log.Info().
					Str("username", event.User.Username).
					Msg("Discord conectado")
			},
		),
	)

	if err != nil {
		return nil, err
	}

	return &Client{
		bot: client,
		log: log,
	}, nil
}


func (c *Client) Start(ctx context.Context) error {

	c.log.Info().
		Msg("Iniciando conexão Discord")

	return c.bot.OpenGateway(ctx)
}


func (c *Client) Close(ctx context.Context) {

	c.log.Info().
		Msg("Encerrando conexão Discord")

	c.bot.Close(ctx)
}
