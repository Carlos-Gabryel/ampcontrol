package rcon

import (
	"context"
	"encoding/binary"
	"net"
	"strings"
	"testing"
	"time"
)

type fakeServerPacket struct {
	id         int32
	packetType int32
	body       string
}

// startFakeServer aceita uma conexão e entrega os pacotes recebidos ao
// handler, que responde escrevendo diretamente na conexão.
func startFakeServer(
	t *testing.T,
	handler func(connection net.Conn, received func() fakeServerPacket),
) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	go func() {
		connection, err := listener.Accept()
		if err != nil {
			return
		}
		defer connection.Close()
		_ = connection.SetDeadline(time.Now().Add(5 * time.Second))

		handler(connection, func() fakeServerPacket {
			id, packetType, body, err := receivePacket(connection)
			if err != nil {
				return fakeServerPacket{id: -99}
			}
			return fakeServerPacket{id: id, packetType: packetType, body: body}
		})
	}()

	return listener.Addr().String()
}

func writeFakePacket(connection net.Conn, id int32, packetType int32, body string) {
	_, _ = connection.Write(buildPacket(id, packetType, body))
}

// sourceServer imita um servidor Source: um pacote vazio antes da
// confirmação da autenticação e a resposta ao comando esperado.
func sourceServer(
	t *testing.T,
	password string,
	command string,
	response string,
) func(net.Conn, func() fakeServerPacket) {
	return func(connection net.Conn, received func() fakeServerPacket) {
		auth := received()
		if auth.packetType != authPacketType {
			t.Errorf("tipo do pacote de autenticação inesperado: %d", auth.packetType)
			return
		}
		writeFakePacket(connection, auth.id, responseValuePacketType, "")
		if auth.body != password {
			writeFakePacket(connection, -1, authResponsePacketType, "")
			return
		}
		writeFakePacket(connection, auth.id, authResponsePacketType, "")

		request := received()
		if request.packetType != executeCommandPacketType || request.body != command {
			t.Errorf("comando inesperado: tipo=%d corpo=%q", request.packetType, request.body)
			return
		}
		writeFakePacket(connection, request.id, responseValuePacketType, response)
	}
}

func TestPlayerCountOverRCON(t *testing.T) {
	t.Parallel()

	address := startFakeServer(t, sourceServer(
		t,
		"segredo",
		"ShowPlayers",
		"name,playeruid,steamid\nAna,1,76561190000000001\nBia,2,76561190000000002\n",
	))

	count, err := PlayerCount(context.Background(), address, "segredo")
	if err != nil {
		t.Fatalf("PlayerCount retornou erro: %v", err)
	}
	if count != 2 {
		t.Fatalf("contagem inesperada: %d", count)
	}
}

func TestPlayerCountOverRCONWithEmptyServer(t *testing.T) {
	t.Parallel()

	address := startFakeServer(t, sourceServer(t, "segredo", "ShowPlayers", "name,playeruid,steamid\n"))

	count, err := PlayerCount(context.Background(), address, "segredo")
	if err != nil || count != 0 {
		t.Fatalf("servidor vazio deveria contar 0: contagem=%d erro=%v", count, err)
	}
}

func TestProjectZomboidPlayerCountOverRCON(t *testing.T) {
	t.Parallel()

	address := startFakeServer(t, sourceServer(t, "segredo", "players", "Players connected (1):\n-Ana\n"))

	count, err := ProjectZomboidPlayerCount(context.Background(), address, "segredo")
	if err != nil || count != 1 {
		t.Fatalf("contagem inesperada: contagem=%d erro=%v", count, err)
	}
}

func TestExecuteCommandRejectsWrongPassword(t *testing.T) {
	t.Parallel()

	address := startFakeServer(t, sourceServer(t, "segredo", "ShowPlayers", ""))

	_, err := ExecuteCommand(context.Background(), address, "errada", "ShowPlayers")
	if err == nil || !strings.Contains(err.Error(), "recusada") {
		t.Fatalf("era esperada a recusa da senha, obtido: %v", err)
	}
}

func TestExecuteCommandRejectsInvalidPacketSize(t *testing.T) {
	t.Parallel()

	address := startFakeServer(t, func(connection net.Conn, received func() fakeServerPacket) {
		received()
		size := make([]byte, 4)
		binary.LittleEndian.PutUint32(size, maximumPacketSize+1)
		_, _ = connection.Write(size)
	})

	_, err := ExecuteCommand(context.Background(), address, "segredo", "ShowPlayers")
	if err == nil || !strings.Contains(err.Error(), "tamanho de pacote") {
		t.Fatalf("era esperado erro de tamanho de pacote, obtido: %v", err)
	}
}

func TestExecuteCommandHonorsContextDeadline(t *testing.T) {
	t.Parallel()

	address := startFakeServer(t, func(connection net.Conn, received func() fakeServerPacket) {
		received()
		time.Sleep(2 * time.Second)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	started := time.Now()
	_, err := ExecuteCommand(ctx, address, "segredo", "ShowPlayers")
	if err == nil {
		t.Fatal("era esperado erro com o servidor sem resposta")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("o prazo do contexto não foi respeitado: %s", elapsed)
	}
}

func TestExecuteCommandRejectsEmptyCommand(t *testing.T) {
	t.Parallel()

	if _, err := ExecuteCommand(context.Background(), "127.0.0.1:1", "segredo", "  "); err == nil {
		t.Fatal("era esperado erro para comando vazio")
	}
}

func TestBuildPacketLayout(t *testing.T) {
	t.Parallel()

	packet := buildPacket(authPacketID, authPacketType, "abc")

	if size := binary.LittleEndian.Uint32(packet[0:4]); size != uint32(len(packet)-4) || size != 4+4+3+2 {
		t.Fatalf("tamanho declarado inesperado: %d (pacote com %d bytes)", size, len(packet))
	}
	if id := int32(binary.LittleEndian.Uint32(packet[4:8])); id != authPacketID {
		t.Fatalf("ID inesperado: %d", id)
	}
	if packetType := int32(binary.LittleEndian.Uint32(packet[8:12])); packetType != authPacketType {
		t.Fatalf("tipo inesperado: %d", packetType)
	}
	if body := string(packet[12:15]); body != "abc" {
		t.Fatalf("corpo inesperado: %q", body)
	}
	if packet[15] != 0 || packet[16] != 0 {
		t.Fatalf("o pacote deveria terminar com dois bytes nulos: %v", packet[15:])
	}
}

func TestParsePlayerCountIgnoresBlankRowsAndRejectsEmptyOutput(t *testing.T) {
	t.Parallel()

	count, err := parsePlayerCount("name,playeruid,steamid\nAna,1,2\n,,\n")
	if err != nil || count != 1 {
		t.Fatalf("contagem inesperada: contagem=%d erro=%v", count, err)
	}
	if _, err := parsePlayerCount("   "); err == nil {
		t.Fatal("era esperado erro para resposta vazia")
	}
}
