//go:build windows

package media

import (
	"fmt"
	"runtime"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32                 = windows.NewLazySystemDLL("user32.dll")
	procRegisterHotKey     = user32.NewProc("RegisterHotKey")
	procUnregisterHotKey   = user32.NewProc("UnregisterHotKey")
	procGetMessageW        = user32.NewProc("GetMessageW")
	procPeekMessageW       = user32.NewProc("PeekMessageW")
	procPostThreadMessageW = user32.NewProc("PostThreadMessageW")
)

const (
	wmHotKey = 0x0312
	wmQuit   = 0x0012
)

type point struct {
	X int32
	Y int32
}

type msg struct {
	Hwnd     uintptr
	Message  uint32
	WParam   uintptr
	LParam   uintptr
	Time     uint32
	Pt       point
	LPrivate uint32
}

type windowsController struct {
	events chan Command
	done   chan struct{}
	thread uint32

	registered map[int]CommandKind

	closeOnce sync.Once
}

type hotKey struct {
	id  int
	key uintptr
	cmd CommandKind
}

var hotKeys = []hotKey{
	{1, windows.VK_MEDIA_PLAY_PAUSE, PlayPause},
	{2, windows.VK_MEDIA_NEXT_TRACK, Next},
	{3, windows.VK_MEDIA_PREV_TRACK, Previous},
	{4, windows.VK_MEDIA_STOP, Stop},
}

// Start registers global Windows media hotkeys on a dedicated message-loop
// thread. Windows does not provide the system media overlay (SMTC) here; this
// only forwards the keys to the player.
func Start() (Controller, error) {
	c := &windowsController{
		events:     make(chan Command, 8),
		done:       make(chan struct{}),
		registered: make(map[int]CommandKind),
	}
	ready := make(chan error, 1)
	go c.loop(ready)
	if err := <-ready; err != nil {
		return nil, err
	}
	return c, nil
}

func (c *windowsController) Events() <-chan Command { return c.events }
func (c *windowsController) Update(State)           {}

func (c *windowsController) Close() error {
	c.closeOnce.Do(func() {
		procPostThreadMessageW.Call(uintptr(c.thread), wmQuit, 0, 0)
		<-c.done
	})
	return nil
}

func (c *windowsController) loop(ready chan<- error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(c.done)
	defer close(c.events)

	c.thread = windows.GetCurrentThreadId()
	// Force creation of the thread message queue before RegisterHotKey.
	var queueMsg msg
	procPeekMessageW.Call(uintptr(unsafe.Pointer(&queueMsg)), 0, 0, 0, 0)
	registered := make([]int, 0, len(hotKeys))
	for _, hk := range hotKeys {
		r, _, err := procRegisterHotKey.Call(0, uintptr(hk.id), 0, hk.key)
		if r == 0 {
			_ = err
			continue
		}
		registered = append(registered, hk.id)
		c.registered[hk.id] = hk.cmd
	}
	if len(registered) == 0 {
		ready <- fmt.Errorf("no media keys could be registered; another application may own them")
		return
	}
	ready <- nil
	defer func() {
		for _, id := range registered {
			procUnregisterHotKey.Call(0, uintptr(id))
		}
	}()

	for {
		var m msg
		r, _, err := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) == -1 {
			_ = err
			return
		}
		if r == 0 {
			return
		}
		if m.Message != wmHotKey {
			continue
		}
		id := int(m.WParam)
		for _, hk := range hotKeys {
			if hk.id == id {
				select {
				case c.events <- Command{Kind: hk.cmd}:
				default:
				}
				break
			}
		}
	}
}

// Available describes the platform integration for --check.
func Available() string {
	return "Windows global media hotkeys (if free)"
}
