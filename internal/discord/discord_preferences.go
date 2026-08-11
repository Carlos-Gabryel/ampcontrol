package discord

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alabamaamp/ampcontrol/internal/amp"
)

type discordPreferences struct {
	HiddenInstances  []string                       `json:"hidden_instances"`
	InstanceSettings []instancePresentationOverride `json:"instance_settings,omitempty"`
}

type instancePresentationOverride struct {
	Instance       string `json:"instance"`
	DisplayName    string `json:"display_name,omitempty"`
	Game           string `json:"game,omitempty"`
	MaximumPlayers int    `json:"maximum_players,omitempty"`
	Address        string `json:"address,omitempty"`
}

func loadDiscordPreferences(path string) (discordPreferences, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return discordPreferences{}, fmt.Errorf(
			"o caminho das preferências do Discord não foi configurado",
		)
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return discordPreferences{HiddenInstances: []string{}, InstanceSettings: []instancePresentationOverride{}}, nil
	}
	if err != nil {
		return discordPreferences{}, fmt.Errorf(
			"não foi possível ler as preferências do Discord em %s: %w",
			path,
			err,
		)
	}

	var preferences discordPreferences
	if err := json.Unmarshal(data, &preferences); err != nil {
		return discordPreferences{}, fmt.Errorf(
			"as preferências do Discord em %s são inválidas: %w",
			path,
			err,
		)
	}

	preferences.HiddenInstances = normalizeHiddenInstances(
		preferences.HiddenInstances,
	)
	preferences.InstanceSettings, err = normalizeInstancePresentationOverrides(preferences.InstanceSettings)
	if err != nil {
		return discordPreferences{}, fmt.Errorf("as preferências de apresentação são inválidas: %w", err)
	}

	return preferences, nil
}

func saveDiscordPreferences(
	path string,
	preferences discordPreferences,
) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf(
			"o caminho das preferências do Discord não foi configurado",
		)
	}

	preferences.HiddenInstances = normalizeHiddenInstances(
		preferences.HiddenInstances,
	)
	normalizedSettings, settingsErr := normalizeInstancePresentationOverrides(preferences.InstanceSettings)
	if settingsErr != nil {
		return settingsErr
	}
	preferences.InstanceSettings = normalizedSettings

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf(
			"não foi possível criar a pasta das preferências do Discord: %w",
			err,
		)
	}

	data, err := json.MarshalIndent(preferences, "", "  ")
	if err != nil {
		return fmt.Errorf(
			"não foi possível serializar as preferências do Discord: %w",
			err,
		)
	}
	data = append(data, '\n')

	temporaryPath := path + ".tmp"
	if err := os.WriteFile(temporaryPath, data, 0o600); err != nil {
		return fmt.Errorf(
			"não foi possível gravar as preferências temporárias do Discord: %w",
			err,
		)
	}

	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf(
			"não foi possível publicar as preferências do Discord: %w",
			err,
		)
	}

	return nil
}

func normalizeHiddenInstances(instances []string) []string {
	byKey := make(map[string]string, len(instances))
	for _, instance := range instances {
		name := strings.TrimSpace(instance)
		if name == "" {
			continue
		}

		key := normalizeInstanceVisibilityKey(name)
		if _, exists := byKey[key]; !exists {
			byKey[key] = name
		}
	}

	result := make([]string, 0, len(byKey))
	for _, name := range byKey {
		result = append(result, name)
	}

	sort.Slice(result, func(left int, right int) bool {
		return strings.ToLower(result[left]) < strings.ToLower(result[right])
	})

	return result
}

func normalizeInstanceVisibilityKey(instance string) string {
	return strings.ToLower(strings.TrimSpace(instance))
}

func hiddenInstanceMap(instances []string) map[string]string {
	result := make(map[string]string, len(instances))
	for _, name := range normalizeHiddenInstances(instances) {
		result[normalizeInstanceVisibilityKey(name)] = name
	}
	return result
}

