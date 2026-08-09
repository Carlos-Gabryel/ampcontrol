package amp

import (
	"context"
	"fmt"
	"math"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

const activeUsersMetricName = "Active Users"

type StatusMetric struct {
	RawValue float64 `json:"RawValue"`
	MaxValue float64 `json:"MaxValue"`
	Units    string  `json:"Units"`
}

type PlayerCounts struct {
	Current int
	Maximum int
}

type ModuleInfo struct {
	Name          string `json:"Name"`
	Application   string `json:"AppName"`
	SupportsSleep bool   `json:"SupportsSleep"`
}

type adsInstanceGroup struct {
	AvailableInstances []adsManagedInstance `json:"AvailableInstances"`
}

type adsManagedInstance struct {
	ID                string `json:"InstanceID"`
	Name              string `json:"InstanceName"`
	FriendlyName      string `json:"FriendlyName"`
	Module            string `json:"Module"`
	ModuleDisplayName string `json:"ModuleDisplayName"`
	IP                string `json:"IP"`
	Port              int    `json:"Port"`
	HTTPS             bool   `json:"IsHTTPS"`
	Running           bool   `json:"Running"`
}

func (s ApplicationStatus) PlayerCounts() (PlayerCounts, error) {
	var metric StatusMetric
	var found bool

	for name, candidate := range s.Metrics {
		if strings.EqualFold(strings.TrimSpace(name), activeUsersMetricName) {
			metric = candidate
			found = true
			break
		}
	}

	if !found {
		return PlayerCounts{}, fmt.Errorf(
			"a métrica %q não foi retornada pelo AMP",
			activeUsersMetricName,
		)
	}

	if !validWholeNumber(metric.RawValue) ||
		!validWholeNumber(metric.MaxValue) {
		return PlayerCounts{}, fmt.Errorf(
			"a métrica %q contém valores não inteiros ou não finitos: atual=%v máximo=%v",
			activeUsersMetricName,
			metric.RawValue,
			metric.MaxValue,
		)
	}

	current := int(metric.RawValue)
	maximum := int(metric.MaxValue)

	if current < 0 || maximum <= 0 || current > maximum {
		return PlayerCounts{}, fmt.Errorf(
			"a métrica %q contém valores inválidos: atual=%d máximo=%d",
			activeUsersMetricName,
			current,
			maximum,
		)
	}

	return PlayerCounts{
		Current: current,
		Maximum: maximum,
	}, nil
}

func validWholeNumber(value float64) bool {
	return !math.IsNaN(value) &&
		!math.IsInf(value, 0) &&
		math.Trunc(value) == value
}

// GetModuleInfo consulta metadados públicos da instância. O AMP expõe
// este método sem autenticação e, no Generic Module, AppName contém o
// nome real do jogo.
func (c *APIClient) GetModuleInfo(
	ctx context.Context,
	baseURL string,
) (ModuleInfo, error) {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return ModuleInfo{}, fmt.Errorf(
			"a URL da instância AMP não foi informada",
		)
	}

	var response ModuleInfo
	if err := c.postJSON(
		ctx,
		buildAPIURL(baseURL, "Core", "GetModuleInfo"),
		"",
		struct{}{},
		&response,
	); err != nil {
		return ModuleInfo{}, fmt.Errorf(
			"falha chamando Core.GetModuleInfo: %w",
			err,
		)
	}

	return response, nil
}

// DiscoverManagedInstances usa o ADS como fonte principal do inventário.
// Além do estado e URL, ADSModule.GetInstances informa ModuleDisplayName,
// que identifica o jogo mesmo quando a instância está desligada.
func (c *APIClient) DiscoverManagedInstances(
	ctx context.Context,
	adsURL string,
) ([]ManagedInstance, error) {
	adsURL = strings.TrimSpace(adsURL)
	if adsURL == "" {
		return nil, fmt.Errorf("a URL do ADS não foi informada")
	}

	sessionID, err := c.login(ctx, adsURL)
	if err != nil {
		return nil, err
	}

	requestBody := struct {
		SessionID        string `json:"SESSIONID"`
		ForceIncludeSelf bool   `json:"ForceIncludeSelf"`
	}{
		SessionID: sessionID,
	}

	var groups []adsInstanceGroup
	if err := c.postJSON(
		ctx,
		buildAPIURL(adsURL, "ADSModule", "GetInstances"),
		sessionID,
		requestBody,
		&groups,
	); err != nil {
		return nil, fmt.Errorf(
			"falha chamando ADSModule.GetInstances: %w",
			err,
		)
	}

	instances := make([]ManagedInstance, 0)
	seen := make(map[string]struct{})

	for _, group := range groups {
		for _, item := range group.AvailableInstances {
			if item.Name == "ADS01" || strings.EqualFold(item.Module, "ADS") {
				continue
			}

			if _, exists := seen[item.Name]; exists {
				continue
			}
			seen[item.Name] = struct{}{}

			instances = append(instances, ManagedInstance{
				ID:           item.ID,
				Name:         item.Name,
				FriendlyName: item.FriendlyName,
				Module:       item.Module,
				Game:         managedInstanceGame(item),
				APIURL:       managedInstanceAPIURL(item),
				Running:      item.Running,
			})
		}
	}

	if len(instances) == 0 {
		return nil, fmt.Errorf("o ADS não retornou instâncias controláveis")
	}

	sort.Slice(instances, func(left, right int) bool {
		leftName := strings.ToLower(instances[left].FriendlyName)
		rightName := strings.ToLower(instances[right].FriendlyName)
		if leftName == rightName {
			return instances[left].Name < instances[right].Name
		}
		return leftName < rightName
	})

	return instances, nil
}

func managedInstanceGame(instance adsManagedInstance) string {
	if game := strings.TrimSpace(instance.ModuleDisplayName); game != "" {
		return game
	}

	module := strings.TrimSpace(instance.Module)
	if strings.EqualFold(module, "Minecraft") {
		return "Minecraft"
	}

	if module == "" {
		return "Desconhecido"
	}

	return module
}

func managedInstanceAPIURL(instance adsManagedInstance) string {
	scheme := "http"
	if instance.HTTPS {
		scheme = "https"
	}

	host := strings.TrimSpace(instance.IP)
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}

	return (&url.URL{
		Scheme: scheme,
		Host: net.JoinHostPort(
			host,
			strconv.Itoa(instance.Port),
		),
		Path: "/",
	}).String()
}
