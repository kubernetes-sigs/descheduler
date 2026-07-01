package cycle

import (
	"errors"
	"testing"
	"time"
)

func TestManagerRejectsConcurrentStart(t *testing.T) {
	manager := NewManager()
	release := make(chan struct{})
	manager.SetRunner(func() error {
		<-release
		return nil
	})

	if err := manager.TryStart(); err != nil {
		t.Fatalf("expected first cycle to start, got %v", err)
	}
	if err := manager.TryStart(); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("expected already running error, got %v", err)
	}

	close(release)

	deadline := time.After(time.Second)
	for {
		err := manager.TryStart()
		if err == nil {
			return
		}
		if !errors.Is(err, ErrAlreadyRunning) {
			t.Fatalf("expected already running while waiting for finish, got %v", err)
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for cycle manager to release running cycle")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func TestManagerRunRejectsConcurrentTryStart(t *testing.T) {
	manager := NewManager()
	started := make(chan struct{})
	release := make(chan struct{})
	manager.SetRunner(func() error {
		close(started)
		<-release
		return nil
	})

	errCh := make(chan error, 1)
	go func() {
		errCh <- manager.Run()
	}()

	<-started
	if err := manager.TryStart(); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("expected already running error, got %v", err)
	}

	close(release)
	if err := <-errCh; err != nil {
		t.Fatalf("expected running cycle to finish successfully, got %v", err)
	}
}

func TestManagerRejectsBeforeRunnerReady(t *testing.T) {
	manager := NewManager()

	if err := manager.TryStart(); !errors.Is(err, ErrRunnerNotReady) {
		t.Fatalf("expected runner not ready error, got %v", err)
	}
}
