//go:build windows

package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"
)

func comspec(t *testing.T) string {
	t.Helper()
	shell := os.Getenv("COMSPEC")
	if shell == "" {
		t.Fatal("COMSPEC is not set")
	}
	return shell
}

func TestRunWindows_ForwardsOutputAndExitCode(t *testing.T) {
	var out bytes.Buffer
	code := run([]string{comspec(t), "/c", "echo hello-conpty & exit 3"}, nonTerminalStdin(t), &out, nil, nil, nil, nil, nil)

	if code != 3 {
		t.Errorf("exit code = %d, want 3", code)
	}
	if !strings.Contains(out.String(), "hello-conpty") {
		t.Errorf("output %q missing hello-conpty", out.String())
	}
}

func TestRunWindows_MissingCommandFails(t *testing.T) {
	var out bytes.Buffer
	if code := run([]string{"getsloth-test-nonexistent-binary-xyz"}, nonTerminalStdin(t), &out, nil, nil, nil, nil, nil); code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
}

func TestRunWindows_ArgumentsWithSpacesAndQuotesSurvive(t *testing.T) {
	var out bytes.Buffer
	run([]string{comspec(t), "/c", "echo", `"two words"`}, nonTerminalStdin(t), &out, nil, nil, nil, nil, nil)

	if !strings.Contains(out.String(), "two words") {
		t.Errorf("output %q missing quoted argument", out.String())
	}
}

func TestRunWindows_ResizeReachesConsole(t *testing.T) {
	var out bytes.Buffer
	ready := make(chan ptyConn, 1)
	done := make(chan int, 1)
	go func() {
		done <- run([]string{comspec(t), "/c", "ping -n 3 127.0.0.1 >nul & mode con"}, nonTerminalStdin(t), &out, nil, func(c ptyConn) { ready <- c }, nil, nil, nil)
	}()

	conn := <-ready
	if err := conn.resize(101, 33); err != nil {
		t.Fatalf("resize: %v", err)
	}
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("run did not return")
	}
	if !strings.Contains(out.String(), "101") || !strings.Contains(out.String(), "33") {
		t.Errorf("mode con output %q does not report the resized grid", out.String())
	}
}

func TestRunWindows_PanicKillEndsDescendants(t *testing.T) {
	panicKill := make(chan struct{})
	var out bytes.Buffer
	done := make(chan int, 1)
	go func() {
		done <- run([]string{comspec(t), "/c", "start /b ping -n 200 127.0.0.1 >nul & ping -n 200 127.0.0.1 >nul"}, nonTerminalStdin(t), &out, nil, nil, nil, nil, panicKill)
	}()

	time.Sleep(2 * time.Second)
	close(panicKill)
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("run did not return after panic kill")
	}
}
