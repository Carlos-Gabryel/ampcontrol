package amp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Carlos-Gabryel/ampcontrol/internal/i18n"
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
		return nil, errors.New(i18n.Choose("o cliente ADS do inventário não foi informado", "the inventory ADS client was not provided"))
	}
	adsURL = strings.TrimRight(strings.TrimSpace(adsURL), "/")
	if adsURL == "" {
		return nil, errors.New(i18n.Choose("a URL ADS do inventário não foi informada", "the inventory ADS URL was not provided"))
	}
	return &Inventory{ads: client, adsURL: adsURL, fallback: DiscoverInstances}, nil
}

func (i *Inventory) DiscoverInstances(ctx context.Context) ([]ManagedInstance, error) {
	if i == nil || i.ads == nil {
		return nil, errors.New(i18n.Choose("o inventário AMP não foi inicializado", "the AMP inventory was not initialized"))
	}
	instances, adsErr := i.ads.DiscoverManagedInstances(ctx, i.adsURL)
	if adsErr == nil {
		return instances, nil
	}

	fallback, fallbackErr := i.fallback(ctx)
	if fallbackErr != nil {
		return nil, fmt.Errorf(
			i18n.Choose("inventário ADS falhou (%v) e o fallback ampinstmgr também falhou: %w", "the ADS inventory failed (%v) and the ampinstmgr fallback also failed: %w"),
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
