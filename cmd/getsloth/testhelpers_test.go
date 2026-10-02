package main

import (
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// stringBuffer is a concurrency-safe append-only buffer - run()'s copy
// goroutine writes to stdout while the test polls it, which a plain
// bytes.Buffer doesn't support safely.
type stringBuffer struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *stringBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *stringBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

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

func skipOnWindows(t *testing.T, reason string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip(reason)
	}
}
