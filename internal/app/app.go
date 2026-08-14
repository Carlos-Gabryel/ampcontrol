package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/alabamaamp/ampcontrol/internal/amp"
	"github.com/alabamaamp/ampcontrol/internal/config"
	discordClient "github.com/alabamaamp/ampcontrol/internal/discord"
	"github.com/alabamaamp/ampcontrol/internal/idle"
	"github.com/alabamaamp/ampcontrol/internal/logger"
	"github.com/alabamaamp/ampcontrol/internal/operation"
)

type App struct {
	Discord      *discordClient.Client
	IdleObserver *idleObserver
	Operations   *operation.Manager
}

func New() (*App, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}

	log := logger.New(
		cfg.LogLevel,
	)

	ampAPIClient := amp.NewAPIClient(
		cfg.AMPUsername,
		cfg.AMPPassword,
	)

	operationManager := operation.NewManager()

	idleConfig, err := idle.LoadCombinedWithDetectionOverrides(
		idleObserverConfigPath,
		idleAdditionalServersPath,
		idleDetectionOverridesPath,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"não foi possível carregar a configuração compartilhada do Idle: %w",
			err,
		)
	}

	gameOverrides := make(map[string]string)
	for _, server := range idleConfig.Servers {
		if game := strings.TrimSpace(server.Game); game != "" {
			gameOverrides[server.Instance] = game
		}
	}

	playerCountResolver, err := newDashboardPlayerCountResolver(
		ampAPIClient,
		idleConfig,
	)
	if err != nil {
		return nil, err
	}

	discord, err := discordClient.New(
		cfg.DiscordToken,
		ampAPIClient,
		discordClient.ClientConfig{
			GuildID:               cfg.DiscordGuildID,
			NotificationChannelID: cfg.DiscordNotificationChannelID,
			AuditChannelID:        cfg.DiscordAuditChannelID,
			OwnerUserID:           cfg.DiscordOwnerUserID,
			NotificationTTL:       cfg.DiscordNotificationTTL,
			StatusRefreshInterval: cfg.DiscordStatusRefreshInterval,
			CommandUserCooldown:   cfg.DiscordCommandUserCooldown,
			CommandServerCooldown: cfg.DiscordCommandServerCooldown,
			ADSURL:                cfg.AMPADSURL,
			AMPPublicURL:          cfg.AMPPublicURL,
			GameServerAddress:     cfg.AMPGameServerAddress,
			StatusStatePath:       "data/discord_status.json",
			PreferencesPath:       "data/discord_preferences.json",
			GameOverrides:         gameOverrides,
			PlayerCountResolver:   playerCountResolver,
			IdleRegisteredInstances: idleRegisteredInstanceNames(
				idleConfig,
			),
		},
		log,
	)
	if err != nil {
		return nil, err
	}

	if err := discord.SetOperationManager(
		operationManager,
	); err != nil {
		return nil, err
	}

	idleObserver, err := newIdleObserver(
		ampAPIClient,
		operationManager,
		discord,
		idleConfig,
		log,
	)
	if err != nil {
		return nil, err
	}
	if err := discord.SetIdleServerRegistrar(idleObserver); err != nil {
		return nil, err
	}
	idleObserver.SetConfigChangedHandler(playerCountResolver.ReplaceConfig)

	discord.EnableAMPCommandHandling()

	return &App{
		Discord:      discord,
		IdleObserver: idleObserver,
		Operations:   operationManager,
	}, nil
}

func idleRegisteredInstanceNames(config idle.Config) []string {
	names := make([]string, 0, len(config.Servers))
	for _, server := range config.Servers {
		names = append(names, server.Instance)
	}
	return names
}

func (a *App) Start(
	ctx context.Context,
) error {
	if err := a.Discord.Start(ctx); err != nil {
		return err
	}

	if a.IdleObserver != nil {
		go a.IdleObserver.Run(
			ctx,
		)
	}

	return nil
}
