package discord

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Carlos-Gabryel/ampcontrol/internal/amp"
)

type IdleServerRegistrar interface {
	RegisterIdleServer(
		ctx context.Context,
		instance amp.ManagedInstance,
	) error
}

type IdleDetectionSettings struct {
	Method           string
	Detector         string
	FallbackDetector string
	RCONConfigured   bool
}

type IdleDetectionManager interface {
	IdleDetectionSettings(instance string) (IdleDetectionSettings, error)
	SetIdleDetectionMethod(ctx context.Context, instance string, method string) (IdleDetectionSettings, error)
	ResetIdleDetectionMethod(ctx context.Context, instance string) (IdleDetectionSettings, error)
}

type IdleDiagnosticsSnapshot struct {
	Running           bool
	StartedAt         time.Time
	LastEventAt       time.Time
	LastErrorAt       time.Time
	LastError         string
	CheckInterval     time.Duration
	RegisteredServers int
	EnabledServers    int
	ActiveServers     int
	ObserveServers    int
	RCONServers       int
	RCONReadyServers  int
}

type IdleDiagnosticsProvider interface {
	IdleDiagnostics() IdleDiagnosticsSnapshot
}

func (c *Client) SetIdleServerRegistrar(
	registrar IdleServerRegistrar,
) error {
	if registrar == nil {
		return fmt.Errorf(
			"o gerenciador de cadastros do Idle não foi informado",
		)
	}

	c.idleRegistrationMu.Lock()
	c.idleServerRegistrar = registrar
	if manager, ok := registrar.(IdleDetectionManager); ok {
		c.idleDetectionManager = manager
	}
	if diagnostics, ok := registrar.(IdleDiagnosticsProvider); ok {
		c.idleDiagnosticsProvider = diagnostics
	}
	c.idleRegistrationMu.Unlock()
	return nil
}

func (c *Client) idleDetectionConfigurator() IdleDetectionManager {
	c.idleRegistrationMu.RLock()
	defer c.idleRegistrationMu.RUnlock()
	return c.idleDetectionManager
}

func instanceNameMap(instances []string) map[string]string {
	result := make(map[string]string, len(instances))
	for _, instance := range instances {
		name := strings.TrimSpace(instance)
		if name == "" {
			continue
		}
		result[normalizeInstanceVisibilityKey(name)] = name
	}
	return result
}

func filterUnregisteredAMPInstances(
	instances []amp.ManagedInstance,
	registeredNames []string,
) []amp.ManagedInstance {
	registered := instanceNameMap(registeredNames)
	result := make([]amp.ManagedInstance, 0, len(instances))
	for _, instance := range instances {
		if _, exists := registered[normalizeInstanceVisibilityKey(instance.Name)]; !exists {
			result = append(result, instance)
		}
	}
	return result
}

func (c *Client) idleRegisteredInstanceNames() []string {
	c.idleRegistrationMu.RLock()
	defer c.idleRegistrationMu.RUnlock()

	result := make([]string, 0, len(c.idleRegistered))
	for _, name := range c.idleRegistered {
		result = append(result, name)
	}
	return normalizeHiddenInstances(result)
}

func (c *Client) idleInstanceRegistered(instance string) bool {
	c.idleRegistrationMu.RLock()
	defer c.idleRegistrationMu.RUnlock()

	_, exists := c.idleRegistered[normalizeInstanceVisibilityKey(instance)]
	return exists
}

func (c *Client) idleRegistrar() IdleServerRegistrar {
	c.idleRegistrationMu.RLock()
	defer c.idleRegistrationMu.RUnlock()
	return c.idleServerRegistrar
}

func (c *Client) markIdleInstanceRegistered(instance string) {
	instance = strings.TrimSpace(instance)
	if instance == "" {
		return
	}

	c.idleRegistrationMu.Lock()
	c.idleRegistered[normalizeInstanceVisibilityKey(instance)] = instance
	c.idleRegistrationMu.Unlock()
}
