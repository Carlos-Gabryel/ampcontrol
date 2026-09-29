package idle

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/Carlos-Gabryel/ampcontrol/internal/i18n"

	"github.com/Carlos-Gabryel/ampcontrol/internal/secret"
)

const (
	defaultCheckIntervalMinutes = 0
	defaultCheckIntervalSeconds = 30
	defaultIdleTimeoutMinutes   = 15
	defaultStartupGraceMinutes  = 5
)

type Detector string

const (
	DetectorPalworldRCON       Detector = "palworld_rcon"
	DetectorProjectZomboidRCON Detector = "project_zomboid_rcon"
	DetectorMinecraftRCON      Detector = "minecraft_rcon"
	DetectorSourceQuery        Detector = "source_query"
	DetectorAMPPlayers         Detector = "amp_players"
)

type ServerMode string

const (
	ServerModeObserve ServerMode = "observe"
	ServerModeActive  ServerMode = "active"
)

type Config struct {
	CheckInterval time.Duration
	Servers       []Server
}

type Server struct {
	Instance         string
	DisplayName      string
	Game             string
	Enabled          bool
	Mode             ServerMode
	Detector         Detector
	FallbackDetector Detector
	IdleTimeout      time.Duration
	StartupGrace     time.Duration
	RCONAddress      string
	RCONPasswordEnv  string
	RCONCredential   string
}

type rawConfig struct {
	CheckIntervalSeconds      int         `json:"check_interval_seconds"`
	DefaultIdleTimeoutMinutes int         `json:"default_idle_timeout_minutes"`
	Servers                   []rawServer `json:"servers"`
}

type rawServer struct {
	Instance            string         `json:"instance"`
	DisplayName         string         `json:"display_name"`
	Game                string         `json:"game"`
	Enabled             bool           `json:"enabled"`
	Mode                ServerMode     `json:"mode"`
	Detector            Detector       `json:"detector"`
	FallbackDetector    Detector       `json:"fallback_detector"`
	IdleTimeoutMinutes  int            `json:"idle_timeout_minutes"`
	StartupGraceMinutes int            `json:"startup_grace_minutes"`
	RCON                *rawRCONConfig `json:"rcon"`
}

type rawRCONConfig struct {
	Address            string `json:"address"`
	PasswordCredential string `json:"password_credential"`
	PasswordEnv        string `json:"password_env"`
}

func Load(
	path string,
) (Config, error) {
	path = strings.TrimSpace(
		path,
	)

	if path == "" {
		return Config{}, errors.New(i18n.Choose("o caminho do arquivo de configuração de Idle não foi informado", "the Idle configuration file path was not provided"))
	}

	file, err := os.Open(
		path,
	)
	if err != nil {
		return Config{}, fmt.Errorf(
			i18n.Choose("não foi possível abrir a configuração de Idle em %s: %w", "could not open the Idle configuration at %s: %w"),
			path,
			err,
		)
	}
	defer file.Close()

	decoder := json.NewDecoder(
		file,
	)

	decoder.DisallowUnknownFields()

	var raw rawConfig

	if err := decoder.Decode(&raw); err != nil {
		return Config{}, fmt.Errorf(
			i18n.Choose("não foi possível interpretar a configuração de Idle em %s: %w", "could not parse the Idle configuration at %s: %w"),
			path,
			err,
		)
	}

	if err := ensureSingleJSONObject(
		decoder,
		path,
	); err != nil {
		return Config{}, err
	}

	return buildConfig(
		raw,
	)
}

func ensureSingleJSONObject(
	decoder *json.Decoder,
	path string,
) error {
	var extra any

	err := decoder.Decode(
		&extra,
	)

	if err == io.EOF {
		return nil
	}

	if err != nil {
		return fmt.Errorf(
			i18n.Choose("há conteúdo inválido depois da configuração de Idle em %s: %w", "there is invalid content after the Idle configuration at %s: %w"),
			path,
			err,
		)
	}

	return fmt.Errorf(
		i18n.Choose("o arquivo %s contém mais de um objeto JSON", "file %s contains more than one JSON object"),
		path,
	)
}

