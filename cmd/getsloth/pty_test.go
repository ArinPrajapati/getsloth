package main

import (
	"bytes"
	"os"
	"strings"
	"sync/atomic"
	"testing"
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

	code := run([]string{"echo", "hello"}, nonTerminalStdin(t), &out, nil, nil)

	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if got := out.String(); !strings.Contains(got, "hello") {
		t.Errorf("output = %q, want it to contain %q", got, "hello")
	}
}

func TestRun_PropagatesNonZeroExitCode(t *testing.T) {
	var out bytes.Buffer

	code := run([]string{"sh", "-c", "exit 3"}, nonTerminalStdin(t), &out, nil, nil)

	if code != 3 {
		t.Errorf("exit code = %d, want 3", code)
	}
}

func TestRun_NoCommandGiven(t *testing.T) {
	var out bytes.Buffer

	code := run(nil, nonTerminalStdin(t), &out, nil, nil)

	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
}

func TestRun_NonexistentCommand(t *testing.T) {
	var out bytes.Buffer

	code := run([]string{"getsloth-test-nonexistent-binary-xyz"}, nonTerminalStdin(t), &out, nil, nil)

	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
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

func TestRun_OnPTYReadyCalledWithMasterFile(t *testing.T) {
	var out bytes.Buffer
	var got *os.File

	code := run([]string{"echo", "hi"}, nonTerminalStdin(t), &out, nil, func(f *os.File) { got = f })

	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got == nil {
		t.Error("onPTYReady was never called")
	}
}
