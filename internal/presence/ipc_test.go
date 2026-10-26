package presence

import (
	"encoding/json"
	"fmt"
	"net"
	"testing"
)

func TestIPCHandshake(t *testing.T) {
	clientSide, serverSide := net.Pipe()
	defer clientSide.Close()
	defer serverSide.Close()

	server := &ipcConn{conn: serverSide}
	handshakeSeen := make(chan map[string]any, 1)
	pongSeen := make(chan map[string]any, 1)
	serverErr := make(chan error, 1)
	go func() {
		op, payload, err := server.read()
		if err != nil {
			serverErr <- err
			return
		}
		if op != opHandshake {
			serverErr <- fmt.Errorf("unexpected opcode %d", op)
			return
		}
		var hs map[string]any
		if err := json.Unmarshal(payload, &hs); err != nil {
			serverErr <- err
			return
		}
		handshakeSeen <- hs

		if err := server.write(opPing, map[string]any{"ping": true}); err != nil {
			serverErr <- err
			return
		}
		op, payload, err = server.read()
		if err != nil {
			serverErr <- err
			return
		}
		if op != opPong {
			serverErr <- fmt.Errorf("expected PONG, got opcode %d", op)
			return
		}
		var pong map[string]any
		if err := json.Unmarshal(payload, &pong); err != nil {
			serverErr <- err
			return
		}
		pongSeen <- pong

		if err := server.write(opFrame, map[string]any{
			"cmd": "DISPATCH", "evt": "READY", "data": map[string]any{"v": 1},
		}); err != nil {
			serverErr <- err
			return
		}
		serverErr <- nil
	}()

	client := &ipcConn{conn: clientSide}
	if err := client.handshake("1234567890"); err != nil {
		t.Fatal(err)
	}
	if hs := <-handshakeSeen; hs["client_id"] != "1234567890" {
		t.Fatalf("handshake = %+v", hs)
	}
	if pong := <-pongSeen; pong["ping"] != true {
		t.Fatalf("pong = %+v", pong)
	}
	if err := <-serverErr; err != nil {
		t.Fatal(err)
	}
}
