package idle

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Carlos-Gabryel/ampcontrol/internal/i18n"
)

var ErrServerAlreadyRegistered = errors.New(
	"a instância já está cadastrada no motor de Idle",
)

type ServerRegistration struct {
	Instance    string
	DisplayName string
	Game        string
}

type rawAdditionalServers struct {
	Servers []rawServer `json:"servers"`
}

func LoadCombined(
	basePath string,
	additionalPath string,
) (Config, error) {
	base, err := Load(basePath)
	if err != nil {
		return Config{}, err
	}

	additional, err := loadAdditionalServers(additionalPath)
	if err != nil {
		return Config{}, err
	}

	return mergeAdditionalServers(base, additional)
}

func RegisterAdditionalServer(
	basePath string,
	additionalPath string,
	registration ServerRegistration,
) (Config, error) {
	registration.Instance = strings.TrimSpace(registration.Instance)
	registration.DisplayName = strings.TrimSpace(registration.DisplayName)
	registration.Game = strings.TrimSpace(registration.Game)
	if registration.Instance == "" {
		return Config{}, fmt.Errorf("a instância AMP não foi informada")
	}
	if registration.DisplayName == "" {
		registration.DisplayName = registration.Instance
	}

	combined, err := LoadCombined(basePath, additionalPath)
	if err != nil {
		return Config{}, err
	}
	if _, exists := combined.FindServer(registration.Instance); exists {
		return Config{}, ErrServerAlreadyRegistered
	}

	additional, err := loadRawAdditionalServers(additionalPath)
	if err != nil {
		return Config{}, err
	}

	rawNewServer := rawServer{
		Instance:            registration.Instance,
		DisplayName:         registration.DisplayName,
		Game:                registration.Game,
		Enabled:             true,
		Mode:                ServerModeActive,
		Detector:            DetectorAMPPlayers,
		IdleTimeoutMinutes:  defaultIdleTimeoutMinutes,
		StartupGraceMinutes: defaultStartupGraceMinutes,
	}
	newServer, err := buildServer(rawNewServer, defaultIdleTimeoutMinutes)
	if err != nil {
		return Config{}, err
	}
	additional.Servers = append(additional.Servers, rawNewServer)

	next, err := mergeAdditionalServers(combined, []Server{newServer})
	if err != nil {
		return Config{}, err
	}
	if err := saveRawAdditionalServers(additionalPath, additional); err != nil {
		return Config{}, err
	}

	return next, nil
}

func loadAdditionalServers(path string) ([]Server, error) {
	raw, err := loadRawAdditionalServers(path)
	if err != nil {
		return nil, err
	}

	servers := make([]Server, 0, len(raw.Servers))
	for index, rawServer := range raw.Servers {
		server, buildErr := buildServer(
			rawServer,
			defaultIdleTimeoutMinutes,
		)
		if buildErr != nil {
			return nil, fmt.Errorf(
				i18n.Choose("servidor adicional de índice %d: %w", "additional server at index %d: %w"),
				index,
				buildErr,
			)
		}
		servers = append(servers, server)
	}

	return servers, nil
}

func loadRawAdditionalServers(path string) (rawAdditionalServers, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return rawAdditionalServers{}, errors.New(i18n.Choose("o caminho dos servidores adicionais de Idle não foi configurado", "the additional Idle servers path was not configured"))
	}

	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return rawAdditionalServers{Servers: []rawServer{}}, nil
	}
	if err != nil {
		return rawAdditionalServers{}, fmt.Errorf(
			i18n.Choose("não foi possível abrir os servidores adicionais em %s: %w", "could not open the additional servers at %s: %w"),
			path,
			err,
		)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()

	var raw rawAdditionalServers
	if err := decoder.Decode(&raw); err != nil {
		return rawAdditionalServers{}, fmt.Errorf(
			i18n.Choose("não foi possível interpretar os servidores adicionais em %s: %w", "could not parse the additional servers at %s: %w"),
			path,
			err,
		)
	}
	if err := ensureSingleJSONObject(decoder, path); err != nil {
		return rawAdditionalServers{}, err
	}

	return raw, nil
}

func mergeAdditionalServers(
	base Config,
	additional []Server,
) (Config, error) {
	result := Config{
		CheckInterval: base.CheckInterval,
		Servers:       append([]Server(nil), base.Servers...),
	}
	seen := make(map[string]struct{}, len(result.Servers)+len(additional))
	for _, server := range result.Servers {
		seen[strings.ToLower(strings.TrimSpace(server.Instance))] = struct{}{}
	}

	for _, server := range additional {
		key := strings.ToLower(strings.TrimSpace(server.Instance))
		if _, exists := seen[key]; exists {
			return Config{}, fmt.Errorf(
				i18n.Choose("a instância %q aparece mais de uma vez na configuração combinada de Idle", "instance %q appears more than once in the combined Idle configuration"),
				server.Instance,
			)
		}
		seen[key] = struct{}{}
		result.Servers = append(result.Servers, server)
	}

	return result, nil
}

func saveRawAdditionalServers(
	path string,
	raw rawAdditionalServers,
) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf(
			"não foi possível criar a pasta dos servidores adicionais: %w",
			err,
		)
	}

	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return fmt.Errorf(
			i18n.Choose("não foi possível serializar os servidores adicionais: %w", "could not serialize the additional servers: %w"),
			err,
		)
	}
	data = append(data, '\n')

	temporaryPath := path + ".tmp"
	if err := os.WriteFile(temporaryPath, data, 0o600); err != nil {
		return fmt.Errorf(
			i18n.Choose("não foi possível gravar os servidores adicionais temporários: %w", "could not write the temporary additional servers: %w"),
			err,
		)
	}
	if err := os.Chmod(temporaryPath, 0o600); err != nil {
		return fmt.Errorf(
			i18n.Choose("não foi possível proteger os servidores adicionais temporários: %w", "could not protect the temporary additional servers: %w"),
			err,
		)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf(
			i18n.Choose("não foi possível publicar os servidores adicionais: %w", "could not publish the additional servers: %w"),
			err,
		)
	}

	return nil
}
