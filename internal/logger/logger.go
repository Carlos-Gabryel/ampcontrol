package logger

import (
	"os"

	"github.com/rs/zerolog"
)

func New(level string) zerolog.Logger {

	switch level {

	case "debug":
		zerolog.SetGlobalLevel(zerolog.DebugLevel)

	case "warn":
		zerolog.SetGlobalLevel(zerolog.WarnLevel)

	case "error":
		zerolog.SetGlobalLevel(zerolog.ErrorLevel)

	default:
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}

	log := zerolog.New(os.Stdout).
		With().
		Timestamp().
		Logger()

	return log
}
