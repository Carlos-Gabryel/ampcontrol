package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	DiscordToken                 string
	DiscordNotificationChannelID string
	AMPUsername                  string
	AMPPassword                  string
	AlamamaRCONPassword          string
	KalagaRCONPassword           string
	IdleTimeout                  time.Duration
	LogLevel                     string
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		DiscordToken: strings.TrimSpace(
			os.Getenv("DISCORD_TOKEN"),
		),
		DiscordNotificationChannelID: strings.TrimSpace(
			os.Getenv("DISCORD_NOTIFICATION_CHANNEL_ID"),
		),
		AMPUsername: strings.TrimSpace(
			os.Getenv("AMP_USERNAME"),
		),
		AMPPassword: os.Getenv("AMP_PASSWORD"),
		AlamamaRCONPassword: os.Getenv(
			"ALAMAMA_RCON_PASSWORD",
		),
		KalagaRCONPassword: os.Getenv(
			"KALAGA_RCON_PASSWORD",
		),
		LogLevel: strings.TrimSpace(
			os.Getenv("LOG_LEVEL"),
		),
	}

	if cfg.DiscordToken == "" {
		return nil, fmt.Errorf(
			"DISCORD_TOKEN não foi configurado",
		)
	}

	if cfg.DiscordNotificationChannelID == "" {
		return nil, fmt.Errorf(
			"DISCORD_NOTIFICATION_CHANNEL_ID não foi configurado",
		)
	}

	_, err := strconv.ParseUint(
		cfg.DiscordNotificationChannelID,
		10,
		64,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"DISCORD_NOTIFICATION_CHANNEL_ID é inválido: %w",
			err,
		)
	}

	if cfg.AMPUsername == "" {
		return nil, fmt.Errorf(
			"AMP_USERNAME não foi configurado",
		)
	}

	if cfg.AMPPassword == "" {
		return nil, fmt.Errorf(
			"AMP_PASSWORD não foi configurado",
		)
	}

	if cfg.AlamamaRCONPassword == "" {
		return nil, fmt.Errorf(
			"ALAMAMA_RCON_PASSWORD não foi configurado",
		)
	}

	if cfg.KalagaRCONPassword == "" {
		return nil, fmt.Errorf(
			"KALAGA_RCON_PASSWORD não foi configurado",
		)
	}

	idleTimeoutMinutes := 10

	rawIdleTimeout := strings.TrimSpace(
		os.Getenv("IDLE_TIMEOUT_MINUTES"),
	)

	if rawIdleTimeout != "" {
		idleTimeoutMinutes, err = strconv.Atoi(
			rawIdleTimeout,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"IDLE_TIMEOUT_MINUTES é inválido: %w",
				err,
			)
		}
	}

	if idleTimeoutMinutes <= 0 {
		return nil, fmt.Errorf(
			"IDLE_TIMEOUT_MINUTES precisa ser maior que zero",
		)
	}

	cfg.IdleTimeout = time.Duration(
		idleTimeoutMinutes,
	) * time.Minute

	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}

	return cfg, nil
}
