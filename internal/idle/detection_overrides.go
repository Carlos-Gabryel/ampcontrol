package idle

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
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
		return nil, fmt.Errorf("o caminho dos overrides de detecção não foi configurado")
	}

	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return []DetectionOverride{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("não foi possível abrir os overrides de detecção em %s: %w", path, err)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var raw detectionOverridesFile
	if err := decoder.Decode(&raw); err != nil {
		return nil, fmt.Errorf("não foi possível interpretar os overrides de detecção em %s: %w", path, err)
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
				"a instância %q dos overrides de detecção não existe na configuração de Idle",
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
		return fmt.Errorf("a instância do override de detecção não foi informada")
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
			return nil, fmt.Errorf("há um override de detecção sem instância")
		}
		if !isImplementedDetector(override.Detector) {
			return nil, fmt.Errorf(
				"o detector %q do override da instância %s não foi implementado",
				override.Detector,
				override.Instance,
			)
		}
		if override.FallbackDetector != "" &&
			!isImplementedDetector(override.FallbackDetector) {
			return nil, fmt.Errorf(
				"o detector fallback %q do override da instância %s não foi implementado",
				override.FallbackDetector,
				override.Instance,
			)
		}
		if override.FallbackDetector == override.Detector {
			return nil, fmt.Errorf(
				"a instância %s usa o mesmo detector como primário e fallback",
				override.Instance,
			)
		}
		key := strings.ToLower(override.Instance)
		if _, exists := byKey[key]; exists {
			return nil, fmt.Errorf("a instância %s possui mais de um override de detecção", override.Instance)
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
			"o fallback %q da instância %s não é gerenciável pelo Discord",
			server.FallbackDetector,
			server.Instance,
		)
	}
	if detectorUsesRCON(server.FallbackDetector) &&
		(strings.TrimSpace(server.RCONAddress) == "" ||
			(strings.TrimSpace(server.RCONCredential) == "" &&
				strings.TrimSpace(server.RCONPasswordEnv) == "")) {
		return fmt.Errorf(
			"a instância %s não possui endereço e credencial de senha RCON configurados",
			server.Instance,
		)
	}
	return nil
}

func saveDetectionOverrides(path string, overrides []DetectionOverride) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("o caminho dos overrides de detecção não foi configurado")
	}
	normalized, err := normalizeDetectionOverrides(overrides)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("não foi possível criar a pasta dos overrides de detecção: %w", err)
	}

	data, err := json.MarshalIndent(detectionOverridesFile{Servers: normalized}, "", "  ")
	if err != nil {
		return fmt.Errorf("não foi possível serializar os overrides de detecção: %w", err)
	}
	data = append(data, '\n')
	temporaryPath := path + ".tmp"
	if err := os.WriteFile(temporaryPath, data, 0o600); err != nil {
		return fmt.Errorf("não foi possível gravar os overrides de detecção temporários: %w", err)
	}
	if err := os.Chmod(temporaryPath, 0o600); err != nil {
		return fmt.Errorf("não foi possível proteger os overrides de detecção temporários: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("não foi possível publicar os overrides de detecção: %w", err)
	}
	return nil
}
