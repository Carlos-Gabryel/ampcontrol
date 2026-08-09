package amp

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
)

const ampcontrolAMPWrapperPath = "/usr/local/bin/ampcontrol-amp"

type ManagedInstance struct {
	ID           string
	Name         string
	FriendlyName string
	Module       string
	APIURL       string
	Running      bool
}

// DiscoverInstances consulta o AMP e retorna todas as instâncias
// que podem ser administradas pelo AmpControl.
//
// A instância ADS01 e qualquer módulo ADS são excluídos.
func DiscoverInstances(
	ctx context.Context,
) ([]ManagedInstance, error) {
	command := exec.CommandContext(
		ctx,
		sudoPath,
		"-n",
		"-u",
		ampSystemUser,
		ampcontrolAMPWrapperPath,
		"list",
	)

	command.Env = append(
		os.Environ(),
		"TERM=dumb",
		"NO_COLOR=1",
	)

	output, err := command.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf(
				"a descoberta das instâncias excedeu o tempo limite: %w",
				ctx.Err(),
			)
		}

		return nil, fmt.Errorf(
			"não foi possível listar as instâncias AMP: %w; saída: %s",
			err,
			strings.TrimSpace(string(output)),
		)
	}

	instances, err := parseInstancesList(
		string(output),
	)
	if err != nil {
		return nil, err
	}

	controllable := make(
		[]ManagedInstance,
		0,
		len(instances),
	)

	for _, instance := range instances {
		if instance.Name == "ADS01" {
			continue
		}

		if strings.EqualFold(
			instance.Module,
			"ADS",
		) {
			continue
		}

		controllable = append(
			controllable,
			instance,
		)
	}

	sort.Slice(
		controllable,
		func(left int, right int) bool {
			leftName := strings.ToLower(
				controllable[left].FriendlyName,
			)

			rightName := strings.ToLower(
				controllable[right].FriendlyName,
			)

			if leftName == rightName {
				return controllable[left].Name <
					controllable[right].Name
			}

			return leftName < rightName
		},
	)

	return controllable, nil
}

func parseInstancesList(
	output string,
) ([]ManagedInstance, error) {
	instances := make(
		[]ManagedInstance,
		0,
	)

	current := ManagedInstance{}

	appendCurrent := func() {
		if strings.TrimSpace(current.Name) == "" {
			return
		}

		if strings.TrimSpace(current.FriendlyName) == "" {
			current.FriendlyName = current.Name
		}

		instances = append(
			instances,
			current,
		)

		current = ManagedInstance{}
	}

	scanner := bufio.NewScanner(
		strings.NewReader(output),
	)

	for scanner.Scan() {
		line := strings.TrimSpace(
			stripTerminalSequences(
				scanner.Text(),
			),
		)

		if line == "" {
			continue
		}

		parts := strings.SplitN(
			line,
			"│",
			2,
		)

		if len(parts) != 2 {
			continue
		}

		field := strings.TrimSpace(
			parts[0],
		)

		value := strings.TrimSpace(
			parts[1],
		)

		if field == "Instance ID" &&
			strings.TrimSpace(current.Name) != "" {
			appendCurrent()
		}

		switch field {
		case "Instance ID":
			current.ID = value

		case "Module":
			current.Module = value

		case "Instance Name":
			current.Name = value

		case "Friendly Name":
			current.FriendlyName = value

		case "URL":
			current.APIURL = normalizeInstanceURL(
				value,
			)

		case "Running":
			current.Running = strings.EqualFold(
				value,
				"Yes",
			)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf(
			"erro lendo a lista de instâncias AMP: %w",
			err,
		)
	}

	appendCurrent()

	if len(instances) == 0 {
		return nil, fmt.Errorf(
			"o AMP não retornou nenhuma instância reconhecível",
		)
	}

	names := make(
		map[string]struct{},
		len(instances),
	)

	for _, instance := range instances {
		if strings.TrimSpace(instance.Name) == "" {
			return nil, fmt.Errorf(
				"o AMP retornou uma instância sem nome",
			)
		}

		if _, exists := names[instance.Name]; exists {
			return nil, fmt.Errorf(
				"o AMP retornou a instância %q mais de uma vez",
				instance.Name,
			)
		}

		names[instance.Name] = struct{}{}
	}

	return instances, nil
}

func normalizeInstanceURL(
	value string,
) string {
	value = strings.TrimSpace(value)

	if strings.HasPrefix(value, "[") {
		separator := strings.Index(
			value,
			"](",
		)

		if separator >= 0 &&
			strings.HasSuffix(value, ")") {
			return strings.TrimSpace(
				value[separator+2 : len(value)-1],
			)
		}
	}

	return value
}
