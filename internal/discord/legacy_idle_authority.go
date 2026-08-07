package discord

import "strings"

// partitionLegacyIdleServers separa os servidores que continuam sob
// autoridade do monitor antigo daqueles cuja autoridade foi transferida
// para o motor genérico de Idle.
//
// A lista transferredInstances deve ser derivada do mesmo snapshot de
// configuração utilizado pelo motor genérico.
func partitionLegacyIdleServers(
	servers []managedServer,
	transferredInstances []string,
) ([]managedServer, []managedServer) {
	transferredNames := make(
		map[string]struct{},
		len(transferredInstances),
	)

	for _, instance := range transferredInstances {
		instance = strings.TrimSpace(
			instance,
		)

		if instance == "" {
			continue
		}

		transferredNames[instance] = struct{}{}
	}

	managed := make(
		[]managedServer,
		0,
		len(servers),
	)

	transferred := make(
		[]managedServer,
		0,
		len(servers),
	)

	for _, server := range servers {
		instance := strings.TrimSpace(
			server.Instance.Name,
		)

		if _, exists := transferredNames[instance]; exists {
			transferred = append(
				transferred,
				server,
			)

			continue
		}

		managed = append(
			managed,
			server,
		)
	}

	return managed, transferred
}
