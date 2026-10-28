//go:build windows

package media

import (
	"testing"
	"time"
)

func TestStartAndCloseWindows(t *testing.T) {
	c, err := Start()
	if err != nil {
		t.Skipf("media hotkeys unavailable: %v", err)
	}
	if c == nil {
		t.Fatal("nil controller")
	}
	defer c.Close()
	wc, ok := c.(*windowsController)
	if !ok {
		t.Fatalf("controller type = %T", c)
	}
	var id int
	var want CommandKind
	for registeredID, kind := range wc.registered {
		id, want = registeredID, kind
		break
	}
	if id == 0 {
		t.Fatal("no registered hotkeys")
	}
	procPostThreadMessageW.Call(uintptr(wc.thread), wmHotKey, uintptr(id), 0)
	select {
	case cmd := <-c.Events():
		if cmd.Kind != want {
			t.Fatalf("command = %v, want %v", cmd.Kind, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("media key event was not delivered")
	}
}
