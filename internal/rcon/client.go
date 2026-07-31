package rcon

import (
	"context"
	"encoding/binary"
	"encoding/csv"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

const (
	responseValuePacketType  int32 = 0
	executeCommandPacketType int32 = 2
	authResponsePacketType   int32 = 2
	authPacketType           int32 = 3

	authPacketID    int32 = 100
	commandPacketID int32 = 200

	maximumPacketSize = 4 * 1024 * 1024
)

func PlayerCount(
	ctx context.Context,
	address string,
	password string,
) (int, error) {
	dialer := net.Dialer{
		Timeout: 5 * time.Second,
	}

	connection, err := dialer.DialContext(
		ctx,
		"tcp",
		address,
	)
	if err != nil {
		return 0, fmt.Errorf(
			"não foi possível conectar ao RCON %s: %w",
			address,
			err,
		)
	}
	defer connection.Close()

	deadline := time.Now().Add(
		8 * time.Second,
	)

	if contextDeadline, exists := ctx.Deadline(); exists &&
		contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}

	if err := connection.SetDeadline(deadline); err != nil {
		return 0, fmt.Errorf(
			"não foi possível configurar o prazo RCON: %w",
			err,
		)
	}

	if err := authenticate(
		connection,
		password,
	); err != nil {
		return 0, err
	}

	output, err := executeCommand(
		connection,
		"ShowPlayers",
	)
	if err != nil {
		return 0, err
	}

	count, err := parsePlayerCount(
		output,
	)
	if err != nil {
		return 0, err
	}

	return count, nil
}

func authenticate(
	connection net.Conn,
	password string,
) error {
	packet := buildPacket(
		authPacketID,
		authPacketType,
		password,
	)

	if _, err := connection.Write(packet); err != nil {
		return fmt.Errorf(
			"não foi possível enviar a autenticação RCON: %w",
			err,
		)
	}

	for attempt := 0; attempt < 3; attempt++ {
		packetID, packetType, _, err := receivePacket(
			connection,
		)
		if err != nil {
			return fmt.Errorf(
				"não foi possível receber a autenticação RCON: %w",
				err,
			)
		}

		if packetType != authResponsePacketType {
			continue
		}

		if packetID == -1 {
			return fmt.Errorf(
				"a senha RCON foi recusada",
			)
		}

		if packetID == authPacketID {
			return nil
		}
	}

	return fmt.Errorf(
		"o servidor não confirmou a autenticação RCON",
	)
}

func executeCommand(
	connection net.Conn,
	command string,
) (string, error) {
	packet := buildPacket(
		commandPacketID,
		executeCommandPacketType,
		command,
	)

	if _, err := connection.Write(packet); err != nil {
		return "", fmt.Errorf(
			"não foi possível enviar o comando RCON: %w",
			err,
		)
	}

	for attempt := 0; attempt < 5; attempt++ {
		_, packetType, body, err := receivePacket(
			connection,
		)
		if err != nil {
			return "", fmt.Errorf(
				"não foi possível receber a resposta RCON: %w",
				err,
			)
		}

		if packetType != responseValuePacketType {
			continue
		}

		if strings.TrimSpace(body) == "" {
			continue
		}

		return body, nil
	}

	return "", fmt.Errorf(
		"o servidor RCON não retornou a resposta de ShowPlayers",
	)
}

func buildPacket(
	packetID int32,
	packetType int32,
	body string,
) []byte {
	bodyBytes := []byte(body)

	payloadSize := 4 + 4 + len(bodyBytes) + 2

	packet := make(
		[]byte,
		4+payloadSize,
	)

	binary.LittleEndian.PutUint32(
		packet[0:4],
		uint32(payloadSize),
	)

	binary.LittleEndian.PutUint32(
		packet[4:8],
		uint32(packetID),
	)

	binary.LittleEndian.PutUint32(
		packet[8:12],
		uint32(packetType),
	)

	copy(
		packet[12:],
		bodyBytes,
	)

	return packet
}

func receivePacket(
	connection net.Conn,
) (int32, int32, string, error) {
	sizeBuffer := make(
		[]byte,
		4,
	)

	if _, err := io.ReadFull(
		connection,
		sizeBuffer,
	); err != nil {
		return 0, 0, "", err
	}

	packetSize := int(
		binary.LittleEndian.Uint32(
			sizeBuffer,
		),
	)

	if packetSize < 10 ||
		packetSize > maximumPacketSize {
		return 0, 0, "", fmt.Errorf(
			"tamanho de pacote RCON inválido: %d",
			packetSize,
		)
	}

	payload := make(
		[]byte,
		packetSize,
	)

	if _, err := io.ReadFull(
		connection,
		payload,
	); err != nil {
		return 0, 0, "", err
	}

	packetID := int32(
		binary.LittleEndian.Uint32(
			payload[0:4],
		),
	)

	packetType := int32(
		binary.LittleEndian.Uint32(
			payload[4:8],
		),
	)

	body := string(
		payload[8 : len(payload)-2],
	)

	return packetID, packetType, body, nil
}

func parsePlayerCount(
	output string,
) (int, error) {
	reader := csv.NewReader(
		strings.NewReader(
			strings.TrimSpace(output),
		),
	)

	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true

	records, err := reader.ReadAll()
	if err != nil {
		return 0, fmt.Errorf(
			"não foi possível interpretar ShowPlayers: %w",
			err,
		)
	}

	if len(records) == 0 {
		return 0, fmt.Errorf(
			"ShowPlayers retornou uma resposta vazia",
		)
	}

	count := 0

	for index, record := range records {
		if index == 0 {
			continue
		}

		hasContent := false

		for _, value := range record {
			if strings.TrimSpace(value) != "" {
				hasContent = true
				break
			}
		}

		if hasContent {
			count++
		}
	}

	return count, nil
}
