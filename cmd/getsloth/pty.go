package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/creack/pty"
	"golang.org/x/term"
)

// run spawns args in a pseudo-terminal, copying stdin to it and its
// output to stdout, and returns the wrapped process's exit code.
//
// When stdin is a real interactive terminal, run also puts it into raw
// mode for the duration - so keystrokes like arrow keys and Ctrl+C pass
// through to the wrapped program unmodified instead of being
// line-buffered or interpreted by this process - and keeps the PTY's
// window size in sync via SIGWINCH. Tests pass a pipe as stdin, which is
// never a terminal, so that path is skipped entirely rather than faked.
func run(args []string, stdin *os.File, stdout io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "getsloth: no command given")
		return 2
	}

	cmd := exec.Command(args[0], args[1:]...)
	ptmx, err := pty.Start(cmd)
	if err != nil {
		fmt.Fprintln(os.Stderr, "getsloth:", err)
		return 1
	}
	defer func() { _ = ptmx.Close() }()

	if term.IsTerminal(int(stdin.Fd())) {
		stopResize := watchResize(stdin, ptmx)
		defer stopResize()

		if oldState, err := term.MakeRaw(int(stdin.Fd())); err == nil {
			defer func() { _ = term.Restore(int(stdin.Fd()), oldState) }()
		}
	}

	go func() { _, _ = io.Copy(ptmx, stdin) }()
	// A read error here is expected once the child exits and the PTY's
	// slave side closes (creack/pty's documented behavior) - not a real
	// failure, so it's intentionally not surfaced.
	_, _ = io.Copy(stdout, ptmx)

	if err := cmd.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		fmt.Fprintln(os.Stderr, "getsloth:", err)
		return 1
	}
	return 0
}

// watchResize keeps ptmx's window size matching stdin's terminal size,
// setting it once immediately and again on every SIGWINCH. The returned
// func stops watching and must be called to avoid leaking the goroutine.
func watchResize(stdin *os.File, ptmx *os.File) func() {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGWINCH)
	sig <- syscall.SIGWINCH // trigger an initial resize
	done := make(chan struct{})

	go func() {
		for {
			select {
			case <-sig:
				_ = pty.InheritSize(stdin, ptmx)
			case <-done:
				return
			}
		}
	}()

	return func() {
		signal.Stop(sig)
		close(done)
	}
}
