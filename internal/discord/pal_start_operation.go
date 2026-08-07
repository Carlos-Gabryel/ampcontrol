package discord

const palStartOperationName = "comando /pal iniciar"

// acquirePalStartOperation reserva exclusivamente a instância usada
// pelo comando legado /pal iniciar.
//
// O bloqueio utiliza o mesmo operation.Manager compartilhado pelos
// comandos /amp, pelo monitor antigo e pelo motor genérico de Idle.
func (c *Client) acquirePalStartOperation(
	server managedServer,
) (
	release func(),
	activeOperation string,
	acquired bool,
	err error,
) {
	return c.acquireSharedOperation(
		server.Instance.Name,
		palStartOperationName,
	)
}
