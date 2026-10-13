package idle

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Carlos-Gabryel/ampcontrol/internal/i18n"
)

const (
	MinIdleTimeoutMinutes = 1
	MaxIdleTimeoutMinutes = 1440
)

type idleTimeoutOverrideFile struct {
	IdleTimeoutMinutes int `json:"idle_timeout_minutes"`
}

func validateIdleTimeoutMinutes(minutes int) error {
	if minutes < MinIdleTimeoutMinutes || minutes > MaxIdleTimeoutMinutes {
		return fmt.Errorf(
			i18n.Choose("o tempo de Idle deve ficar entre %d e %d minutos", "the Idle timeout must be between %d and %d minutes"),
			MinIdleTimeoutMinutes,
			MaxIdleTimeoutMinutes,
		)
	}
	return nil
}

// LoadIdleTimeoutOverride devolve o tempo global de Idle em minutos, ou 0 quando não há override.
func LoadIdleTimeoutOverride(path string) (int, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return 0, errors.New(i18n.Choose("o caminho do override de tempo de Idle não foi configurado", "the Idle timeout override path was not configured"))
	}

	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf(i18n.Choose("não foi possível abrir o override de tempo de Idle em %s: %w", "could not open the Idle timeout override at %s: %w"), path, err)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var raw idleTimeoutOverrideFile
	if err := decoder.Decode(&raw); err != nil {
		return 0, fmt.Errorf(i18n.Choose("não foi possível interpretar o override de tempo de Idle em %s: %w", "could not parse the Idle timeout override at %s: %w"), path, err)
	}
	if err := ensureSingleJSONObject(decoder, path); err != nil {
		return 0, err
	}
	if raw.IdleTimeoutMinutes == 0 {
		return 0, nil
	}
	if err := validateIdleTimeoutMinutes(raw.IdleTimeoutMinutes); err != nil {
		return 0, err
	}
	return raw.IdleTimeoutMinutes, nil
}

// SaveIdleTimeoutOverride grava atomicamente o tempo global de Idle.
func SaveIdleTimeoutOverride(path string, minutes int) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New(i18n.Choose("o caminho do override de tempo de Idle não foi configurado", "the Idle timeout override path was not configured"))
	}
	if err := validateIdleTimeoutMinutes(minutes); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf(i18n.Choose("não foi possível criar a pasta do override de tempo de Idle: %w", "could not create the Idle timeout override folder: %w"), err)
	}

	data, err := json.MarshalIndent(idleTimeoutOverrideFile{IdleTimeoutMinutes: minutes}, "", "  ")
	if err != nil {
		return fmt.Errorf(i18n.Choose("não foi possível serializar o override de tempo de Idle: %w", "could not serialize the Idle timeout override: %w"), err)
	}
	data = append(data, '\n')
	temporaryPath := path + ".tmp"
	if err := os.WriteFile(temporaryPath, data, 0o600); err != nil {
		return fmt.Errorf(i18n.Choose("não foi possível gravar o override de tempo de Idle temporário: %w", "could not write the temporary Idle timeout override: %w"), err)
	}
	if err := os.Chmod(temporaryPath, 0o600); err != nil {
		return fmt.Errorf(i18n.Choose("não foi possível proteger o override de tempo de Idle temporário: %w", "could not protect the temporary Idle timeout override: %w"), err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf(i18n.Choose("não foi possível publicar o override de tempo de Idle: %w", "could not publish the Idle timeout override: %w"), err)
	}
	return nil
}

// ApplyIdleTimeoutOverride aplica o tempo global a todos os servidores; minutes <= 0 não altera nada.
func ApplyIdleTimeoutOverride(config Config, minutes int) Config {
	result := Config{
		CheckInterval: config.CheckInterval,
		Servers:       append([]Server(nil), config.Servers...),
	}
	if minutes <= 0 {
		return result
	}
	timeout := time.Duration(minutes) * time.Minute
	for index := range result.Servers {
		result.Servers[index].IdleTimeout = timeout
	}
	return result
}

// LoadCombinedWithOverrides carrega a configuração combinada com overrides de detecção e de tempo.
func LoadCombinedWithOverrides(
	basePath string,
	additionalPath string,
	detectionOverridesPath string,
	idleTimeoutOverridePath string,
) (Config, error) {
	config, err := LoadCombinedWithDetectionOverrides(basePath, additionalPath, detectionOverridesPath)
	if err != nil {
		return Config{}, err
	}
	minutes, err := LoadIdleTimeoutOverride(idleTimeoutOverridePath)
	if err != nil {
		return Config{}, err
	}
	return ApplyIdleTimeoutOverride(config, minutes), nil
}
