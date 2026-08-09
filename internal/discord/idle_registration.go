package discord

import (
	"context"
	"fmt"
	"strings"

	"github.com/alabamaamp/ampcontrol/internal/amp"
)

type IdleServerRegistrar interface {
	RegisterIdleServer(
		ctx context.Context,
		instance amp.ManagedInstance,
	) error
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
	c.idleRegistrationMu.Unlock()
	return nil
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
