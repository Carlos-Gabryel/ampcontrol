package amp

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type InstanceOperation string

const (
	InstanceOperationStart   InstanceOperation = "start"
	InstanceOperationStop    InstanceOperation = "stop"
	InstanceOperationRestart InstanceOperation = "restart"
	InstanceOperationUpdate  InstanceOperation = "update"
)

// ControlInstance executa uma operação permitida sobre uma instância AMP.
//
// A validação definitiva do nome da instância e a proteção do ADS01
// também são aplicadas pelo wrapper /usr/local/bin/ampcontrol-amp.
func ControlInstance(
	ctx context.Context,
	operation InstanceOperation,
	instanceName string,
) error {
	instanceName = strings.TrimSpace(instanceName)

	if instanceName == "" {
		return fmt.Errorf(
			"o nome da instância não foi informado",
		)
	}

	if strings.EqualFold(instanceName, "ADS01") {
		return fmt.Errorf("a instância ADS01 é protegida")
	}

	if !isValidInstanceOperation(operation) {
		return fmt.Errorf(
			"operação AMP inválida: %q",
			operation,
		)
	}

	command := exec.CommandContext(
		ctx,
		sudoPath,
		"-n",
		"-u",
		ampSystemUser,
		ampcontrolAMPWrapperPath,
		string(operation),
		instanceName,
	)

	command.Env = append(
		os.Environ(),
		"TERM=dumb",
		"NO_COLOR=1",
	)

	output, err := command.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf(
				"a operação %s na instância %s excedeu o tempo limite: %w",
				operation,
				instanceName,
				ctx.Err(),
			)
		}

		message := strings.TrimSpace(
			stripTerminalSequences(
				string(output),
			),
		)

		if message == "" {
			message = "o controlador não retornou detalhes"
		}

		return fmt.Errorf(
			"não foi possível executar %s na instância %s: %w; saída: %s",
			operation,
			instanceName,
			err,
			message,
		)
	}

	return nil
}

func StartInstance(
	ctx context.Context,
	instanceName string,
) error {
	return ControlInstance(
		ctx,
		InstanceOperationStart,
		instanceName,
	)
}

func StopInstance(
	ctx context.Context,
	instanceName string,
) error {
	return ControlInstance(
		ctx,
		InstanceOperationStop,
		instanceName,
	)
}

func RestartInstance(
	ctx context.Context,
	instanceName string,
) error {
	return ControlInstance(
		ctx,
		InstanceOperationRestart,
		instanceName,
	)
}

func UpdateInstance(
	ctx context.Context,
	instanceName string,
) error {
	return ControlInstance(
		ctx,
		InstanceOperationUpdate,
		instanceName,
	)
}

func isValidInstanceOperation(
	operation InstanceOperation,
) bool {
	switch operation {
	case InstanceOperationStart,
		InstanceOperationStop,
		InstanceOperationRestart,
		InstanceOperationUpdate:
		return true

	default:
		return false
	}
}
