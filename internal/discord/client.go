package discord

import (
	"context"
	"fmt"
	"strings"
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
	Key          string
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
				Key:          "alamama",
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
				Key:          "kalaga",
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

		bot.NewListenerFunc(
			func(event *events.ApplicationCommandInteractionCreate) {
				client.handleCommandEvent(event)
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

func (c *Client) handleCommandEvent(
	event *events.ApplicationCommandInteractionCreate,
) {
	defer func() {
		if recovered := recover(); recovered != nil {
			c.log.Error().
				Interface("panic", recovered).
				Msg("Panic processando comando Discord")
		}
	}()

	data := event.SlashCommandInteractionData()

	c.log.Info().
		Str("command", data.CommandName()).
		Str("command_path", data.CommandPath()).
		Interface(
			"subcommand",
			data.SubCommandName,
		).
		Interface("options", data.Options).
		Msg("Comando Discord recebido")

	if data.CommandName() != "pal" {
		return
	}

	if data.SubCommandName == nil {
		c.sendInteractionMessage(
			event,
			"⚠️ Nenhum subcomando foi informado.",
		)

		return
	}

	switch *data.SubCommandName {
	case "status":
		c.handleStatusCommand(event)

	case "iniciar":
		c.handleStartCommand(
			event,
			data,
		)

	default:
		c.sendInteractionMessage(
			event,
			"⚠️ Subcomando não reconhecido.",
		)
	}
}

func (c *Client) handleStatusCommand(
	event *events.ApplicationCommandInteractionCreate,
) {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		2*time.Second,
	)
	defer cancel()

	statuses, err := amp.GetServerStatuses(
		ctx,
		c.serverInstances(),
	)
	if err != nil {
		c.log.Error().
			Err(err).
			Msg("Erro consultando status dos servidores")

		c.sendInteractionMessage(
			event,
			"⚠️ Não foi possível consultar o estado dos servidores.",
		)

		return
	}

	c.sendInteractionMessage(
		event,
		c.buildStatusMessage(statuses),
	)
}

func (c *Client) handleStartCommand(
	event *events.ApplicationCommandInteractionCreate,
	data discord.SlashCommandInteractionData,
) {
	err := event.DeferCreateMessage(false)
	if err != nil {
		c.log.Error().
			Err(err).
			Msg("Erro adiando resposta da interação")

		return
	}

	c.log.Info().
		Msg("Interação adiada com sucesso")

	serverKey, exists := data.OptString(
		"servidor",
	)
	if !exists {
		c.updateInteractionMessage(
			event,
			"⚠️ O servidor não foi informado.",
		)

		return
	}

	server, exists := c.findManagedServer(
		serverKey,
	)
	if !exists {
		c.updateInteractionMessage(
			event,
			"⚠️ O servidor selecionado não é reconhecido.",
		)

		return
	}

	releaseOperation, activeOperation, acquired, err :=
		c.acquirePalStartOperation(
			server,
		)
	if err != nil {
		c.log.Error().
			Err(err).
			Str("server", server.Instance.Name).
			Msg("Não foi possível adquirir o bloqueio de /pal iniciar")

		c.updateInteractionMessage(
			event,
			fmt.Sprintf(
				"❌ Não foi possível reservar **%s** para a inicialização.\n"+
					"Erro: `%s`",
				server.DisplayName,
				sanitizeAMPError(err),
			),
		)

		return
	}

	if !acquired {
		c.log.Info().
			Str("server", server.Instance.Name).
			Str("active_operation", activeOperation).
			Msg("/pal iniciar recusado porque a instância está ocupada")

		c.updateInteractionMessage(
			event,
			fmt.Sprintf(
				"⏳ **%s** já possui uma operação em andamento.\n"+
					"Operação atual: `%s`\n"+
					"Aguarde a conclusão antes de iniciar o servidor.",
				server.DisplayName,
				activeOperation,
			),
		)

		return
	}

	releaseOnReturn := true

	defer func() {
		if releaseOnReturn &&
			releaseOperation != nil {
			releaseOperation()
		}
	}()

	statusCtx, statusCancel := context.WithTimeout(
		context.Background(),
		3*time.Second,
	)

	statuses, err := amp.GetServerStatuses(
		statusCtx,
		[]amp.Instance{
			server.Instance,
		},
	)

	statusCancel()

	if err != nil {
		c.log.Error().
			Err(err).
			Str("server", server.Instance.Name).
			Msg("Erro consultando servidor antes da inicialização")

		c.updateInteractionMessage(
			event,
			"⚠️ Não foi possível consultar o estado do servidor.",
		)

		return
	}

	state := statuses[server.Instance.Name]

	switch state {
	case amp.ServerStateOnline:
		c.updateInteractionMessage(
			event,
			fmt.Sprintf(
				"🟢 **%s** já está online e pronto para receber jogadores.",
				server.DisplayName,
			),
		)

		return

	case amp.ServerStateOffline:
		c.updateInteractionMessage(
			event,
			fmt.Sprintf(
				"⚫ **%s** está offline.\n"+
					"A instância AMP `%s` precisa ser iniciada primeiro.",
				server.DisplayName,
				server.Instance.Name,
			),
		)

		return
	}

	c.updateInteractionMessage(
		event,
		fmt.Sprintf(
			"⏳ Solicitando a inicialização de **%s**...",
			server.DisplayName,
		),
	)

	apiCtx, apiCancel := context.WithTimeout(
		context.Background(),
		20*time.Second,
	)

	err = c.ampClient.StartApplication(
		apiCtx,
		server.APIURL,
	)

	apiCancel()

	if err != nil {
		c.log.Error().
			Err(err).
			Str("server", server.Instance.Name).
			Str("api_url", server.APIURL).
			Msg("Erro iniciando servidor pela API AMP")

		c.updateInteractionMessage(
			event,
			fmt.Sprintf(
				"❌ Não foi possível confirmar a inicialização de **%s**.\n"+
					"Verifique o status novamente, pois o AMP pode ter recebido a solicitação.",
				server.DisplayName,
			),
		)

		return
	}

	c.log.Info().
		Str("server", server.Instance.Name).
		Msg("Inicialização solicitada ao AMP")

	c.updateInteractionMessage(
		event,
		fmt.Sprintf(
			"🚀 **%s** está iniciando...\n"+
				"Aguardando o servidor ficar online.",
			server.DisplayName,
		),
	)

	releaseOnReturn = false

	go func() {
		defer releaseOperation()

		c.waitForServerOnline(
			event.ApplicationID(),
			event.Token(),
			server,
		)
	}()
}

func (c *Client) waitForServerOnline(
	applicationID snowflake.ID,
	interactionToken string,
	server managedServer,
) {
	const (
		maximumWait = 2 * time.Minute
		checkEvery  = 3 * time.Second
	)

	timeout := time.NewTimer(maximumWait)
	defer timeout.Stop()

	ticker := time.NewTicker(checkEvery)
	defer ticker.Stop()

	c.log.Info().
		Str("server", server.Instance.Name).
		Dur("maximum_wait", maximumWait).
		Msg("Aguardando servidor ficar online")

	for {
		select {
		case <-timeout.C:
			c.log.Warn().
				Str("server", server.Instance.Name).
				Msg("Tempo limite aguardando servidor ficar online")

			c.updateInteractionMessageByToken(
				applicationID,
				interactionToken,
				fmt.Sprintf(
					"🟡 A inicialização de **%s** foi solicitada, "+
						"mas o servidor ainda não ficou online.\n"+
						"Use `/pal status` para acompanhar.",
					server.DisplayName,
				),
			)

			return

		case <-ticker.C:
			statusCtx, statusCancel := context.WithTimeout(
				context.Background(),
				3*time.Second,
			)

			statuses, err := amp.GetServerStatuses(
				statusCtx,
				[]amp.Instance{
					server.Instance,
				},
			)

			statusCancel()

			if err != nil {
				c.log.Warn().
					Err(err).
					Str("server", server.Instance.Name).
					Msg("Falha temporária verificando inicialização")

				continue
			}

			if statuses[server.Instance.Name] != amp.ServerStateOnline {
				continue
			}

			c.log.Info().
				Str("server", server.Instance.Name).
				Int("game_port", server.Instance.GamePort).
				Msg("Servidor está online")

			c.updateInteractionMessageByToken(
				applicationID,
				interactionToken,
				fmt.Sprintf(
					"✅ **%s está online e funcionando!**\n"+
						"O servidor está pronto para receber jogadores.\n"+
						"Porta: `%d/UDP`",
					server.DisplayName,
					server.Instance.GamePort,
				),
			)

			return
		}
	}
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

func (c *Client) findManagedServer(
	key string,
) (managedServer, bool) {
	for _, server := range c.managedServers {
		if server.Key == key {
			return server, true
		}
	}

	return managedServer{}, false
}

func (c *Client) buildStatusMessage(
	statuses map[string]amp.ServerState,
) string {
	var message strings.Builder

	message.WriteString(
		"🎮 **PalControl — Status dos servidores**\n\n",
	)

	for _, server := range c.managedServers {
		state := statuses[server.Instance.Name]

		switch state {
		case amp.ServerStateOnline:
			_, _ = fmt.Fprintf(
				&message,
				"🟢 **%s**\n"+
					"Status: **Online**\n"+
					"Porta: `%d/UDP`\n\n",
				server.DisplayName,
				server.Instance.GamePort,
			)

		case amp.ServerStateIdle:
			_, _ = fmt.Fprintf(
				&message,
				"🟡 **%s**\n"+
					"Status: **Idle**\n"+
					"Use `/pal iniciar servidor:%s` para iniciar.\n\n",
				server.DisplayName,
				server.DisplayName,
			)

		default:
			_, _ = fmt.Fprintf(
				&message,
				"⚫ **%s**\n"+
					"Status: **Offline**\n"+
					"A instância AMP está desligada.\n\n",
				server.DisplayName,
			)
		}
	}

	return strings.TrimSpace(
		message.String(),
	)
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