func buildConfig(
	raw rawConfig,
) (Config, error) {
	checkIntervalSeconds := raw.CheckIntervalSeconds

	if checkIntervalSeconds == defaultCheckIntervalMinutes {
		checkIntervalSeconds = defaultCheckIntervalSeconds
	}

	if checkIntervalSeconds <= 0 {
		return Config{}, fmt.Errorf(
			"check_interval_seconds precisa ser maior que zero",
		)
	}

	defaultTimeoutMinutes := raw.DefaultIdleTimeoutMinutes

	if defaultTimeoutMinutes == 0 {
		defaultTimeoutMinutes = defaultIdleTimeoutMinutes
	}

	if defaultTimeoutMinutes <= 0 {
		return Config{}, fmt.Errorf(
			"default_idle_timeout_minutes precisa ser maior que zero",
		)
	}

	config := Config{
		CheckInterval: time.Duration(
			checkIntervalSeconds,
		) * time.Second,
		Servers: make(
			[]Server,
			0,
			len(raw.Servers),
		),
	}

	instances := make(
		map[string]struct{},
		len(raw.Servers),
	)

	for index, rawServer := range raw.Servers {
		server, err := buildServer(
			rawServer,
			defaultTimeoutMinutes,
		)
		if err != nil {
			return Config{}, fmt.Errorf(
				i18n.Choose("servidor de índice %d: %w", "server at index %d: %w"),
				index,
				err,
			)
		}

		instanceKey := strings.ToLower(
			server.Instance,
		)

		if _, exists := instances[instanceKey]; exists {
			return Config{}, fmt.Errorf(
				i18n.Choose("a instância %q aparece mais de uma vez na configuração de Idle", "instance %q appears more than once in the Idle configuration"),
				server.Instance,
			)
		}

		instances[instanceKey] = struct{}{}

		config.Servers = append(
			config.Servers,
			server,
		)
	}

	return config, nil
}

func buildServer(
	raw rawServer,
	defaultTimeoutMinutes int,
) (Server, error) {
	instance := strings.TrimSpace(
		raw.Instance,
	)

	if instance == "" {
		return Server{}, errors.New(i18n.Choose("o nome da instância AMP não foi informado", "the AMP instance name was not provided"))
	}

	displayName := strings.TrimSpace(
		raw.DisplayName,
	)

	if displayName == "" {
		displayName = instance
	}

	mode := raw.Mode

	if mode == "" {
		mode = ServerModeObserve
	}

	if !isKnownServerMode(mode) {
		return Server{}, fmt.Errorf(
			i18n.Choose("o modo de Idle %q da instância %s não é reconhecido", "Idle mode %q of instance %s is not recognized"),
			mode,
			instance,
		)
	}

	idleTimeoutMinutes := raw.IdleTimeoutMinutes

	if idleTimeoutMinutes == 0 {
		idleTimeoutMinutes = defaultTimeoutMinutes
	}

	if idleTimeoutMinutes <= 0 {
		return Server{}, fmt.Errorf(
			i18n.Choose("idle_timeout_minutes da instância %s precisa ser maior que zero", "idle_timeout_minutes of instance %s must be greater than zero"),
			instance,
		)
	}

	startupGraceMinutes := raw.StartupGraceMinutes

	if startupGraceMinutes == 0 {
		startupGraceMinutes = defaultStartupGraceMinutes
	}

	if startupGraceMinutes <= 0 {
		return Server{}, fmt.Errorf(
			i18n.Choose("startup_grace_minutes da instância %s precisa ser maior que zero", "startup_grace_minutes of instance %s must be greater than zero"),
			instance,
		)
	}

	server := Server{
		Instance:         instance,
		DisplayName:      displayName,
		Game:             strings.TrimSpace(raw.Game),
		Enabled:          raw.Enabled,
		Mode:             mode,
		Detector:         raw.Detector,
		FallbackDetector: raw.FallbackDetector,
		IdleTimeout: time.Duration(
			idleTimeoutMinutes,
		) * time.Minute,
		StartupGrace: time.Duration(
			startupGraceMinutes,
		) * time.Minute,
	}

	if !server.Enabled {
		if server.Detector != "" &&
			!isKnownDetector(server.Detector) {
			return Server{}, fmt.Errorf(
				i18n.Choose("o detector %q da instância %s não é reconhecido", "detector %q of instance %s is not recognized"),
				server.Detector,
				instance,
			)
		}

		if server.FallbackDetector != "" &&
			!isKnownDetector(server.FallbackDetector) {
			return Server{}, fmt.Errorf(
				i18n.Choose("o detector fallback %q da instância %s não é reconhecido", "fallback detector %q of instance %s is not recognized"),
				server.FallbackDetector,
				instance,
			)
		}

		return server, nil
	}

	if !isImplementedDetector(server.Detector) {
		if server.Detector == "" {
			return Server{}, fmt.Errorf(
				i18n.Choose("a instância %s está habilitada, mas nenhum detector foi informado", "instance %s is enabled, but no detector was provided"),
				instance,
			)
		}

		return Server{}, fmt.Errorf(
			i18n.Choose("o detector %q da instância %s ainda não foi implementado", "detector %q of instance %s is not implemented yet"),
			server.Detector,
			instance,
		)
	}

	if server.FallbackDetector != "" {
		if server.FallbackDetector == server.Detector {
			return Server{}, fmt.Errorf(
				i18n.Choose("a instância %s usa o mesmo detector como primário e fallback", "instance %s uses the same detector as primary and fallback"),
				instance,
			)
		}

		if !isImplementedDetector(server.FallbackDetector) {
			return Server{}, fmt.Errorf(
				i18n.Choose("o detector fallback %q da instância %s ainda não foi implementado", "fallback detector %q of instance %s is not implemented yet"),
				server.FallbackDetector,
				instance,
			)
		}
	}

	if detectorUsesRCON(server.Detector) ||
		detectorUsesRCON(server.FallbackDetector) {
		if raw.RCON == nil {
			return Server{}, fmt.Errorf(
				i18n.Choose("a instância %s usa um detector RCON, mas não possui configuração RCON", "instance %s uses an RCON detector but has no RCON configuration"),
				instance,
			)
		}

		server.RCONAddress = strings.TrimSpace(
			raw.RCON.Address,
		)

		server.RCONPasswordEnv = strings.TrimSpace(
			raw.RCON.PasswordEnv,
		)
		server.RCONCredential = strings.TrimSpace(
			raw.RCON.PasswordCredential,
		)

		if server.RCONAddress == "" {
			return Server{}, fmt.Errorf(
				i18n.Choose("o endereço RCON da instância %s não foi informado", "the RCON address of instance %s was not provided"),
				instance,
			)
		}

		if server.RCONPasswordCredentialName() == "" && server.RCONPasswordEnv == "" {
			return Server{}, fmt.Errorf(
				i18n.Choose("a credencial de senha RCON da instância %s não foi informada", "the RCON password credential of instance %s was not provided"),
				instance,
			)
		}
	}

	return server, nil
}

