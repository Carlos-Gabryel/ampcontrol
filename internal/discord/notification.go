package discord

import "fmt"

// SendNotification envia uma mensagem operacional para o canal
// de notificações configurado no Discord.
func (c *Client) SendNotification(
	content string,
) error {
	if c == nil {
		return fmt.Errorf(
			"o cliente Discord não está disponível",
		)
	}

	err := c.sendChannelMessage(
		content,
	)
	c.requestStatusRefresh()

	return err
}
