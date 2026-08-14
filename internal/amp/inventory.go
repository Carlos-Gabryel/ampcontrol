package amp

import (
	"context"
	"fmt"
	"strings"
)

type InstanceDiscoverer interface {
	DiscoverInstances(ctx context.Context) ([]ManagedInstance, error)
}

type adsInstanceDiscoverer interface {
	DiscoverManagedInstances(ctx context.Context, adsURL string) ([]ManagedInstance, error)
}

type Inventory struct {
	ads      adsInstanceDiscoverer
	adsURL   string
	fallback func(context.Context) ([]ManagedInstance, error)
}

func NewInventory(client adsInstanceDiscoverer, adsURL string) (*Inventory, error) {
	if client == nil {
		return nil, fmt.Errorf("o cliente ADS do inventário não foi informado")
	}
	adsURL = strings.TrimRight(strings.TrimSpace(adsURL), "/")
	if adsURL == "" {
		return nil, fmt.Errorf("a URL ADS do inventário não foi informada")
	}
	return &Inventory{ads: client, adsURL: adsURL, fallback: DiscoverInstances}, nil
}

func (i *Inventory) DiscoverInstances(ctx context.Context) ([]ManagedInstance, error) {
	if i == nil || i.ads == nil {
		return nil, fmt.Errorf("o inventário AMP não foi inicializado")
	}
	instances, adsErr := i.ads.DiscoverManagedInstances(ctx, i.adsURL)
	if adsErr == nil {
		return instances, nil
	}

	fallback, fallbackErr := i.fallback(ctx)
	if fallbackErr != nil {
		return nil, fmt.Errorf(
			"inventário ADS falhou (%v) e o fallback ampinstmgr também falhou: %w",
			adsErr,
			fallbackErr,
		)
	}
	for index := range fallback {
		if strings.EqualFold(fallback[index].Module, "Minecraft") {
			fallback[index].Game = "Minecraft"
		} else if strings.TrimSpace(fallback[index].Game) == "" {
			fallback[index].Game = fallback[index].Module
		}
	}
	return fallback, nil
}
