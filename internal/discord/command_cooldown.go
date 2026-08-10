package discord

import (
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	disgoDiscord "github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
)

type ampCommandCooldownScope string

const (
	ampCommandCooldownScopeNone   ampCommandCooldownScope = "none"
	ampCommandCooldownScopeUser   ampCommandCooldownScope = "user"
	ampCommandCooldownScopeServer ampCommandCooldownScope = "server"
)

type ampCommandCooldownDecision struct {
	Allowed   bool
	Scope     ampCommandCooldownScope
	Server    string
	Remaining time.Duration
}

type ampCommandCooldowns struct {
	mu           sync.Mutex
	userWindow   time.Duration
	serverWindow time.Duration
	users        map[snowflake.ID]time.Time
	servers      map[string]time.Time
	now          func() time.Time
}

func newAMPCommandCooldowns(
	userWindow time.Duration,
	serverWindow time.Duration,
) *ampCommandCooldowns {
	return &ampCommandCooldowns{
		userWindow:   userWindow,
		serverWindow: serverWindow,
		users:        make(map[snowflake.ID]time.Time),
		servers:      make(map[string]time.Time),
		now:          time.Now,
	}
}

func (c *Client) reserveAMPCommandCooldown(
	userID snowflake.ID,
	data disgoDiscord.SlashCommandInteractionData,
) ampCommandCooldownDecision {
	if c.commandCooldowns == nil {
		return ampCommandCooldownDecision{Allowed: true}
	}

	server, _ := data.OptString("servidor")
	return c.commandCooldowns.reserve(userID, server)
}

func (c *ampCommandCooldowns) reserve(
	userID snowflake.ID,
	server string,
) ampCommandCooldownDecision {
	if c == nil {
		return ampCommandCooldownDecision{Allowed: true}
	}

	now := c.now()
	server = strings.TrimSpace(server)
	serverKey := strings.ToLower(server)

	c.mu.Lock()
	defer c.mu.Unlock()

	c.removeExpired(now)

	if c.userWindow > 0 {
		if reservedAt, exists := c.users[userID]; exists {
			remaining := c.userWindow - now.Sub(reservedAt)
			if remaining > 0 {
				return ampCommandCooldownDecision{
					Scope:     ampCommandCooldownScopeUser,
					Server:    server,
					Remaining: remaining,
				}
			}
		}
	}

	if serverKey != "" && c.serverWindow > 0 {
		if reservedAt, exists := c.servers[serverKey]; exists {
			remaining := c.serverWindow - now.Sub(reservedAt)
			if remaining > 0 {
				return ampCommandCooldownDecision{
					Scope:     ampCommandCooldownScopeServer,
					Server:    server,
					Remaining: remaining,
				}
			}
		}
	}

	if c.userWindow > 0 {
		c.users[userID] = now
	}
	if serverKey != "" && c.serverWindow > 0 {
		c.servers[serverKey] = now
	}

	return ampCommandCooldownDecision{
		Allowed: true,
		Scope:   ampCommandCooldownScopeNone,
		Server:  server,
	}
}

func (c *ampCommandCooldowns) removeExpired(now time.Time) {
	for userID, reservedAt := range c.users {
		if c.userWindow <= 0 || now.Sub(reservedAt) >= c.userWindow {
			delete(c.users, userID)
		}
	}
	for server, reservedAt := range c.servers {
		if c.serverWindow <= 0 || now.Sub(reservedAt) >= c.serverWindow {
			delete(c.servers, server)
		}
	}
}

func (d ampCommandCooldownDecision) UserMessage() string {
	seconds := cooldownSeconds(d.Remaining)
	if d.Scope == ampCommandCooldownScopeServer {
		return fmt.Sprintf(
			"⏱️ Aguarde **%d s** antes de enviar outro comando para a instância `%s`.",
			seconds,
			d.Server,
		)
	}

	return fmt.Sprintf(
		"⏱️ Aguarde **%d s** antes de enviar outro comando `/amp`.",
		seconds,
	)
}

func (d ampCommandCooldownDecision) AuditReason() string {
	seconds := cooldownSeconds(d.Remaining)
	if d.Scope == ampCommandCooldownScopeServer {
		return fmt.Sprintf(
			"Recusado: cooldown da instância ativo por %d s",
			seconds,
		)
	}

	return fmt.Sprintf(
		"Recusado: cooldown do usuário ativo por %d s",
		seconds,
	)
}

func cooldownSeconds(duration time.Duration) int {
	return int(math.Max(1, math.Ceil(duration.Seconds())))
}
