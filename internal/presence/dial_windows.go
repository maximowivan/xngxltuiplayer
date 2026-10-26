//go:build windows

package presence

import (
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

type pipeAddr struct{}

func (pipeAddr) Network() string { return "pipe" }
func (pipeAddr) String() string  { return "discord-ipc" }

// pipeConn uses overlapped I/O. Discord keeps a read pending at all times, so
// a synchronous ReadFile would block every WriteFile on the same handle and
// the activity would never reach Discord.
type pipeConn struct {
	handle     windows.Handle
	readEvent  windows.Handle
	writeEvent windows.Handle

	readMu  sync.Mutex
	writeMu sync.Mutex

	closeOnce sync.Once
	closeErr  error
}

func newPipeConn(handle windows.Handle) (*pipeConn, error) {
	readEvent, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		return nil, err
	}
	writeEvent, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		_ = windows.CloseHandle(readEvent)
		return nil, err
	}
	return &pipeConn{handle: handle, readEvent: readEvent, writeEvent: writeEvent}, nil
}

func (p *pipeConn) Read(b []byte) (int, error) {
	if len(b) == 0 {
		return 0, nil
	}
	p.readMu.Lock()
	defer p.readMu.Unlock()
	if p.handle == 0 {
		return 0, net.ErrClosed
	}
	_ = windows.ResetEvent(p.readEvent)
	var ov windows.Overlapped
	ov.HEvent = p.readEvent
	var n uint32
	err := windows.ReadFile(p.handle, b, &n, &ov)
	if errors.Is(err, windows.ERROR_IO_PENDING) {
		if _, werr := windows.WaitForSingleObject(p.readEvent, windows.INFINITE); werr != nil {
			return 0, werr
		}
		err = windows.GetOverlappedResult(p.handle, &ov, &n, false)
	}
	if err != nil {
		return int(n), err
	}
	if n == 0 {
		return 0, io.EOF
	}
	return int(n), nil
}

func (p *pipeConn) Write(b []byte) (int, error) {
	if len(b) == 0 {
		return 0, nil
	}
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	if p.handle == 0 {
		return 0, net.ErrClosed
	}
	_ = windows.ResetEvent(p.writeEvent)
	var ov windows.Overlapped
	ov.HEvent = p.writeEvent
	var n uint32
	err := windows.WriteFile(p.handle, b, &n, &ov)
	if errors.Is(err, windows.ERROR_IO_PENDING) {
		if _, werr := windows.WaitForSingleObject(p.writeEvent, windows.INFINITE); werr != nil {
			return 0, werr
		}
		err = windows.GetOverlappedResult(p.handle, &ov, &n, false)
	}
	if err != nil {
		return int(n), err
	}
	if n == 0 && len(b) > 0 {
		return 0, io.ErrShortWrite
	}
	return int(n), nil
}

func (p *pipeConn) Close() error {
	p.closeOnce.Do(func() {
		if p.handle != 0 {
			_ = windows.CancelIoEx(p.handle, nil)
		}
		// CancelIoEx wakes both pending operations. Wait for them to release
		// their events before closing the handles.
		p.readMu.Lock()
		p.writeMu.Lock()
		if p.handle != 0 {
			p.closeErr = windows.CloseHandle(p.handle)
			p.handle = 0
		}
		if p.readEvent != 0 {
			_ = windows.CloseHandle(p.readEvent)
			p.readEvent = 0
		}
		if p.writeEvent != 0 {
			_ = windows.CloseHandle(p.writeEvent)
			p.writeEvent = 0
		}
		p.writeMu.Unlock()
		p.readMu.Unlock()
	})
	return p.closeErr
}

func (p *pipeConn) LocalAddr() net.Addr              { return pipeAddr{} }
func (p *pipeConn) RemoteAddr() net.Addr             { return pipeAddr{} }
func (p *pipeConn) SetDeadline(time.Time) error      { return nil }
func (p *pipeConn) SetReadDeadline(time.Time) error  { return nil }
func (p *pipeConn) SetWriteDeadline(time.Time) error { return nil }

func dialDiscordIPC() (net.Conn, error) {
	var lastErr, accessErr error
	prefixes := []string{`\\.\pipe\`, `\\?\pipe\`}
	for _, prefix := range prefixes {
		for i := 0; i < 10; i++ {
			name := fmt.Sprintf("%sdiscord-ipc-%d", prefix, i)
			path, err := syscall.UTF16PtrFromString(name)
			if err != nil {
				return nil, err
			}
			handle, err := syscall.CreateFile(
				path,
				syscall.GENERIC_READ|syscall.GENERIC_WRITE,
				0,
				nil,
				syscall.OPEN_EXISTING,
				syscall.FILE_FLAG_OVERLAPPED,
				0,
			)
			if err == nil {
				conn, cerr := newPipeConn(windows.Handle(handle))
				if cerr != nil {
					_ = syscall.CloseHandle(handle)
					return nil, cerr
				}
				return conn, nil
			}
			wrapped := fmt.Errorf("open %s: %w", name, err)
			if errors.Is(err, syscall.ERROR_ACCESS_DENIED) && accessErr == nil {
				accessErr = wrapped
			}
			lastErr = wrapped
		}
	}
	// Access denied is the actionable error: the pipe exists, but Discord runs
	// with higher privileges than the player.
	if accessErr != nil {
		return nil, accessErr
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("discord IPC pipe not found")
	}
	return nil, lastErr
}
