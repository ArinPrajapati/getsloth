package main

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// nonTerminalStdin returns an *os.File that is definitely not a terminal
// (a pipe), with its write end already closed so any read on it returns
// EOF immediately instead of blocking - exercising the same code path
// run() takes when getsloth isn't given an interactive terminal to
// forward, without leaking a goroutine blocked on an empty pipe.
func nonTerminalStdin(t *testing.T) *os.File {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("closing pipe writer: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return r
}

func TestRun_PrintsOutputAndExitsZero(t *testing.T) {
	var out bytes.Buffer

	code := run([]string{"echo", "hello"}, nonTerminalStdin(t), &out, nil, nil, nil, nil, nil)

	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if got := out.String(); !strings.Contains(got, "hello") {
		t.Errorf("output = %q, want it to contain %q", got, "hello")
	}
}

func TestRun_PropagatesNonZeroExitCode(t *testing.T) {
	var out bytes.Buffer

	code := run([]string{"sh", "-c", "exit 3"}, nonTerminalStdin(t), &out, nil, nil, nil, nil, nil)

	if code != 3 {
		t.Errorf("exit code = %d, want 3", code)
	}
}

func TestRun_NoCommandGiven(t *testing.T) {
	var out bytes.Buffer

	code := run(nil, nonTerminalStdin(t), &out, nil, nil, nil, nil, nil)

	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
}

func TestRun_NonexistentCommand(t *testing.T) {
	var out bytes.Buffer

	code := run([]string{"getsloth-test-nonexistent-binary-xyz"}, nonTerminalStdin(t), &out, nil, nil, nil, nil, nil)

	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
}

// TestRun_PanicKill_TerminatesEntireProcessGroup is the actual point of
// panic kill: not just stopping the top-level wrapped process, but
// every process it spawned during the session - a malicious actor with
// control could have started a background process that would otherwise
// survive a simple "stop the shell" kill.
func TestRun_PanicKill_TerminatesEntireProcessGroup(t *testing.T) {
	var out stringBuffer
	panicKill := make(chan struct{})

	done := make(chan int, 1)
	go func() {
		// Spawns a background child (sleep) and prints its PID, then
		// waits on it - if panicKill only reached the shell itself and
		// not its child, "sleep 100" would keep running independently.
		done <- run([]string{"sh", "-c", "sleep 100 & echo $!; wait"}, nonTerminalStdin(t), &out, nil, nil, nil, nil, panicKill)
	}()

	var childPID int
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if n, err := fmt.Sscanf(strings.TrimSpace(out.String()), "%d", &childPID); err == nil && n == 1 && childPID > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if childPID == 0 {
		t.Fatal("did not observe the background child's PID in time")
	}

	close(panicKill)

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("run() did not return after panicKill was triggered")
	}

	// Signal 0 checks liveness without actually sending a signal -
	// ESRCH means the process is gone.
	if err := syscall.Kill(childPID, 0); err == nil {
		t.Errorf("background child pid %d is still alive after panicKill - the kill did not reach the whole process group", childPID)
	}
}

func TestGatedWriter_DropsWritesWhenInactive(t *testing.T) {
	var out bytes.Buffer
	var active atomic.Bool
	active.Store(false)

	w := &gatedWriter{dst: &out, active: &active}
	n, err := w.Write([]byte("dropped"))

	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != len("dropped") {
		t.Errorf("n = %d, want %d (a drop still reports the full length written, not an error)", n, len("dropped"))
	}
	if out.Len() != 0 {
		t.Errorf("dst received %q while inactive, want nothing written through", out.String())
	}
}

func TestGatedWriter_ForwardsWritesWhenActive(t *testing.T) {
	var out bytes.Buffer
	var active atomic.Bool
	active.Store(true)

	w := &gatedWriter{dst: &out, active: &active}
	if _, err := w.Write([]byte("forwarded")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	if out.String() != "forwarded" {
		t.Errorf("dst = %q, want %q", out.String(), "forwarded")
	}
}

func TestGatedWriter_ReclaimsControlWhileHostInputIsInactive(t *testing.T) {
	var out bytes.Buffer
	var active atomic.Bool
	active.Store(false)
	reclaims := 0

	w := &gatedWriter{
		dst:    &out,
		active: &active,
		actions: &hostInputActions{
			onReclaim: func() { reclaims++ },
		},
	}
	input := []byte{'x', hostCommandPrefix, 'r', 'y'}
	n, err := w.Write(input)

	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != len(input) {
		t.Fatalf("n = %d, want %d", n, len(input))
	}
	if reclaims != 1 {
		t.Fatalf("reclaims = %d, want 1", reclaims)
	}
	if out.Len() != 0 {
		t.Fatalf("inactive PTY received %q", out.String())
	}
}

func TestGatedWriter_ShowsStatusWithoutForwardingCommandBytes(t *testing.T) {
	var out bytes.Buffer
	var active atomic.Bool
	active.Store(true)
	statusRequests := 0

	w := &gatedWriter{
		dst:    &out,
		active: &active,
		actions: &hostInputActions{
			onStatus: func() { statusRequests++ },
		},
	}
	_, err := w.Write([]byte{hostCommandPrefix, 'i'})

	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if statusRequests != 1 {
		t.Fatalf("status requests = %d, want 1", statusRequests)
	}
	if out.Len() != 0 {
		t.Fatalf("PTY received host command bytes %v", out.Bytes())
	}
}

func TestGatedWriter_PreservesUnknownPrefixSequenceWhenActive(t *testing.T) {
	var out bytes.Buffer
	var active atomic.Bool
	active.Store(true)

	w := &gatedWriter{
		dst:     &out,
		active:  &active,
		actions: &hostInputActions{},
	}
	input := []byte{hostCommandPrefix, 'x'}
	if _, err := w.Write(input); err != nil {
		t.Fatalf("Write: %v", err)
	}

	if !bytes.Equal(out.Bytes(), input) {
		t.Fatalf("PTY received %v, want original sequence %v", out.Bytes(), input)
	}
}

func TestRun_OnPTYReadyCalledWithMasterFile(t *testing.T) {
	var out bytes.Buffer
	var got *os.File

	code := run([]string{"echo", "hi"}, nonTerminalStdin(t), &out, nil, func(f *os.File) { got = f }, nil, nil, nil)

	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got == nil {
		t.Error("onPTYReady was never called")
	}
}
