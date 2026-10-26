package presence

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

const (
	opHandshake = 0
	opFrame     = 1
	opClose     = 2
	opPing      = 3
	opPong      = 4

	maxFrame = 1 << 20
)

type rpcFrame struct {
	Cmd   string      `json:"cmd,omitempty"`
	Args  interface{} `json:"args,omitempty"`
	Nonce string      `json:"nonce,omitempty"`
}

type ipcConn struct {
	conn net.Conn
	mu   sync.Mutex
}

func (c *ipcConn) write(op int, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return c.writeRaw(op, payload)
}

func (c *ipcConn) writeRaw(op int, payload []byte) error {
	header := make([]byte, 8)
	binary.LittleEndian.PutUint32(header[0:4], uint32(op))
	binary.LittleEndian.PutUint32(header[4:8], uint32(len(payload)))

	c.mu.Lock()
	defer c.mu.Unlock()
	if err := writeFull(c.conn, header); err != nil {
		return err
	}
	return writeFull(c.conn, payload)
}

func writeFull(w io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := w.Write(data)
		if err != nil {
			return err
		}
		if n <= 0 {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}

func (c *ipcConn) read() (op int, payload []byte, err error) {
	header := make([]byte, 8)
	if _, err := io.ReadFull(c.conn, header); err != nil {
		return 0, nil, err
	}
	op = int(binary.LittleEndian.Uint32(header[0:4]))
	length := int(binary.LittleEndian.Uint32(header[4:8]))
	if length < 0 || length > maxFrame {
		return 0, nil, fmt.Errorf("discord IPC frame is too large: %d", length)
	}
	payload = make([]byte, length)
	if _, err := io.ReadFull(c.conn, payload); err != nil {
		return 0, nil, err
	}
	return op, payload, nil
}

func (c *ipcConn) handshake(clientID string) error {
	if err := c.write(opHandshake, map[string]any{"v": 1, "client_id": clientID}); err != nil {
		return err
	}

	var timedOut atomic.Bool
	timer := time.AfterFunc(6*time.Second, func() {
		timedOut.Store(true)
		_ = c.conn.Close()
	})
	defer timer.Stop()

	for i := 0; i < 20; i++ {
		op, payload, err := c.read()
		if err != nil {
			if timedOut.Load() {
				return errors.New("timed out waiting for Discord; is Discord responding?")
			}
			return err
		}
		if op == opPing {
			_ = c.writeRaw(opPong, payload)
			continue
		}
		if op == opClose {
			return errors.New("Discord closed the connection during handshake")
		}
		var msg struct {
			Evt  string          `json:"evt"`
			Data json.RawMessage `json:"data"`
		}
		_ = json.Unmarshal(payload, &msg)
		switch msg.Evt {
		case "READY":
			return nil
		case "ERROR":
			return fmt.Errorf("Discord rejected the handshake: %s", string(msg.Data))
		}
	}
	return errors.New("discord did not send READY")
}

func (c *ipcConn) readLoop(disconnected chan struct{}, onFrame func([]byte)) {
	defer close(disconnected)
	for {
		op, payload, err := c.read()
		if err != nil {
			return
		}
		switch op {
		case opPing:
			_ = c.writeRaw(opPong, payload)
		case opClose:
			return
		case opFrame:
			if onFrame != nil {
				onFrame(payload)
			}
		}
	}
}

func (c *ipcConn) close() {
	_ = c.conn.Close()
}

func connectIPC(clientID string) (*ipcConn, error) {
	conn, err := dialDiscordIPC()
	if err != nil {
		return nil, err
	}
	ipc := &ipcConn{conn: conn}
	if err := ipc.handshake(clientID); err != nil {
		ipc.close()
		return nil, err
	}
	return ipc, nil
}

func activityNonce() string {
	var buf [16]byte
	_, _ = rand.Read(buf[:])
	buf[6] = (buf[6] & 0x0f) | 0x40
	buf[8] = (buf[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", buf[0:4], buf[4:6], buf[6:8], buf[8:10], buf[10:])
}
