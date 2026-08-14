package i18n

import (
	"fmt"
	"strings"
	"sync/atomic"
)

type Language string

const (
	PortugueseBrazil Language = "pt-BR"
	EnglishUS        Language = "en-US"
)

type Key string

const (
	ConfigInvalid Key = "config.invalid"
	ConfigValid   Key = "config.valid"
	StartupError  Key = "startup.error"
	DiscordError  Key = "discord.error"
)

var catalogs = map[Language]map[Key]string{
	PortugueseBrazil: {
		ConfigInvalid: "Configuração inválida:",
		ConfigValid:   "Configuração válida.",
		StartupError:  "Erro ao iniciar AmpControl:",
		DiscordError:  "Erro Discord:",
	},
	EnglishUS: {
		ConfigInvalid: "Invalid configuration:",
		ConfigValid:   "Configuration is valid.",
		StartupError:  "Could not start AmpControl:",
		DiscordError:  "Discord error:",
	},
}

var current atomic.Value

func init() {
	current.Store(PortugueseBrazil)
}

func Parse(value string) (Language, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "pt", "pt-br", "pt_br", "portuguese", "português", "portugues":
		return PortugueseBrazil, nil
	case "en", "en-us", "en_us", "english":
		return EnglishUS, nil
	default:
		return "", fmt.Errorf("idioma/language inválido/invalid: %q (use pt-BR or en-US)", value)
	}
}

func SetDefault(language Language) {
	if _, exists := catalogs[language]; !exists {
		language = PortugueseBrazil
	}
	current.Store(language)
}

func Default() Language {
	return current.Load().(Language)
}

func T(key Key, arguments ...any) string {
	return For(Default(), key, arguments...)
}

func For(language Language, key Key, arguments ...any) string {
	message, exists := catalogs[language][key]
	if !exists {
		message = catalogs[PortugueseBrazil][key]
	}
	if message == "" {
		return string(key)
	}
	if len(arguments) == 0 {
		return message
	}
	return fmt.Sprintf(message, arguments...)
}
