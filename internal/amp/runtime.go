package amp

import (
	"fmt"
	"path"
	"regexp"
	"strings"
	"sync"

	"github.com/Carlos-Gabryel/ampcontrol/internal/i18n"
)

type RuntimeConfig struct {
	SystemUser  string
	ManagerPath string
	WrapperPath string
	SudoPath    string
}

var systemUserPattern = regexp.MustCompile(`^[a-z_][a-z0-9_-]*[$]?$`)

var runtimeSettings = struct {
	sync.RWMutex
	value RuntimeConfig
}{
	value: RuntimeConfig{
		SystemUser:  "amp",
		ManagerPath: "/usr/bin/ampinstmgr",
		WrapperPath: "/usr/local/bin/ampcontrol-amp",
		SudoPath:    "/usr/bin/sudo",
	},
}

func ConfigureRuntime(config RuntimeConfig) error {
	normalized, err := normalizeRuntimeConfig(config)
	if err != nil {
		return err
	}
	runtimeSettings.Lock()
	runtimeSettings.value = normalized
	runtimeSettings.Unlock()
	return nil
}

func currentRuntimeConfig() RuntimeConfig {
	runtimeSettings.RLock()
	defer runtimeSettings.RUnlock()
	return runtimeSettings.value
}

func normalizeRuntimeConfig(config RuntimeConfig) (RuntimeConfig, error) {
	config.SystemUser = strings.TrimSpace(config.SystemUser)
	config.ManagerPath = strings.TrimSpace(config.ManagerPath)
	config.WrapperPath = strings.TrimSpace(config.WrapperPath)
	config.SudoPath = strings.TrimSpace(config.SudoPath)

	if !systemUserPattern.MatchString(config.SystemUser) {
		return RuntimeConfig{}, fmt.Errorf(i18n.Choose("usuário de sistema do AMP inválido: %q", "invalid AMP system user: %q"), config.SystemUser)
	}
	for name, value := range map[string]string{
		"manager_path": config.ManagerPath,
		"wrapper_path": config.WrapperPath,
		"sudo_path":    config.SudoPath,
	} {
		if !path.IsAbs(value) {
			return RuntimeConfig{}, fmt.Errorf(i18n.Choose("%s precisa ser um caminho Linux absoluto", "%s must be an absolute Linux path"), name)
		}
	}
	return config, nil
}
