package idle

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
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
	Address     string `json:"address"`
	PasswordEnv string `json:"password_env"`
}

func Load(
	path string,
) (Config, error) {
	path = strings.TrimSpace(
		path,
	)

	if path == "" {
		return Config{}, fmt.Errorf(
			"o caminho do arquivo de configuração de Idle não foi informado",
		)
	}

	file, err := os.Open(
		path,
	)
	if err != nil {
		return Config{}, fmt.Errorf(
			"não foi possível abrir a configuração de Idle em %s: %w",
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
			"não foi possível interpretar a configuração de Idle em %s: %w",
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
			"há conteúdo inválido depois da configuração de Idle em %s: %w",
			path,
			err,
		)
	}

	return fmt.Errorf(
		"o arquivo %s contém mais de um objeto JSON",
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
				"servidor de índice %d: %w",
				index,
				err,
			)
		}

		instanceKey := strings.ToLower(
			server.Instance,
		)

		if _, exists := instances[instanceKey]; exists {
			return Config{}, fmt.Errorf(
				"a instância %q aparece mais de uma vez na configuração de Idle",
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
		return Server{}, fmt.Errorf(
			"o nome da instância AMP não foi informado",
		)
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
			"o modo de Idle %q da instância %s não é reconhecido",
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
			"idle_timeout_minutes da instância %s precisa ser maior que zero",
			instance,
		)
	}

	startupGraceMinutes := raw.StartupGraceMinutes

	if startupGraceMinutes == 0 {
		startupGraceMinutes = defaultStartupGraceMinutes
	}

	if startupGraceMinutes <= 0 {
		return Server{}, fmt.Errorf(
			"startup_grace_minutes da instância %s precisa ser maior que zero",
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
				"o detector %q da instância %s não é reconhecido",
				server.Detector,
				instance,
			)
		}

		if server.FallbackDetector != "" &&
			!isKnownDetector(server.FallbackDetector) {
			return Server{}, fmt.Errorf(
				"o detector fallback %q da instância %s não é reconhecido",
				server.FallbackDetector,
				instance,
			)
		}

		return server, nil
	}

	if !isImplementedDetector(server.Detector) {
		if server.Detector == "" {
			return Server{}, fmt.Errorf(
				"a instância %s está habilitada, mas nenhum detector foi informado",
				instance,
			)
		}

		return Server{}, fmt.Errorf(
			"o detector %q da instância %s ainda não foi implementado",
			server.Detector,
			instance,
		)
	}

	if server.FallbackDetector != "" {
		if server.FallbackDetector == server.Detector {
			return Server{}, fmt.Errorf(
				"a instância %s usa o mesmo detector como primário e fallback",
				instance,
			)
		}

		if !isImplementedDetector(server.FallbackDetector) {
			return Server{}, fmt.Errorf(
				"o detector fallback %q da instância %s ainda não foi implementado",
				server.FallbackDetector,
				instance,
			)
		}
	}

	if detectorUsesRCON(server.Detector) ||
		detectorUsesRCON(server.FallbackDetector) {
		if raw.RCON == nil {
			return Server{}, fmt.Errorf(
				"a instância %s usa um detector RCON, mas não possui configuração RCON",
				instance,
			)
		}

		server.RCONAddress = strings.TrimSpace(
			raw.RCON.Address,
		)

		server.RCONPasswordEnv = strings.TrimSpace(
			raw.RCON.PasswordEnv,
		)

		if server.RCONAddress == "" {
			return Server{}, fmt.Errorf(
				"o endereço RCON da instância %s não foi informado",
				instance,
			)
		}

		if server.RCONPasswordEnv == "" {
			return Server{}, fmt.Errorf(
				"a variável de senha RCON da instância %s não foi informada",
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
	if strings.TrimSpace(s.RCONPasswordEnv) == "" {
		return "", fmt.Errorf(
			"a instância %s não possui uma variável de senha RCON configurada",
			s.Instance,
		)
	}

	password := os.Getenv(
		s.RCONPasswordEnv,
	)

	if password == "" {
		return "", fmt.Errorf(
			"a variável %s, usada pela instância %s, não foi configurada",
			s.RCONPasswordEnv,
			s.Instance,
		)
	}

	return password, nil
}
