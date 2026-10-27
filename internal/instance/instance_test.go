package instance

import (
	"errors"
	"testing"
)

func TestAcquireSecondLockFails(t *testing.T) {
	dir := t.TempDir()
	first, err := Acquire(dir)
	if err != nil {
		t.Fatalf("first Acquire: %v", err)
	}
	defer first.Release()

	second, err := Acquire(dir)
	if err == nil {
		second.Release()
		t.Fatal("second Acquire should fail while the first lock is held")
	}
	if !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("second Acquire error = %v, want ErrAlreadyRunning", err)
	}
}

func TestLockCanBeReacquiredAfterRelease(t *testing.T) {
	dir := t.TempDir()
	first, err := Acquire(dir)
	if err != nil {
		t.Fatal(err)
	}
	first.Release()

	second, err := Acquire(dir)
	if err != nil {
		t.Fatalf("Acquire after Release: %v", err)
	}
	second.Release()
}