func splitAMPInstancesByVisibility(
	instances []amp.ManagedInstance,
	hiddenNames []string,
) (visible []amp.ManagedInstance, hidden []amp.ManagedInstance) {
	pendingHidden := hiddenInstanceMap(hiddenNames)
	visible = make([]amp.ManagedInstance, 0, len(instances))
	hidden = make([]amp.ManagedInstance, 0, len(pendingHidden))

	for _, instance := range instances {
		key := normalizeInstanceVisibilityKey(instance.Name)
		if _, isHidden := pendingHidden[key]; isHidden {
			hidden = append(hidden, instance)
			delete(pendingHidden, key)
			continue
		}

		visible = append(visible, instance)
	}

	for _, name := range pendingHidden {
		hidden = append(hidden, amp.ManagedInstance{
			Name:         name,
			FriendlyName: name,
			Game:         "Instância não encontrada",
		})
	}

	sort.Slice(hidden, func(left int, right int) bool {
		return strings.ToLower(hidden[left].FriendlyName) <
			strings.ToLower(hidden[right].FriendlyName)
	})

	return visible, hidden
}

func (c *Client) hiddenInstanceNames() []string {
	c.preferencesMu.RLock()
	defer c.preferencesMu.RUnlock()

	return append([]string(nil), c.preferences.HiddenInstances...)
}

func (c *Client) instanceHidden(instance string) bool {
	key := normalizeInstanceVisibilityKey(instance)
	if key == "" {
		return false
	}

	_, exists := hiddenInstanceMap(c.hiddenInstanceNames())[key]
	return exists
}

func (c *Client) visibleAMPInstances(
	instances []amp.ManagedInstance,
) []amp.ManagedInstance {
	visible, _ := splitAMPInstancesByVisibility(
		instances,
		c.hiddenInstanceNames(),
	)
	return visible
}

func (c *Client) setInstanceHidden(
	instance string,
	hidden bool,
) (bool, error) {
	instance = strings.TrimSpace(instance)
	key := normalizeInstanceVisibilityKey(instance)
	if key == "" {
		return false, fmt.Errorf("a instância não foi informada")
	}

	c.preferencesMu.Lock()
	defer c.preferencesMu.Unlock()

	names := hiddenInstanceMap(c.preferences.HiddenInstances)
	_, exists := names[key]
	if hidden == exists {
		return false, nil
	}

	if hidden {
		names[key] = instance
	} else {
		delete(names, key)
	}

	next := discordPreferences{
		HiddenInstances: make([]string, 0, len(names)),
		InstanceSettings: append(
			[]instancePresentationOverride(nil),
			c.preferences.InstanceSettings...,
		),
	}
	for _, name := range names {
		next.HiddenInstances = append(next.HiddenInstances, name)
	}
	next.HiddenInstances = normalizeHiddenInstances(next.HiddenInstances)

	if err := saveDiscordPreferences(c.preferencesPath, next); err != nil {
		return false, err
	}

	c.preferences = next
	return true, nil
}

func normalizeInstancePresentationOverrides(
	overrides []instancePresentationOverride,
) ([]instancePresentationOverride, error) {
	byKey := make(map[string]instancePresentationOverride, len(overrides))
	for _, override := range overrides {
		override.Instance = strings.TrimSpace(override.Instance)
		override.DisplayName = strings.TrimSpace(override.DisplayName)
		override.Game = strings.TrimSpace(override.Game)
		override.Address = strings.TrimSpace(override.Address)
		if override.Instance == "" {
			return nil, fmt.Errorf("há uma configuração de apresentação sem instância")
		}
		if override.MaximumPlayers < 0 || override.MaximumPlayers > 100000 {
			return nil, fmt.Errorf("o limite de jogadores de %s é inválido", override.Instance)
		}
		key := normalizeInstanceVisibilityKey(override.Instance)
		if _, exists := byKey[key]; exists {
			return nil, fmt.Errorf("a instância %s possui configurações de apresentação duplicadas", override.Instance)
		}
		byKey[key] = override
	}

	result := make([]instancePresentationOverride, 0, len(byKey))
	for _, override := range byKey {
		result = append(result, override)
	}
	sort.Slice(result, func(left, right int) bool {
		return strings.ToLower(result[left].Instance) < strings.ToLower(result[right].Instance)
	})
	return result, nil
}

