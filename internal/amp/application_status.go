package amp

import (
	"context"
	"fmt"
	"strings"
)

type ApplicationState int

const (
	ApplicationStateUndefined         ApplicationState = -1
	ApplicationStateStopped           ApplicationState = 0
	ApplicationStatePreStart          ApplicationState = 5
	ApplicationStateConfiguring       ApplicationState = 7
	ApplicationStateStarting          ApplicationState = 10
	ApplicationStateReady             ApplicationState = 20
	ApplicationStateRestarting        ApplicationState = 30
	ApplicationStateStopping          ApplicationState = 40
	ApplicationStatePreparingForSleep ApplicationState = 45
	ApplicationStateSleeping          ApplicationState = 50
	ApplicationStateWaiting           ApplicationState = 60
	ApplicationStateInstalling        ApplicationState = 70
	ApplicationStateUpdating          ApplicationState = 75
	ApplicationStateAwaitingUserInput ApplicationState = 80
	ApplicationStateFailed            ApplicationState = 100
	ApplicationStateSuspended         ApplicationState = 200
	ApplicationStateMaintenance       ApplicationState = 250
	ApplicationStateIndeterminate     ApplicationState = 999
)

type ApplicationPhase string

const (
	ApplicationPhaseIdle      ApplicationPhase = "idle"
	ApplicationPhaseOnline    ApplicationPhase = "online"
	ApplicationPhaseBusy      ApplicationPhase = "busy"
	ApplicationPhaseFailed    ApplicationPhase = "failed"
	ApplicationPhaseSuspended ApplicationPhase = "suspended"
	ApplicationPhaseUnknown   ApplicationPhase = "unknown"
)

type ApplicationStatus struct {
	State   ApplicationState        `json:"State"`
	Uptime  string                  `json:"Uptime"`
	Metrics map[string]StatusMetric `json:"Metrics"`
}

// Phase retorna uma classificação simplificada do estado atual
// da aplicação hospedada pela instância AMP.
func (s ApplicationStatus) Phase() ApplicationPhase {
	return s.State.Phase()
}

// GetApplicationStatus autentica diretamente na instância AMP e
// consulta o estado real do processo do jogo usando Core.GetStatus.
func (c *APIClient) GetApplicationStatus(
	ctx context.Context,
	baseURL string,
) (ApplicationStatus, error) {
	baseURL = strings.TrimSpace(baseURL)

	if baseURL == "" {
		return ApplicationStatus{}, fmt.Errorf(
			"a URL da instância AMP não foi informada",
		)
	}

	sessionID, err := c.login(
		ctx,
		baseURL,
	)
	if err != nil {
		return ApplicationStatus{}, err
	}

	requestBody := authenticatedRequest{
		SessionID: sessionID,
	}

	var response ApplicationStatus

	err = c.postJSON(
		ctx,
		buildAPIURL(
			baseURL,
			"Core",
			"GetStatus",
		),
		sessionID,
		requestBody,
		&response,
	)
	if err != nil {
		return ApplicationStatus{}, fmt.Errorf(
			"falha chamando Core.GetStatus: %w",
			err,
		)
	}

	return response, nil
}

func (s ApplicationState) Phase() ApplicationPhase {
	switch s {
	case ApplicationStateStopped,
		ApplicationStateSleeping:
		return ApplicationPhaseIdle

	case ApplicationStateReady:
		return ApplicationPhaseOnline

	case ApplicationStatePreStart,
		ApplicationStateConfiguring,
		ApplicationStateStarting,
		ApplicationStateRestarting,
		ApplicationStateStopping,
		ApplicationStatePreparingForSleep,
		ApplicationStateWaiting,
		ApplicationStateInstalling,
		ApplicationStateUpdating,
		ApplicationStateAwaitingUserInput:
		return ApplicationPhaseBusy

	case ApplicationStateFailed:
		return ApplicationPhaseFailed

	case ApplicationStateSuspended:
		return ApplicationPhaseSuspended

	default:
		return ApplicationPhaseUnknown
	}
}

func (s ApplicationState) String() string {
	switch s {
	case ApplicationStateUndefined:
		return "Undefined"

	case ApplicationStateStopped:
		return "Stopped"

	case ApplicationStatePreStart:
		return "PreStart"

	case ApplicationStateConfiguring:
		return "Configuring"

	case ApplicationStateStarting:
		return "Starting"

	case ApplicationStateReady:
		return "Ready"

	case ApplicationStateRestarting:
		return "Restarting"

	case ApplicationStateStopping:
		return "Stopping"

	case ApplicationStatePreparingForSleep:
		return "PreparingForSleep"

	case ApplicationStateSleeping:
		return "Sleeping"

	case ApplicationStateWaiting:
		return "Waiting"

	case ApplicationStateInstalling:
		return "Installing"

	case ApplicationStateUpdating:
		return "Updating"

	case ApplicationStateAwaitingUserInput:
		return "AwaitingUserInput"

	case ApplicationStateFailed:
		return "Failed"

	case ApplicationStateSuspended:
		return "Suspended"

	case ApplicationStateMaintenance:
		return "Maintenance"

	case ApplicationStateIndeterminate:
		return "Indeterminate"

	default:
		return fmt.Sprintf(
			"Unknown(%d)",
			int(s),
		)
	}
}
