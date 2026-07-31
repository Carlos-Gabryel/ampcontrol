package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	DiscordToken string
	AMPUrl       string
	AMPUsername  string
	AMPPassword  string
	LogLevel     string
}

func Load() (*Config, error) {
	// Carrega o arquivo .env (caso exista)
	_ = godotenv.Load()

	cfg := &Config{
		DiscordToken: os.Getenv("DISCORD_TOKEN"),
		AMPUrl:       os.Getenv("AMP_URL"),
		AMPUsername:  os.Getenv("AMP_USERNAME"),
		AMPPassword:  os.Getenv("AMP_PASSWORD"),
		LogLevel:     os.Getenv("LOG_LEVEL"),
	}

	if cfg.DiscordToken == "" {
		return nil, fmt.Errorf("DISCORD_TOKEN não foi configurado")
	}

	if cfg.AMPUrl == "" {
		return nil, fmt.Errorf("AMP_URL não foi configurado")
	}

	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}

	return cfg, nil
}
