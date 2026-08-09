package idle

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/alabamaamp/ampcontrol/internal/amp"
)

const ampDiscoveryCacheTTL = 5 * time.Second

// AMPApplicationClient representa somente as operações da API AMP
// necessárias para o motor genérico de Idle.
//
// A interface permite testar o adaptador sem controlar servidores reais.
type AMPApplicationClient interface {
	GetApplicationStatus(
		ctx context.Context,
		baseURL string,
	) (amp.ApplicationStatus, error)

	StopApplication(
		ctx context.Context,
		baseURL string,
	) error
}

type ampDiscoverInstancesFunc func(
	ctx context.Context,
) ([]amp.ManagedInstance, error)

type ampInstanceCache struct {
	fetchedAt time.Time
	instances []amp.ManagedInstance
}

// AMPAdapter conecta o motor genérico de Idle às instâncias
// e à API do AMP.
type AMPAdapter struct {
	client   AMPApplicationClient
	discover ampDiscoverInstancesFunc

	cacheTTL time.Duration
	now      func() time.Time

	cacheMu sync.Mutex
	cache   ampInstanceCache
}

// NewAMPAdapter cria o adaptador usado em produção.
func NewAMPAdapter(
	client AMPApplicationClient,
) (*AMPAdapter, error) {
	return newAMPAdapter(
		client,
		amp.DiscoverInstances,
	)
}

func newAMPAdapter(
	client AMPApplicationClient,
	discover ampDiscoverInstancesFunc,
) (*AMPAdapter, error) {
	if client == nil {
		return nil, fmt.Errorf(
			"o cliente da API AMP não foi informado",
		)
	}

	if discover == nil {
		return nil, fmt.Errorf(
			"a função de descoberta das instâncias AMP não foi informada",
		)
	}

	return &AMPAdapter{
		client:   client,
		discover: discover,
		cacheTTL: ampDiscoveryCacheTTL,
		now:      time.Now,
	}, nil
}

// RuntimeState implementa RuntimeStatusProvider.
//
// A instância AMP desligada é classificada como Offline sem tentar
// acessar sua API, pois a porta da API também estará indisponível.
//
// As descobertas de instâncias usadas somente para leitura podem ser
// reutilizadas por alguns segundos. Isso evita executar o comando de
// descoberta completo uma vez para cada servidor dentro do mesmo ciclo
// do motor de Idle.
func (a *AMPAdapter) RuntimeState(
	ctx context.Context,
	server Server,
) (RuntimeState, error) {
	instance, err := a.resolveInstance(
		ctx,
		server,
		true,
	)
	if err != nil {
		return RuntimeStateUnknown, err
	}

	if !instance.Running {
		return RuntimeStateOffline, nil
	}

	apiURL := strings.TrimSpace(
		instance.APIURL,
	)

	if apiURL == "" {
		return RuntimeStateUnknown, fmt.Errorf(
			"a instância AMP %s está ligada, mas não possui URL de API",
			instance.Name,
		)
	}

	status, err := a.client.GetApplicationStatus(
		ctx,
		apiURL,
	)
	if err != nil {
		return RuntimeStateUnknown, fmt.Errorf(
			"não foi possível consultar o estado da aplicação da instância %s: %w",
			instance.Name,
			err,
		)
	}

	return mapAMPApplicationStatus(
		status,
	), nil
}

// StopApplication implementa ApplicationStopper.
//
// Antes de executar Core.Stop, o adaptador ignora o cache, descobre
// novamente a instância e consulta novamente Core.GetStatus. Essa
// verificação protege contra mudanças de estado ocorridas entre o último
// ciclo do motor e a solicitação de parada.
func (a *AMPAdapter) StopApplication(
	ctx context.Context,
	server Server,
) error {
	instance, err := a.resolveInstance(
		ctx,
		server,
		false,
	)
	if err != nil {
		return err
	}

	if !instance.Running {
		return fmt.Errorf(
			"a instância AMP %s ficou Offline antes da parada automática",
			instance.Name,
		)
	}

	apiURL := strings.TrimSpace(
		instance.APIURL,
	)

	if apiURL == "" {
		return fmt.Errorf(
			"a instância AMP %s está ligada, mas não possui URL de API",
			instance.Name,
		)
	}

	status, err := a.client.GetApplicationStatus(
		ctx,
		apiURL,
	)
	if err != nil {
		return fmt.Errorf(
			"não foi possível confirmar o estado da aplicação da instância %s antes da parada: %w",
			instance.Name,
			err,
		)
	}

	runtimeState := mapAMPApplicationStatus(
		status,
	)

	switch runtimeState {
	case RuntimeStateIdle:
		// O resultado desejado já foi alcançado por outra operação.
		// A parada é considerada concluída sem repetir Core.Stop.
		return nil

	case RuntimeStateOnline:
		// Estado válido para Core.Stop.

	case RuntimeStateBusy:
		return fmt.Errorf(
			"a aplicação da instância %s entrou em transição antes da parada automática; estado AMP: %s",
			instance.Name,
			status.State.String(),
		)

	case RuntimeStateFailed:
		return fmt.Errorf(
			"a aplicação da instância %s está em estado de falha ou suspensão; estado AMP: %s",
			instance.Name,
			status.State.String(),
		)

	case RuntimeStateOffline:
		return fmt.Errorf(
			"a aplicação da instância %s ficou Offline antes da parada automática",
			instance.Name,
		)

	default:
		return fmt.Errorf(
			"o estado da aplicação da instância %s não é reconhecido; estado AMP: %s",
			instance.Name,
			status.State.String(),
		)
	}

	if err := a.client.StopApplication(
		ctx,
		apiURL,
	); err != nil {
		return fmt.Errorf(
			"Core.Stop falhou na instância %s: %w",
			instance.Name,
			err,
		)
	}

	return nil
}

