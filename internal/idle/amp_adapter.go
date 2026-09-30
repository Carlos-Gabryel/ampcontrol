package idle

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Carlos-Gabryel/ampcontrol/internal/i18n"

	"github.com/Carlos-Gabryel/ampcontrol/internal/amp"
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

func NewAMPAdapterWithInventory(
	client AMPApplicationClient,
	inventory amp.InstanceDiscoverer,
) (*AMPAdapter, error) {
	if inventory == nil {
		return nil, errors.New(i18n.Choose("o inventário AMP não foi informado", "the AMP inventory was not provided"))
	}
	return newAMPAdapter(client, inventory.DiscoverInstances)
}

func newAMPAdapter(
	client AMPApplicationClient,
	discover ampDiscoverInstancesFunc,
) (*AMPAdapter, error) {
	if client == nil {
		return nil, errors.New(i18n.Choose("o cliente da API AMP não foi informado", "the AMP API client was not provided"))
	}

	if discover == nil {
		return nil, errors.New(i18n.Choose("a função de descoberta das instâncias AMP não foi informada", "the AMP instance discovery function was not provided"))
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
	observation, err := a.RuntimeObservation(ctx, server)
	return observation.State, err
}

// RuntimeObservation consulta estado e uptime na mesma chamada ao AMP.
func (a *AMPAdapter) RuntimeObservation(
	ctx context.Context,
	server Server,
) (RuntimeObservation, error) {
	instance, err := a.resolveInstance(
		ctx,
		server,
		true,
	)
	if err != nil {
		return RuntimeObservation{State: RuntimeStateUnknown}, err
	}

	if !instance.Running {
		return RuntimeObservation{State: RuntimeStateOffline}, nil
	}

	apiURL := strings.TrimSpace(
		instance.APIURL,
	)

	if apiURL == "" {
		return RuntimeObservation{State: RuntimeStateUnknown}, fmt.Errorf(
			i18n.Choose("a instância AMP %s está ligada, mas não possui URL de API", "AMP instance %s is running but has no API URL"),
			instance.Name,
		)
	}

	status, err := a.client.GetApplicationStatus(
		ctx,
		apiURL,
	)
	if err != nil {
		return RuntimeObservation{State: RuntimeStateUnknown}, fmt.Errorf(
			i18n.Choose("não foi possível consultar o estado da aplicação da instância %s: %w", "could not query the application state of instance %s: %w"),
			instance.Name,
			err,
		)
	}

	observation := RuntimeObservation{
		State: mapAMPApplicationStatus(status),
	}

	if uptime, uptimeErr := parseAMPApplicationUptime(status.Uptime); uptimeErr == nil {
		observation.Uptime = uptime
		observation.UptimeKnown = true
	}

	return observation, nil
}

func parseAMPApplicationUptime(raw string) (time.Duration, error) {
	parts := strings.Split(strings.TrimSpace(raw), ":")
	if len(parts) != 3 && len(parts) != 4 {
		return 0, fmt.Errorf("uptime AMP possui formato inesperado: %q", raw)
	}

	days := 0
	hoursIndex := 0
	if len(parts) == 4 {
		parsedDays, err := strconv.Atoi(parts[0])
		if err != nil || parsedDays < 0 {
			return 0, fmt.Errorf("dias invalidos no uptime AMP: %q", raw)
		}
		days = parsedDays
		hoursIndex = 1
	} else if dayHour := strings.SplitN(parts[0], ".", 2); len(dayHour) == 2 {
		parsedDays, daysErr := strconv.Atoi(dayHour[0])
		parsedHours, hoursErr := strconv.Atoi(dayHour[1])
		if daysErr != nil || hoursErr != nil || parsedDays < 0 || parsedHours < 0 {
			return 0, fmt.Errorf("dias ou horas invalidos no uptime AMP: %q", raw)
		}
		days = parsedDays
		parts[0] = dayHour[1]
	}

	hours, hoursErr := strconv.Atoi(parts[hoursIndex])
	minutes, minutesErr := strconv.Atoi(parts[hoursIndex+1])
	secondsText := strings.SplitN(parts[hoursIndex+2], ".", 2)[0]
	seconds, secondsErr := strconv.Atoi(secondsText)
	if hoursErr != nil || minutesErr != nil || secondsErr != nil ||
		hours < 0 || minutes < 0 || seconds < 0 ||
		minutes >= 60 || seconds >= 60 {
		return 0, fmt.Errorf("uptime AMP possui valores invalidos: %q", raw)
	}

	return time.Duration(days)*24*time.Hour +
		time.Duration(hours)*time.Hour +
		time.Duration(minutes)*time.Minute +
		time.Duration(seconds)*time.Second, nil
}

// PlayerCount consulta Core.GetStatus diretamente na instância e valida
// a métrica Active Users. Valores ausentes, fracionários ou incoerentes
// retornam erro para preservar o comportamento fail-open do motor de Idle.
func (a *AMPAdapter) PlayerCount(
	ctx context.Context,
	server Server,
) (int, error) {
	instance, err := a.resolveInstance(ctx, server, true)
	if err != nil {
		return 0, err
	}

	if !instance.Running {
		return 0, fmt.Errorf(
			i18n.Choose("a instância AMP %s está Offline", "AMP instance %s is Offline"),
			instance.Name,
		)
	}

	apiURL := strings.TrimSpace(instance.APIURL)
	if apiURL == "" {
		return 0, fmt.Errorf(
			i18n.Choose("a instância AMP %s está ligada, mas não possui URL de API", "AMP instance %s is running but has no API URL"),
			instance.Name,
		)
	}

	status, err := a.client.GetApplicationStatus(ctx, apiURL)
	if err != nil {
		return 0, fmt.Errorf(
			i18n.Choose("não foi possível consultar jogadores da instância %s: %w", "could not query players of instance %s: %w"),
			instance.Name,
			err,
		)
	}

	counts, err := status.PlayerCounts()
	if err != nil {
		return 0, fmt.Errorf(
			i18n.Choose("a contagem de jogadores da instância %s é inválida: %w", "the player count of instance %s is invalid: %w"),
			instance.Name,
			err,
		)
	}

	return counts.Current, nil
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
			i18n.Choose("a instância AMP %s ficou Offline antes da parada automática", "AMP instance %s went Offline before the automatic stop"),
			instance.Name,
		)
	}

	apiURL := strings.TrimSpace(
		instance.APIURL,
	)

	if apiURL == "" {
		return fmt.Errorf(
			i18n.Choose("a instância AMP %s está ligada, mas não possui URL de API", "AMP instance %s is running but has no API URL"),
			instance.Name,
		)
	}

	status, err := a.client.GetApplicationStatus(
		ctx,
		apiURL,
	)
	if err != nil {
		return fmt.Errorf(
			i18n.Choose("não foi possível confirmar o estado da aplicação da instância %s antes da parada: %w", "could not confirm the application state of instance %s before stopping: %w"),
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
			i18n.Choose("a aplicação da instância %s entrou em transição antes da parada automática; estado AMP: %s", "the application of instance %s entered a transition before the automatic stop; AMP state: %s"),
			instance.Name,
			status.State.String(),
		)

	case RuntimeStateFailed:
		return fmt.Errorf(
			i18n.Choose("a aplicação da instância %s está em estado de falha ou suspensão; estado AMP: %s", "the application of instance %s is failed or suspended; AMP state: %s"),
			instance.Name,
			status.State.String(),
		)

	case RuntimeStateOffline:
		return fmt.Errorf(
			i18n.Choose("a aplicação da instância %s ficou Offline antes da parada automática", "the application of instance %s went Offline before the automatic stop"),
			instance.Name,
		)

	default:
		return fmt.Errorf(
			i18n.Choose("o estado da aplicação da instância %s não é reconhecido; estado AMP: %s", "the application state of instance %s is not recognized; AMP state: %s"),
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
		return amp.ManagedInstance{}, errors.New(i18n.Choose("o adaptador AMP não foi inicializado", "the AMP adapter was not initialized"))
	}

	if ctx == nil {
		return amp.ManagedInstance{}, errors.New(i18n.Choose("o contexto da operação AMP é nulo", "the AMP operation context is nil"))
	}

	if err := ctx.Err(); err != nil {
		return amp.ManagedInstance{}, fmt.Errorf(
			i18n.Choose("o contexto da operação AMP foi encerrado: %w", "the AMP operation context has ended: %w"),
			err,
		)
	}

	if a.client == nil {
		return amp.ManagedInstance{}, errors.New(i18n.Choose("o cliente da API AMP não foi inicializado", "the AMP API client was not initialized"))
	}

	if a.discover == nil {
		return amp.ManagedInstance{}, errors.New(i18n.Choose("a descoberta das instâncias AMP não foi inicializada", "AMP instance discovery was not initialized"))
	}

	if a.now == nil {
		return amp.ManagedInstance{}, errors.New(i18n.Choose("o relógio interno do adaptador AMP não foi inicializado", "the AMP adapter's internal clock was not initialized"))
	}

	instanceName := strings.TrimSpace(
		server.Instance,
	)

	if instanceName == "" {
		return amp.ManagedInstance{}, errors.New(i18n.Choose("o servidor de Idle não informou o nome da instância AMP", "the Idle server did not provide the AMP instance name"))
	}

	instances, err := a.discoverInstances(
		ctx,
		allowCache,
	)
	if err != nil {
		return amp.ManagedInstance{}, fmt.Errorf(
			i18n.Choose("não foi possível descobrir as instâncias AMP: %w", "could not discover the AMP instances: %w"),
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

	notFound := fmt.Errorf(
		i18n.Choose("a instância AMP %q configurada no monitor de Idle não foi encontrada", "AMP instance %q configured in the Idle monitor was not found"),
		instanceName,
	)
	// Um inventário vazio não prova que a instância foi apagada.
	if len(instances) == 0 {
		return amp.ManagedInstance{}, notFound
	}
	return amp.ManagedInstance{}, &instanceNotFoundError{err: notFound}
}

// ErrInstanceNotFound indica que o inventário AMP foi lido com sucesso e
// a instância configurada não está nele.
var ErrInstanceNotFound = errors.New("AMP instance not found in inventory")

type instanceNotFoundError struct {
	err error
}

func (e *instanceNotFoundError) Error() string { return e.err.Error() }

func (e *instanceNotFoundError) Is(target error) bool { return target == ErrInstanceNotFound }

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
