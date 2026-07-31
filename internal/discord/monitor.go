package discord

import (
	"context"
	"fmt"
	"time"

	"github.com/alabamaamp/palcontrol/internal/amp"
	"github.com/alabamaamp/palcontrol/internal/rcon"
)

const idleMonitorInterval = 30 * time.Second

func (c *Client) runIdleMonitor(
	ctx context.Context,
) {
	emptySince := make(
		map[string]time.Time,
		len(c.managedServers),
	)

	c.log.Info().
		Dur("check_interval", idleMonitorInterval).
		Dur("idle_timeout", c.idleTimeout).
		Msg("Monitor de inatividade iniciado")

	c.checkIdleServers(
		ctx,
		emptySince,
	)

	ticker := time.NewTicker(
		idleMonitorInterval,
	)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			c.log.Info().
				Msg("Monitor de inatividade encerrado")

			return

		case <-ticker.C:
			c.checkIdleServers(
				ctx,
				emptySince,
			)
		}
	}
}

func (c *Client) checkIdleServers(
	ctx context.Context,
	emptySince map[string]time.Time,
) {
	statusCtx, statusCancel := context.WithTimeout(
		ctx,
		5*time.Second,
	)

	statuses, err := amp.GetServerStatuses(
		statusCtx,
		c.serverInstances(),
	)

	statusCancel()

	if err != nil {
		c.log.Warn().
			Err(err).
			Msg("Monitor não conseguiu consultar o estado dos servidores")

		return
	}

	for _, server := range c.managedServers {
		if ctx.Err() != nil {
			return
		}

		state := statuses[server.Instance.Name]

		if state != amp.ServerStateOnline {
			delete(
				emptySince,
				server.Instance.Name,
			)

			continue
		}

		playerCount, err := c.getPlayerCount(
			ctx,
			server,
		)
		if err != nil {
			delete(
				emptySince,
				server.Instance.Name,
			)

			c.log.Warn().
				Err(err).
				Str("server", server.Instance.Name).
				Msg("Monitor não conseguiu consultar os jogadores")

			continue
		}

		c.log.Debug().
			Str("server", server.Instance.Name).
			Int("players", playerCount).
			Msg("Jogadores consultados pelo RCON")

		if playerCount > 0 {
			if _, existed := emptySince[server.Instance.Name]; existed {
				c.log.Info().
					Str("server", server.Instance.Name).
					Int("players", playerCount).
					Msg("Contador de inatividade cancelado")
			}

			delete(
				emptySince,
				server.Instance.Name,
			)

			continue
		}

		startedAt, exists := emptySince[server.Instance.Name]
		if !exists {
			emptySince[server.Instance.Name] = time.Now()

			c.log.Info().
				Str("server", server.Instance.Name).
				Dur("idle_timeout", c.idleTimeout).
				Msg("Servidor sem jogadores; contador de inatividade iniciado")

			continue
		}

		elapsed := time.Since(
			startedAt,
		)

		if elapsed < c.idleTimeout {
			c.log.Debug().
				Str("server", server.Instance.Name).
				Dur("idle_elapsed", elapsed).
				Dur("idle_remaining", c.idleTimeout-elapsed).
				Msg("Servidor continua sem jogadores")

			continue
		}

		c.confirmAndStopServer(
			ctx,
			server,
			emptySince,
		)
	}
}

func (c *Client) getPlayerCount(
	ctx context.Context,
	server managedServer,
) (int, error) {
	rconCtx, rconCancel := context.WithTimeout(
		ctx,
		10*time.Second,
	)
	defer rconCancel()

	playerCount, err := rcon.PlayerCount(
		rconCtx,
		server.RCONAddress,
		server.RCONPassword,
	)
	if err != nil {
		return 0, fmt.Errorf(
			"consulta RCON falhou: %w",
			err,
		)
	}

	return playerCount, nil
}

func (c *Client) confirmAndStopServer(
	ctx context.Context,
	server managedServer,
	emptySince map[string]time.Time,
) {
	playerCount, err := c.getPlayerCount(
		ctx,
		server,
	)
	if err != nil {
		delete(
			emptySince,
			server.Instance.Name,
		)

		c.log.Warn().
			Err(err).
			Str("server", server.Instance.Name).
			Msg("Falha na confirmação final de inatividade")

		return
	}

	if playerCount > 0 {
		delete(
			emptySince,
			server.Instance.Name,
		)

		c.log.Info().
			Str("server", server.Instance.Name).
			Int("players", playerCount).
			Msg("Parada cancelada porque jogadores entraram")

		return
	}

	warningMessage := fmt.Sprintf(
		"⚠️ **%s está sem jogadores há %s.**\n"+
			"O processo do Palworld será encerrado e o servidor entrará em modo Idle.",
		server.DisplayName,
		formatDuration(c.idleTimeout),
	)

	if err := c.sendChannelMessage(warningMessage); err != nil {
		c.log.Warn().
			Err(err).
			Str("server", server.Instance.Name).
			Msg("Não foi possível enviar o aviso de inatividade")
	}

	stopCtx, stopCancel := context.WithTimeout(
		ctx,
		20*time.Second,
	)

	err = c.ampClient.StopApplication(
		stopCtx,
		server.APIURL,
	)

	stopCancel()

	if err != nil {
		delete(
			emptySince,
			server.Instance.Name,
		)

		c.log.Error().
			Err(err).
			Str("server", server.Instance.Name).
			Msg("Erro encerrando o servidor por inatividade")

		failureMessage := fmt.Sprintf(
			"❌ Não foi possível colocar **%s** em modo Idle automaticamente.\n"+
				"O erro foi registrado pelo PalControl.",
			server.DisplayName,
		)

		if notificationErr := c.sendChannelMessage(
			failureMessage,
		); notificationErr != nil {
			c.log.Warn().
				Err(notificationErr).
				Str("server", server.Instance.Name).
				Msg("Não foi possível enviar a falha de parada")
		}

		return
	}

	delete(
		emptySince,
		server.Instance.Name,
	)

	c.log.Info().
		Str("server", server.Instance.Name).
		Msg("Processo do jogo encerrado por inatividade")

	successMessage := fmt.Sprintf(
		"💤 **%s entrou em modo Idle por inatividade.**\n"+
			"Use `/pal iniciar servidor:%s` quando quiser jogar novamente.",
		server.DisplayName,
		server.DisplayName,
	)

	if err := c.sendChannelMessage(successMessage); err != nil {
		c.log.Warn().
			Err(err).
			Str("server", server.Instance.Name).
			Msg("Não foi possível enviar a confirmação de Idle")
	}
}

func formatDuration(
	duration time.Duration,
) string {
	minutes := int(
		duration.Round(time.Minute).Minutes(),
	)

	if minutes == 1 {
		return "1 minuto"
	}

	return fmt.Sprintf(
		"%d minutos",
		minutes,
	)
}
