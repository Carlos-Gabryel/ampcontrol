package amp

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

type ServerState string

const (
	ServerStateOffline ServerState = "offline"
	ServerStateIdle    ServerState = "idle"
	ServerStateOnline  ServerState = "online"
)

type Instance struct {
	Name     string
	GamePort int
}

// GetServerStatuses retorna o estado real de cada servidor.
//
// Online:
// a porta UDP do jogo está aberta.
//
// Idle:
// a instância AMP está rodando, mas a porta UDP do jogo está fechada.
//
// Offline:
// a instância AMP está parada e a porta UDP do jogo está fechada.
func GetServerStatuses(
	ctx context.Context,
	instances []Instance,
) (map[string]ServerState, error) {
	ampStatuses, err := getAMPInstanceStatuses(
		ctx,
		instances,
	)
	if err != nil {
		return nil, err
	}

	statuses := make(
		map[string]ServerState,
		len(instances),
	)

	for _, instance := range instances {
		portOpen, err := isUDPPortOpen(instance.GamePort)
		if err != nil {
			return nil, fmt.Errorf(
				"não foi possível verificar a porta UDP %d da instância %s: %w",
				instance.GamePort,
				instance.Name,
				err,
			)
		}

		switch {
		case portOpen:
			statuses[instance.Name] = ServerStateOnline

		case ampStatuses[instance.Name]:
			statuses[instance.Name] = ServerStateIdle

		default:
			statuses[instance.Name] = ServerStateOffline
		}
	}

	return statuses, nil
}

func getAMPInstanceStatuses(
	ctx context.Context,
	instances []Instance,
) (map[string]bool, error) {
	runtime := currentRuntimeConfig()
	command := exec.CommandContext(
		ctx,
		runtime.SudoPath,
		"-n",
		"-u",
		runtime.SystemUser,
		runtime.ManagerPath,
		"status",
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
				"a consulta ao AMP excedeu o tempo limite: %w",
				ctx.Err(),
			)
		}

		return nil, fmt.Errorf(
			"não foi possível executar ampinstmgr como o usuário %s: %w; saída: %s",
			runtime.SystemUser,
			err,
			strings.TrimSpace(string(output)),
		)
	}

	return parseAMPStatuses(
		string(output),
		instances,
	)
}

func parseAMPStatuses(
	output string,
	instances []Instance,
) (map[string]bool, error) {
	statuses := make(
		map[string]bool,
		len(instances),
	)

	found := make(
		map[string]bool,
		len(instances),
	)

	for _, instance := range instances {
		statuses[instance.Name] = false
		found[instance.Name] = false
	}

	scanner := bufio.NewScanner(
		strings.NewReader(output),
	)

	for scanner.Scan() {
		line := stripTerminalSequences(
			scanner.Text(),
		)

		for _, instance := range instances {
			if !strings.Contains(line, instance.Name) {
				continue
			}

			found[instance.Name] = true

			statuses[instance.Name] =
				strings.Contains(line, "✓") ||
					strings.Contains(line, "√")
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf(
			"erro lendo a saída do ampinstmgr: %w",
			err,
		)
	}

	var missing []string

	for _, instance := range instances {
		if !found[instance.Name] {
			missing = append(
				missing,
				instance.Name,
			)
		}
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf(
			"as seguintes instâncias não apareceram no ampinstmgr status: %s",
			strings.Join(missing, ", "),
		)
	}

	return statuses, nil
}

// isUDPPortOpen verifica as tabelas de sockets UDP do Linux.
//
// /proc/net/udp contém sockets IPv4.
// /proc/net/udp6 contém sockets IPv6.
func isUDPPortOpen(port int) (bool, error) {
	files := []string{
		"/proc/net/udp",
		"/proc/net/udp6",
	}

	for _, filePath := range files {
		open, err := portExistsInProcFile(
			filePath,
			port,
		)
		if err != nil {
			return false, err
		}

		if open {
			return true, nil
		}
	}

	return false, nil
}

func portExistsInProcFile(
	filePath string,
	targetPort int,
) (bool, error) {
	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}

		return false, fmt.Errorf(
			"não foi possível abrir %s: %w",
			filePath,
			err,
		)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	firstLine := true

	for scanner.Scan() {
		if firstLine {
			firstLine = false
			continue
		}

		fields := strings.Fields(
			scanner.Text(),
		)

		if len(fields) < 2 {
			continue
		}

		localAddress := fields[1]

		addressParts := strings.Split(
			localAddress,
			":",
		)

		if len(addressParts) != 2 {
			continue
		}

		portValue, err := strconv.ParseInt(
			addressParts[1],
			16,
			32,
		)
		if err != nil {
			continue
		}

		if int(portValue) == targetPort {
			return true, nil
		}
	}

	if err := scanner.Err(); err != nil {
		return false, fmt.Errorf(
			"erro lendo %s: %w",
			filePath,
			err,
		)
	}

	return false, nil
}

func stripTerminalSequences(input string) string {
	var output strings.Builder

	for index := 0; index < len(input); {
		if input[index] != 0x1b {
			output.WriteByte(input[index])
			index++
			continue
		}

		index++

		if index >= len(input) {
			break
		}

		switch input[index] {
		case '[':
			index++

			for index < len(input) {
				current := input[index]
				index++

				if current >= 0x40 &&
					current <= 0x7e {
					break
				}
			}

		case ']':
			index++

			for index < len(input) {
				if input[index] == 0x07 {
					index++
					break
				}

				if input[index] == 0x1b &&
					index+1 < len(input) &&
					input[index+1] == '\\' {
					index += 2
					break
				}

				index++
			}

		default:
			index++
		}
	}

	return output.String()
}