func (a *AMPAdapter) resolveInstance(
	ctx context.Context,
	server Server,
	allowCache bool,
) (amp.ManagedInstance, error) {
	if a == nil {
		return amp.ManagedInstance{}, fmt.Errorf(
			"o adaptador AMP não foi inicializado",
		)
	}

	if ctx == nil {
		return amp.ManagedInstance{}, fmt.Errorf(
			"o contexto da operação AMP é nulo",
		)
	}

	if err := ctx.Err(); err != nil {
		return amp.ManagedInstance{}, fmt.Errorf(
			"o contexto da operação AMP foi encerrado: %w",
			err,
		)
	}

	if a.client == nil {
		return amp.ManagedInstance{}, fmt.Errorf(
			"o cliente da API AMP não foi inicializado",
		)
	}

	if a.discover == nil {
		return amp.ManagedInstance{}, fmt.Errorf(
			"a descoberta das instâncias AMP não foi inicializada",
		)
	}

	if a.now == nil {
		return amp.ManagedInstance{}, fmt.Errorf(
			"o relógio interno do adaptador AMP não foi inicializado",
		)
	}

	instanceName := strings.TrimSpace(
		server.Instance,
	)

	if instanceName == "" {
		return amp.ManagedInstance{}, fmt.Errorf(
			"o servidor de Idle não informou o nome da instância AMP",
		)
	}

	instances, err := a.discoverInstances(
		ctx,
		allowCache,
	)
	if err != nil {
		return amp.ManagedInstance{}, fmt.Errorf(
			"não foi possível descobrir as instâncias AMP: %w",
			err,
		)
	}

	for _, instance := range instances {
		if strings.EqualFold(
			strings.TrimSpace(instance.Name),
			instanceName,
		) {
			return instance, nil
		}
	}

	return amp.ManagedInstance{}, fmt.Errorf(
		"a instância AMP %q configurada no monitor de Idle não foi encontrada",
		instanceName,
	)
}

func (a *AMPAdapter) discoverInstances(
	ctx context.Context,
	allowCache bool,
) ([]amp.ManagedInstance, error) {
	a.cacheMu.Lock()
	defer a.cacheMu.Unlock()

	if allowCache &&
		a.cacheTTL > 0 &&
		!a.cache.fetchedAt.IsZero() {
		age := a.now().Sub(
			a.cache.fetchedAt,
		)

		if age >= 0 && age < a.cacheTTL {
			return cloneManagedInstances(
				a.cache.instances,
			), nil
		}
	}

	instances, err := a.discover(
		ctx,
	)
	if err != nil {
		return nil, err
	}

	snapshot := cloneManagedInstances(
		instances,
	)

	a.cache = ampInstanceCache{
		fetchedAt: a.now(),
		instances: snapshot,
	}

	return cloneManagedInstances(
		snapshot,
	), nil
}

func cloneManagedInstances(
	instances []amp.ManagedInstance,
) []amp.ManagedInstance {
	if len(instances) == 0 {
		return nil
	}

	result := make(
		[]amp.ManagedInstance,
		len(instances),
	)

	copy(
		result,
		instances,
	)

	return result
}

func mapAMPApplicationStatus(
	status amp.ApplicationStatus,
) RuntimeState {
	switch status.Phase() {
	case amp.ApplicationPhaseIdle:
		return RuntimeStateIdle

	case amp.ApplicationPhaseOnline:
		return RuntimeStateOnline

	case amp.ApplicationPhaseBusy:
		return RuntimeStateBusy

	case amp.ApplicationPhaseFailed,
		amp.ApplicationPhaseSuspended:
		return RuntimeStateFailed

	default:
		return RuntimeStateUnknown
	}
}