func isKnownServerMode(
	mode ServerMode,
) bool {
	switch mode {
	case ServerModeObserve,
		ServerModeActive:
		return true

	default:
		return false
	}
}

func isKnownDetector(
	detector Detector,
) bool {
	switch detector {
	case DetectorPalworldRCON,
		DetectorProjectZomboidRCON,
		DetectorMinecraftRCON,
		DetectorSourceQuery,
		DetectorAMPPlayers:
		return true

	default:
		return false
	}
}

func isImplementedDetector(
	detector Detector,
) bool {
	return detector == DetectorPalworldRCON ||
		detector == DetectorProjectZomboidRCON ||
		detector == DetectorAMPPlayers
}

func detectorUsesRCON(detector Detector) bool {
	return detector == DetectorPalworldRCON ||
		detector == DetectorProjectZomboidRCON
}

func (c Config) EnabledServers() []Server {
	servers := make(
		[]Server,
		0,
		len(c.Servers),
	)

	for _, server := range c.Servers {
		if server.Enabled {
			servers = append(
				servers,
				server,
			)
		}
	}

	return servers
}

func (c Config) ActiveServers() []Server {
	servers := make(
		[]Server,
		0,
		len(c.Servers),
	)

	for _, server := range c.Servers {
		if server.IsActive() {
			servers = append(
				servers,
				server,
			)
		}
	}

	return servers
}

func (c Config) FindServer(
	instance string,
) (Server, bool) {
	instance = strings.TrimSpace(
		instance,
	)

	for _, server := range c.Servers {
		if strings.EqualFold(
			server.Instance,
			instance,
		) {
			return server, true
		}
	}

	return Server{}, false
}

func (s Server) IsActive() bool {
	return s.Enabled &&
		s.Mode == ServerModeActive
}

func (s Server) RCONPassword() (string, error) {
	credentialName := s.RCONPasswordCredentialName()
	environmentName := strings.TrimSpace(s.RCONPasswordEnv)
	if credentialName == "" && environmentName == "" {
		return "", fmt.Errorf(
			i18n.Choose("a instância %s não possui uma credencial de senha RCON configurada", "instance %s has no RCON password credential configured"),
			s.Instance,
		)
	}
	if credentialName != "" {
		password, err := secret.ReadCredential(credentialName)
		if err == nil {
			return password, nil
		}
		if !os.IsNotExist(err) || environmentName == "" {
			return "", fmt.Errorf("não foi possível carregar a credencial RCON %s: %w", credentialName, err)
		}
	}
	password := os.Getenv(environmentName)
	if password == "" {
		return "", fmt.Errorf(i18n.Choose("a variável %s, usada pela instância %s, não foi configurada", "variable %s, used by instance %s, is not set"), environmentName, s.Instance)
	}
	return password, nil
}

func (s Server) RCONPasswordCredentialName() string {
	return strings.TrimSpace(s.RCONCredential)
}
