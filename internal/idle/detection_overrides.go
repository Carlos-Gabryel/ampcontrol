package idle

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Carlos-Gabryel/ampcontrol/internal/i18n"
)

type DetectionOverride struct {
	Instance         string   `json:"instance"`
	Detector         Detector `json:"detector"`
	FallbackDetector Detector `json:"fallback_detector,omitempty"`
}

type detectionOverridesFile struct {
	Servers []DetectionOverride `json:"servers"`
}

func LoadCombinedWithDetectionOverrides(
	basePath string,
	additionalPath string,
	overridesPath string,
) (Config, error) {
	config, err := LoadCombined(basePath, additionalPath)
	if err != nil {
		return Config{}, err
	}

	overrides, err := LoadDetectionOverrides(overridesPath)
	if err != nil {
		return Config{}, err
	}

	return ApplyDetectionOverrides(config, overrides)
}

func LoadDetectionOverrides(path string) ([]DetectionOverride, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New(i18n.Choose("o caminho dos overrides de detecção não foi configurado", "the detection overrides path was not configured"))
	}

	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return []DetectionOverride{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf(i18n.Choose("não foi possível abrir os overrides de detecção em %s: %w", "could not open the detection overrides at %s: %w"), path, err)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var raw detectionOverridesFile
	if err := decoder.Decode(&raw); err != nil {
		return nil, fmt.Errorf(i18n.Choose("não foi possível interpretar os overrides de detecção em %s: %w", "could not parse the detection overrides at %s: %w"), path, err)
	}
	if err := ensureSingleJSONObject(decoder, path); err != nil {
		return nil, err
	}

	return normalizeDetectionOverrides(raw.Servers)
}

func ApplyDetectionOverrides(
	config Config,
	overrides []DetectionOverride,
) (Config, error) {
	normalized, err := normalizeDetectionOverrides(overrides)
	if err != nil {
		return Config{}, err
	}

	result := Config{
		CheckInterval: config.CheckInterval,
		Servers:       append([]Server(nil), config.Servers...),
	}
	for _, override := range normalized {
		index := -1
		for candidate := range result.Servers {
			if strings.EqualFold(result.Servers[candidate].Instance, override.Instance) {
				index = candidate
				break
			}
		}
		if index < 0 {
			return Config{}, fmt.Errorf(
				i18n.Choose("a instância %q dos overrides de detecção não existe na configuração de Idle", "instance %q in the detection overrides does not exist in the Idle configuration"),
				override.Instance,
			)
		}

		server := result.Servers[index]
		server.Detector = override.Detector
		server.FallbackDetector = override.FallbackDetector
		if err := validateDetectionOverrideServer(server); err != nil {
			return Config{}, err
		}
		result.Servers[index] = server
	}

	return result, nil
}

func SetDetectionOverride(
	path string,
	override DetectionOverride,
) error {
	overrides, err := LoadDetectionOverrides(path)
	if err != nil {
		return err
	}

	override.Instance = strings.TrimSpace(override.Instance)
	if override.Instance == "" {
		return errors.New(i18n.Choose("a instância do override de detecção não foi informada", "the detection override instance was not provided"))
	}

	key := strings.ToLower(override.Instance)
	found := false
	for index := range overrides {
		if strings.ToLower(overrides[index].Instance) == key {
			overrides[index] = override
			found = true
			break
		}
	}
	if !found {
		overrides = append(overrides, override)
	}

	return saveDetectionOverrides(path, overrides)
}

func RemoveDetectionOverride(path string, instance string) (bool, error) {
	overrides, err := LoadDetectionOverrides(path)
	if err != nil {
		return false, err
	}

	key := strings.ToLower(strings.TrimSpace(instance))
	next := make([]DetectionOverride, 0, len(overrides))
	removed := false
	for _, override := range overrides {
		if strings.ToLower(override.Instance) == key {
			removed = true
			continue
		}
		next = append(next, override)
	}
	if !removed {
		return false, nil
	}

	return true, saveDetectionOverrides(path, next)
}

