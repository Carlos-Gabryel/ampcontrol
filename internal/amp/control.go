package amp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/Carlos-Gabryel/ampcontrol/internal/i18n"
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
		return errors.New(i18n.Choose("o nome da instância não foi informado", "the instance name was not provided"))
	}

	if strings.EqualFold(instanceName, "ADS01") {
		return errors.New(i18n.Choose("a instância ADS01 é protegida", "the ADS01 instance is protected"))
	}

	if !isValidInstanceOperation(operation) {
		return fmt.Errorf(
			i18n.Choose("operação AMP inválida: %q", "invalid AMP operation: %q"),
			operation,
		)
	}

	runtime := currentRuntimeConfig()
	command := exec.CommandContext(
		ctx,
		runtime.SudoPath,
		"-n",
		"-u",
		runtime.SystemUser,
		runtime.WrapperPath,
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
			message = i18n.Choose("o controlador não retornou detalhes", "the controller returned no details")
		}

		return fmt.Errorf(
			i18n.Choose("não foi possível executar %s na instância %s: %w; saída: %s", "could not run %s on instance %s: %w; output: %s"),
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
