package main

import (
	"errors"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/danielxfeng/async_gateway/core"
)

type mockService struct {
	name        string
	runFunc     func() error
	cleanCalled bool
}

func (m *mockService) Name() string { return m.name }
func (m *mockService) Run() error   { return m.runFunc() }
func (m *mockService) Clean()       { m.cleanCalled = true }

func TestRun_Normal_Case(t *testing.T) {
	attempt := 0
	s := &mockService{
		name: "task",
		runFunc: func() error {
			attempt++
			time.Sleep(100 * time.Millisecond)
			return nil
		},
	}

	run([]core.Service{s}, 2, 50*time.Millisecond)

	if attempt != 1 {
		t.Errorf("Run() should be called exactly once, but was called %d times", attempt)
	}
	if s.cleanCalled {
		t.Errorf("Clean() should not be called on normal exit")
	}
}

func TestRun_ErrorThenSuccess(t *testing.T) {
	attempt := 0
	s := &mockService{
		name: "error-then-success",
		runFunc: func() error {
			attempt++
			if attempt == 1 {
				return errors.New("first fail")
			}
			return nil
		},
	}

	run([]core.Service{s}, 3, 50*time.Millisecond)

	if attempt != 2 {
		t.Errorf("Run() should be called twice, but was called %d times", attempt)
	}
	if s.cleanCalled {
		t.Errorf("Clean() should not be called when recovered after retry")
	}
}

func TestRun_ErrorRetriesExhausted(t *testing.T) {
	attempt := 0
	s := &mockService{
		name: "error-exhausted",
		runFunc: func() error {
			attempt++
			return errors.New("always fail")
		},
	}

	run([]core.Service{s}, 2, 50*time.Millisecond)

	if attempt != 2 {
		t.Errorf("Run() should be called twice, but was called %d times", attempt)
	}
	if s.cleanCalled {
		t.Errorf("Clean() should not be called when retries exhausted")
	}
}

func TestRun_Panic(t *testing.T) {
	attempt := 0
	s := &mockService{
		name: "panic",
		runFunc: func() error {
			attempt++
			panic("panic")
		},
	}

	run([]core.Service{s}, 2, 50*time.Millisecond)

	if attempt != 2 {
		t.Errorf("Run() should be called twice on panic, but was called %d times", attempt)
	}
	if s.cleanCalled {
		t.Errorf("Clean() should not be called on panic")
	}
}

func TestRun_PanicThenSuccess_NoClean(t *testing.T) {
	attempt := 0
	s := &mockService{
		name: "panic-then-success",
		runFunc: func() error {
			attempt++
			if attempt == 1 {
				panic("panic")
			}
			return nil
		},
	}

	run([]core.Service{s}, 2, 50*time.Millisecond)

	if attempt != 2 {
		t.Errorf("Run() should be called twice (panic then success), but was called %d times", attempt)
	}
	if s.cleanCalled {
		t.Errorf("Clean() should not be called when recovered after panic")
	}
}

func TestRun_SignalExit(t *testing.T) {
	attempt := 0
	s := &mockService{
		name: "signal",
		runFunc: func() error {
			attempt++
			time.Sleep(200 * time.Millisecond)
			return nil
		},
	}

	done := make(chan struct{})
	go func() {
		run([]core.Service{s}, 2, 50*time.Millisecond)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	p, _ := os.FindProcess(os.Getpid())
	_ = p.Signal(syscall.SIGINT)

	select {
	case <-done:
		if !s.cleanCalled {
			t.Errorf("Clean() should be called on signal exit")
		}
	case <-time.After(1 * time.Second):
		t.Errorf("run() did not exit in time")
	}
}

func TestRun_MixedServices_ErrorAndNormal(t *testing.T) {
	// Service A: fail once then success
	attemptA := 0
	sA := &mockService{
		name: "mixed-A",
		runFunc: func() error {
			attemptA++
			if attemptA == 1 {
				return errors.New("fail once")
			}
			return nil
		},
	}

	// Service B: normal exit
	attemptB := 0
	sB := &mockService{
		name: "mixed-B",
		runFunc: func() error {
			attemptB++
			return nil
		},
	}

	run([]core.Service{sA, sB}, 2, 50*time.Millisecond)

	if attemptA != 2 {
		t.Errorf("Service A should be called twice, but was called %d times", attemptA)
	}
	if attemptB != 1 {
		t.Errorf("Service B should be called once, but was called %d times", attemptB)
	}
	if sA.cleanCalled || sB.cleanCalled {
		t.Errorf("Clean() should not be called for mixed services scenario")
	}
}

func TestRun_MultipleServices_SignalExit(t *testing.T) {
	sA := &mockService{
		name: "multi-A",
		runFunc: func() error {
			time.Sleep(200 * time.Millisecond)
			return nil
		},
	}
	sB := &mockService{
		name: "multi-B",
		runFunc: func() error {
			time.Sleep(200 * time.Millisecond)
			return nil
		},
	}

	done := make(chan struct{})
	go func() {
		run([]core.Service{sA, sB}, 2, 50*time.Millisecond)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	p, _ := os.FindProcess(os.Getpid())
	_ = p.Signal(syscall.SIGINT)

	select {
	case <-done:
		if !sA.cleanCalled || !sB.cleanCalled {
			t.Errorf("Clean() should be called on both services during signal exit")
		}
	case <-time.After(1 * time.Second):
		t.Errorf("run() did not exit in time")
	}
}