func normalizeDetectionOverrides(
	overrides []DetectionOverride,
) ([]DetectionOverride, error) {
	byKey := make(map[string]DetectionOverride, len(overrides))
	for _, override := range overrides {
		override.Instance = strings.TrimSpace(override.Instance)
		if override.Instance == "" {
			return nil, errors.New(i18n.Choose("há um override de detecção sem instância", "there is a detection override without an instance"))
		}
		if !isImplementedDetector(override.Detector) {
			return nil, fmt.Errorf(
				i18n.Choose("o detector %q do override da instância %s não foi implementado", "detector %q in the override for instance %s is not implemented"),
				override.Detector,
				override.Instance,
			)
		}
		if override.FallbackDetector != "" &&
			!isImplementedDetector(override.FallbackDetector) {
			return nil, fmt.Errorf(
				i18n.Choose("o detector fallback %q do override da instância %s não foi implementado", "fallback detector %q in the override for instance %s is not implemented"),
				override.FallbackDetector,
				override.Instance,
			)
		}
		if override.FallbackDetector == override.Detector {
			return nil, fmt.Errorf(
				i18n.Choose("a instância %s usa o mesmo detector como primário e fallback", "instance %s uses the same detector as primary and fallback"),
				override.Instance,
			)
		}
		key := strings.ToLower(override.Instance)
		if _, exists := byKey[key]; exists {
			return nil, fmt.Errorf(i18n.Choose("a instância %s possui mais de um override de detecção", "instance %s has more than one detection override"), override.Instance)
		}
		byKey[key] = override
	}

	result := make([]DetectionOverride, 0, len(byKey))
	for _, override := range byKey {
		result = append(result, override)
	}
	sort.Slice(result, func(left, right int) bool {
		return strings.ToLower(result[left].Instance) < strings.ToLower(result[right].Instance)
	})
	return result, nil
}

func validateDetectionOverrideServer(server Server) error {
	if server.Detector != DetectorAMPPlayers {
		return fmt.Errorf(
			"a instância %s precisa manter a API AMP como detector principal",
			server.Instance,
		)
	}
	if server.FallbackDetector != "" &&
		server.FallbackDetector != DetectorPalworldRCON &&
		server.FallbackDetector != DetectorProjectZomboidRCON {
		return fmt.Errorf(
			i18n.Choose("o fallback %q da instância %s não é gerenciável pelo Discord", "fallback %q of instance %s cannot be managed from Discord"),
			server.FallbackDetector,
			server.Instance,
		)
	}
	if detectorUsesRCON(server.FallbackDetector) &&
		(strings.TrimSpace(server.RCONAddress) == "" ||
			(strings.TrimSpace(server.RCONCredential) == "" &&
				strings.TrimSpace(server.RCONPasswordEnv) == "")) {
		return fmt.Errorf(
			i18n.Choose("a instância %s não possui endereço e credencial de senha RCON configurados", "instance %s does not have an RCON address and password credential configured"),
			server.Instance,
		)
	}
	return nil
}

func saveDetectionOverrides(path string, overrides []DetectionOverride) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New(i18n.Choose("o caminho dos overrides de detecção não foi configurado", "the detection overrides path was not configured"))
	}
	normalized, err := normalizeDetectionOverrides(overrides)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf(i18n.Choose("não foi possível criar a pasta dos overrides de detecção: %w", "could not create the detection overrides folder: %w"), err)
	}

	data, err := json.MarshalIndent(detectionOverridesFile{Servers: normalized}, "", "  ")
	if err != nil {
		return fmt.Errorf(i18n.Choose("não foi possível serializar os overrides de detecção: %w", "could not serialize the detection overrides: %w"), err)
	}
	data = append(data, '\n')
	temporaryPath := path + ".tmp"
	if err := os.WriteFile(temporaryPath, data, 0o600); err != nil {
		return fmt.Errorf(i18n.Choose("não foi possível gravar os overrides de detecção temporários: %w", "could not write the temporary detection overrides: %w"), err)
	}
	if err := os.Chmod(temporaryPath, 0o600); err != nil {
		return fmt.Errorf(i18n.Choose("não foi possível proteger os overrides de detecção temporários: %w", "could not protect the temporary detection overrides: %w"), err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf(i18n.Choose("não foi possível publicar os overrides de detecção: %w", "could not publish the detection overrides: %w"), err)
	}
	return nil
}
