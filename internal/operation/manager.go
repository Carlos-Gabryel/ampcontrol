package operation

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// ErrShuttingDown é retornado por TryAcquire depois que o encerramento do
// serviço começou.
var ErrShuttingDown = errors.New(
	"o AmpControl está sendo reiniciado; tente novamente em instantes",
)

// Info descreve uma operação que atualmente possui o controle
// exclusivo de uma instância AMP.
type Info struct {
	Instance  string
	Operation string
	StartedAt time.Time
}

// AcquireResult representa o resultado de uma tentativa de adquirir
// o controle exclusivo de uma instância.
//
// Quando Acquired retorna true, Lease contém o bloqueio adquirido.
//
// Quando Acquired retorna false, Active descreve a operação que já
// estava usando a instância.
type AcquireResult struct {
	Lease  *Lease
	Active *Info
}

// Acquired informa se o controle da instância foi adquirido.
func (r AcquireResult) Acquired() bool {
	return r.Lease != nil
}

type activeOperation struct {
	info  Info
	token uint64
}

// Manager coordena operações exclusivas por instância AMP.
//
// Instâncias diferentes podem ser controladas simultaneamente.
// Uma mesma instância aceita somente uma operação de cada vez.
type Manager struct {
	mu sync.Mutex

	active    map[string]activeOperation
	nextToken uint64
	now       func() time.Time

	// closing recusa novas operações; drained é fechado quando a última
	// operação ativa termina durante o encerramento.
	closing bool
	drained chan struct{}
}

// Lease representa o controle exclusivo temporário de uma instância.
//
// Release deve ser chamado quando a operação terminar. O método é
// seguro para chamadas repetidas.
type Lease struct {
	manager *Manager
	key     string
	token   uint64
	once    sync.Once
}

// NewManager cria um gerenciador vazio.
func NewManager() *Manager {
	return newManager(
		time.Now,
	)
}

func newManager(
	now func() time.Time,
) *Manager {
	if now == nil {
		now = time.Now
	}

	return &Manager{
		active: make(
			map[string]activeOperation,
		),
		now: now,
	}
}

// TryAcquire tenta adquirir o controle exclusivo de uma instância.
//
// O método não espera uma operação anterior terminar. Quando a
// instância já está ocupada, ele retorna imediatamente as informações
// da operação ativa.
func (m *Manager) TryAcquire(
	instance string,
	operation string,
) (AcquireResult, error) {
	if m == nil {
		return AcquireResult{}, fmt.Errorf(
			"o gerenciador de operações não foi inicializado",
		)
	}

	instance = strings.TrimSpace(
		instance,
	)

	if instance == "" {
		return AcquireResult{}, fmt.Errorf(
			"o nome da instância não foi informado",
		)
	}

	operation = strings.TrimSpace(
		operation,
	)

	if operation == "" {
		return AcquireResult{}, fmt.Errorf(
			"o nome da operação não foi informado",
		)
	}

	key := normalizeInstance(
		instance,
	)

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.active == nil {
		m.active = make(
			map[string]activeOperation,
		)
	}

	if m.closing {
		return AcquireResult{}, ErrShuttingDown
	}

	if current, exists := m.active[key]; exists {
		info := current.info

		return AcquireResult{
			Active: &info,
		}, nil
	}

	m.nextToken++

	if m.nextToken == 0 {
		m.nextToken++
	}

	now := m.now
	if now == nil {
		now = time.Now
	}

	info := Info{
		Instance:  instance,
		Operation: operation,
		StartedAt: now(),
	}

	m.active[key] = activeOperation{
		info:  info,
		token: m.nextToken,
	}

	return AcquireResult{
		Lease: &Lease{
			manager: m,
			key:     key,
			token:   m.nextToken,
		},
	}, nil
}

// Current retorna a operação que atualmente controla a instância.
func (m *Manager) Current(
	instance string,
) (Info, bool) {
	if m == nil {
		return Info{}, false
	}

	key := normalizeInstance(
		instance,
	)

	if key == "" {
		return Info{}, false
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	current, exists := m.active[key]
	if !exists {
		return Info{}, false
	}

	return current.info, true
}

// Release libera o controle exclusivo associado ao Lease.
//
// Chamadas repetidas não afetam operações adquiridas posteriormente.
func (l *Lease) Release() {
	if l == nil {
		return
	}

	l.once.Do(
		func() {
			if l.manager == nil {
				return
			}

			l.manager.release(
				l.key,
				l.token,
			)
		},
	)
}

func (m *Manager) release(
	key string,
	token uint64,
) {
	if m == nil ||
		key == "" ||
		token == 0 {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	current, exists := m.active[key]
	if !exists {
		return
	}

	if current.token != token {
		return
	}

	delete(
		m.active,
		key,
	)

	if m.closing && len(m.active) == 0 && m.drained != nil {
		close(m.drained)
		m.drained = nil
	}
}

// Shutdown passa a recusar novas operações e espera as que estão em
// andamento terminarem. Se o contexto expirar antes, retorna as operações
// que ainda estavam ativas junto com o erro do contexto.
func (m *Manager) Shutdown(
	ctx context.Context,
) ([]Info, error) {
	if m == nil {
		return nil, nil
	}

	m.mu.Lock()
	m.closing = true
	if len(m.active) == 0 {
		m.mu.Unlock()
		return nil, nil
	}
	if m.drained == nil {
		m.drained = make(chan struct{})
	}
	drained := m.drained
	m.mu.Unlock()

	select {
	case <-drained:
		return nil, nil
	case <-ctx.Done():
		return m.Active(), ctx.Err()
	}
}

// Active retorna as operações em andamento, da mais antiga para a mais nova.
func (m *Manager) Active() []Info {
	if m == nil {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	infos := make([]Info, 0, len(m.active))
	for _, current := range m.active {
		infos = append(infos, current.info)
	}
	sort.Slice(infos, func(i, j int) bool {
		return infos[i].StartedAt.Before(infos[j].StartedAt)
	})
	return infos
}

func normalizeInstance(
	instance string,
) string {
	return strings.ToLower(
		strings.TrimSpace(instance),
	)
}
