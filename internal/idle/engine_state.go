package idle

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const engineStateVersion = 1

type engineState struct {
	Version   int                             `json:"version"`
	UpdatedAt time.Time                       `json:"updated_at"`
	Servers   map[string]persistedServerState `json:"servers"`
}

type persistedServerState struct {
	LastRuntime          RuntimeState `json:"last_runtime"`
	OnlineSince          time.Time    `json:"online_since,omitempty"`
	EmptySince           time.Time    `json:"empty_since,omitempty"`
	ApplicationStartedAt time.Time    `json:"application_started_at,omitempty"`
}

// WithStatePath persiste os contadores individuais do motor entre reinicios
// do AmpControl. O arquivo e publicado por rename atomico.
func WithStatePath(path string) EngineOption {
	return func(engine *Engine) error {
		path = strings.TrimSpace(path)
		if path == "" {
			return fmt.Errorf(
				"o caminho do estado persistente do motor de Idle esta vazio",
			)
		}

		state, err := loadEngineState(path)
		if err != nil {
			return err
		}

		engine.statePath = path
		engine.restoreState(state)

		return nil
	}
}

func loadEngineState(path string) (engineState, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return engineState{}, nil
	}
	if err != nil {
		return engineState{}, fmt.Errorf(
			"nao foi possivel abrir o estado do motor de Idle em %s: %w",
			path,
			err,
		)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()

	var state engineState
	if err := decoder.Decode(&state); err != nil {
		return engineState{}, fmt.Errorf(
			"o estado do motor de Idle em %s e invalido: %w",
			path,
			err,
		)
	}

	if state.Version != engineStateVersion {
		return engineState{}, fmt.Errorf(
			"a versao %d do estado do motor de Idle nao e suportada",
			state.Version,
		)
	}

	return state, nil
}

func (e *Engine) restoreState(state engineState) {
	if e == nil || state.Version == 0 {
		return
	}

	for _, server := range e.config.EnabledServers() {
		key := normalizeTrackerKey(server.Instance)
		persisted, exists := state.Servers[key]
		if !exists || !validPersistedServerState(persisted) {
			continue
		}

		e.trackers[key] = &serverTracker{
			LastRuntime:          persisted.LastRuntime,
			OnlineSince:          persisted.OnlineSince,
			EmptySince:           persisted.EmptySince,
			ApplicationStartedAt: persisted.ApplicationStartedAt,
			Restored:             true,
		}
	}
}

func validPersistedServerState(state persistedServerState) bool {
	if state.LastRuntime != RuntimeStateOnline {
		return false
	}
	if state.OnlineSince.IsZero() || state.ApplicationStartedAt.IsZero() {
		return false
	}
	if !state.EmptySince.IsZero() && state.EmptySince.Before(state.OnlineSince) {
		return false
	}

	return true
}

func (e *Engine) persistState(now time.Time) error {
	if e == nil || strings.TrimSpace(e.statePath) == "" {
		return nil
	}

	state := engineState{
		Version:   engineStateVersion,
		UpdatedAt: now,
		Servers: make(
			map[string]persistedServerState,
			len(e.trackers),
		),
	}

	for key, tracker := range e.trackers {
		if tracker == nil || tracker.LastRuntime != RuntimeStateOnline {
			continue
		}

		state.Servers[key] = persistedServerState{
			LastRuntime:          tracker.LastRuntime,
			OnlineSince:          tracker.OnlineSince,
			EmptySince:           tracker.EmptySince,
			ApplicationStartedAt: tracker.ApplicationStartedAt,
		}
	}

	if err := os.MkdirAll(filepath.Dir(e.statePath), 0o750); err != nil {
		return fmt.Errorf(
			"nao foi possivel criar a pasta do estado do motor de Idle: %w",
			err,
		)
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf(
			"nao foi possivel serializar o estado do motor de Idle: %w",
			err,
		)
	}
	data = append(data, '\n')

	temporaryPath := e.statePath + ".tmp"
	if err := os.WriteFile(temporaryPath, data, 0o600); err != nil {
		return fmt.Errorf(
			"nao foi possivel gravar o estado temporario do motor de Idle: %w",
			err,
		)
	}

	if err := os.Rename(temporaryPath, e.statePath); err != nil {
		_ = os.Remove(temporaryPath)

		return fmt.Errorf(
			"nao foi possivel publicar o estado do motor de Idle: %w",
			err,
		)
	}

	return nil
}

func normalizeTrackerKey(instance string) string {
	return strings.ToLower(strings.TrimSpace(instance))
}
