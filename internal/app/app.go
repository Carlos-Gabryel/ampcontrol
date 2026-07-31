package app

import (
	"context"

	"github.com/alabamaamp/palcontrol/internal/config"
	discordClient "github.com/alabamaamp/palcontrol/internal/discord"
	"github.com/alabamaamp/palcontrol/internal/logger"
)

type App struct {
	Discord *discordClient.Client
}

func New() (*App, error) {

	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}

	log := logger.New(cfg.LogLevel)

	discord, err := discordClient.New(
		cfg.DiscordToken,
		log,
	)

	if err != nil {
		return nil, err
	}

	return &App{
		Discord: discord,
	}, nil
}

func (a *App) Start(ctx context.Context) error {

	return a.Discord.Start(ctx)
}
