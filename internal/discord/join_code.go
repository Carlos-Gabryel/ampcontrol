package discord

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/Carlos-Gabryel/ampcontrol/internal/amp"
)

// joinCodeEntry guarda o código de convite do processo em execução e o
// maior uptime já visto, para detectar reinícios.
type joinCodeEntry struct {
	code          string
	uptimeSeconds int
	checkedAt     time.Time
}

// valheimJoinCodeRecheckInterval é o intervalo mínimo entre leituras do
// console quando já existe um código, pois o código pode mudar sem reinício.
const valheimJoinCodeRecheckInterval = 5 * time.Minute

func (c *Client) joinCodeClock() time.Time {
	if c.joinCodeNow != nil {
		return c.joinCodeNow()
	}

	return time.Now()
}

func isValheimGame(game string) bool {
	return strings.Contains(strings.ToLower(game), "valheim")
}

// ampUptimeSeconds converte o uptime do AMP ("dias:horas:minutos:segundos").
func ampUptimeSeconds(raw string) (int, bool) {
	parts := strings.Split(strings.TrimSpace(raw), ":")
	if len(parts) != 4 {
		return 0, false
	}

	multipliers := [4]int{86400, 3600, 60, 1}
	total := 0
	for index, part := range parts {
		value, err := strconv.Atoi(part)
		if err != nil || value < 0 {
			return 0, false
		}
		total += value * multipliers[index]
	}

	return total, true
}

func (c *Client) clearJoinCode(instance string) {
	c.joinCodeMu.Lock()
	defer c.joinCodeMu.Unlock()

	delete(c.joinCodes, instance)
}

// resolveJoinCode devolve o código de convite do Valheim em execução.
// O console só é lido enquanto o código do processo atual é desconhecido;
// reinício (uptime menor) ou parada descartam o valor em cache.
func (c *Client) resolveJoinCode(
	instance amp.ManagedInstance,
	status amp.ApplicationStatus,
) string {
	if status.Phase() != amp.ApplicationPhaseOnline {
		c.clearJoinCode(instance.Name)
		return ""
	}

	uptime, uptimeKnown := ampUptimeSeconds(status.Uptime)

	c.joinCodeMu.Lock()
	cached, exists := c.joinCodes[instance.Name]
	if exists && uptimeKnown && uptime < cached.uptimeSeconds {
		delete(c.joinCodes, instance.Name)
		cached = joinCodeEntry{}
		exists = false
	}
	now := c.joinCodeClock()
	if exists && cached.code != "" {
		if uptimeKnown {
			cached.uptimeSeconds = uptime
			c.joinCodes[instance.Name] = cached
		}
		if now.Sub(cached.checkedAt) < valheimJoinCodeRecheckInterval {
			c.joinCodeMu.Unlock()
			return cached.code
		}
	}
	c.joinCodeMu.Unlock()

	ctx, cancel := context.WithTimeout(
		context.Background(),
		ampApplicationTimeout,
	)
	defer cancel()

	entries, err := c.ampClient.GetConsoleEntries(ctx, instance.APIURL)
	if err != nil {
		c.log.Warn().
			Err(err).
			Str("instance", instance.Name).
			Msg("Não foi possível ler o console para obter o código de convite")

		return cached.code
	}

	code := amp.ExtractValheimJoinCode(entries)
	if code == "" {
		// A linha pode ter saído do histórico: mantém o código conhecido.
		code = cached.code
	}

	c.joinCodeMu.Lock()
	defer c.joinCodeMu.Unlock()

	if c.joinCodes == nil {
		c.joinCodes = map[string]joinCodeEntry{}
	}
	c.joinCodes[instance.Name] = joinCodeEntry{code: code, uptimeSeconds: uptime, checkedAt: now}

	return code
}
