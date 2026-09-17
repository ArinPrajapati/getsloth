package main

import (
	"bytes"
	"os"
	"strings"
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

	code := run([]string{"echo", "hello"}, nonTerminalStdin(t), &out)

	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if got := out.String(); !strings.Contains(got, "hello") {
		t.Errorf("output = %q, want it to contain %q", got, "hello")
	}
}

func TestRun_PropagatesNonZeroExitCode(t *testing.T) {
	var out bytes.Buffer

	code := run([]string{"sh", "-c", "exit 3"}, nonTerminalStdin(t), &out)

	if code != 3 {
		t.Errorf("exit code = %d, want 3", code)
	}
}

func TestRun_NoCommandGiven(t *testing.T) {
	var out bytes.Buffer

	code := run(nil, nonTerminalStdin(t), &out)

	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
}

func TestRun_NonexistentCommand(t *testing.T) {
	var out bytes.Buffer

	code := run([]string{"getsloth-test-nonexistent-binary-xyz"}, nonTerminalStdin(t), &out)

	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
}