func (c *Client) instancePresentationSettings(instance string) (instancePresentationOverride, bool) {
	c.preferencesMu.RLock()
	defer c.preferencesMu.RUnlock()
	key := normalizeInstanceVisibilityKey(instance)
	for _, override := range c.preferences.InstanceSettings {
		if normalizeInstanceVisibilityKey(override.Instance) == key {
			return override, true
		}
	}
	return instancePresentationOverride{}, false
}

func (c *Client) instancePresentationSettingsSnapshot() []instancePresentationOverride {
	c.preferencesMu.RLock()
	defer c.preferencesMu.RUnlock()
	return append([]instancePresentationOverride(nil), c.preferences.InstanceSettings...)
}

func (c *Client) setInstancePresentationSettings(
	instance string,
	displayName *string,
	game *string,
	maximumPlayers *int,
	address *string,
) (instancePresentationOverride, error) {
	instance = strings.TrimSpace(instance)
	if instance == "" {
		return instancePresentationOverride{}, fmt.Errorf("a instância não foi informada")
	}

	c.preferencesMu.Lock()
	defer c.preferencesMu.Unlock()

	next := discordPreferences{
		HiddenInstances:  append([]string(nil), c.preferences.HiddenInstances...),
		InstanceSettings: append([]instancePresentationOverride(nil), c.preferences.InstanceSettings...),
	}
	key := normalizeInstanceVisibilityKey(instance)
	index := -1
	for candidate := range next.InstanceSettings {
		if normalizeInstanceVisibilityKey(next.InstanceSettings[candidate].Instance) == key {
			index = candidate
			break
		}
	}
	if index < 0 {
		next.InstanceSettings = append(next.InstanceSettings, instancePresentationOverride{Instance: instance})
		index = len(next.InstanceSettings) - 1
	}
	setting := next.InstanceSettings[index]
	if displayName != nil {
		setting.DisplayName = strings.TrimSpace(*displayName)
	}
	if game != nil {
		setting.Game = strings.TrimSpace(*game)
	}
	if maximumPlayers != nil {
		setting.MaximumPlayers = *maximumPlayers
	}
	if address != nil {
		setting.Address = strings.TrimSpace(*address)
	}
	if setting.MaximumPlayers < 0 || setting.MaximumPlayers > 100000 {
		return instancePresentationOverride{}, fmt.Errorf("o limite de jogadores precisa estar entre 0 e 100000")
	}
	next.InstanceSettings[index] = setting

	if err := saveDiscordPreferences(c.preferencesPath, next); err != nil {
		return instancePresentationOverride{}, err
	}
	c.preferences = next
	return setting, nil
}

func (c *Client) resetInstancePresentationSettings(instance string) (bool, error) {
	instance = strings.TrimSpace(instance)
	key := normalizeInstanceVisibilityKey(instance)
	if key == "" {
		return false, fmt.Errorf("a instância não foi informada")
	}

	c.preferencesMu.Lock()
	defer c.preferencesMu.Unlock()
	next := discordPreferences{
		HiddenInstances:  append([]string(nil), c.preferences.HiddenInstances...),
		InstanceSettings: make([]instancePresentationOverride, 0, len(c.preferences.InstanceSettings)),
	}
	removed := false
	for _, setting := range c.preferences.InstanceSettings {
		if normalizeInstanceVisibilityKey(setting.Instance) == key {
			removed = true
			continue
		}
		next.InstanceSettings = append(next.InstanceSettings, setting)
	}
	if !removed {
		return false, nil
	}
	if err := saveDiscordPreferences(c.preferencesPath, next); err != nil {
		return false, err
	}
	c.preferences = next
	return true, nil
}

func applyInstancePresentationOverrides(
	instances []amp.ManagedInstance,
	overrides []instancePresentationOverride,
) {
	byKey := make(map[string]instancePresentationOverride, len(overrides))
	for _, override := range overrides {
		byKey[normalizeInstanceVisibilityKey(override.Instance)] = override
	}
	for index := range instances {
		override, exists := byKey[normalizeInstanceVisibilityKey(instances[index].Name)]
		if !exists {
			continue
		}
		if override.DisplayName != "" {
			instances[index].FriendlyName = override.DisplayName
		}
		if override.Game != "" {
			instances[index].Game = override.Game
		}
	}
}
