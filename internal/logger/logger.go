package logger

import (
	"bytes"
	"encoding/json"
	"io"
	"os"

	"github.com/Carlos-Gabryel/ampcontrol/internal/i18n"
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

	log := zerolog.New(localizedJSONWriter{target: os.Stdout}).
		With().
		Timestamp().
		Logger()

	return log
}

type localizedJSONWriter struct {
	target io.Writer
}

func (w localizedJSONWriter) Write(data []byte) (int, error) {
	if i18n.Default() != i18n.EnglishUS {
		return w.target.Write(data)
	}
	var event map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(data), &event); err != nil {
		return w.target.Write(data)
	}
	for _, field := range []string{zerolog.MessageFieldName, zerolog.ErrorFieldName} {
		if value, ok := event[field].(string); ok {
			event[field] = i18n.Text(value)
		}
	}
	translated, err := json.Marshal(event)
	if err != nil {
		return w.target.Write(data)
	}
	translated = append(translated, '\n')
	if _, err := w.target.Write(translated); err != nil {
		return 0, err
	}
	return len(data), nil
}
