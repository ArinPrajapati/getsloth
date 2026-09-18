package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"sync/atomic"
	"syscall"

	"github.com/arinprajapati/getsloth/internal/protocol"
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
//
// isActiveWriter gates whether the host's own local keystrokes actually
// reach the PTY - per docs/protocol.md's Control model, the host's input
// is subject to the exact same active-writer rule a viewer's is, just
// applied locally instead of by the relay (see "Host's own input never
// touches the network"). A nil isActiveWriter means always active -
// used both by tests and by the no-relay-connection fallback, where
// there's no shared control concept to gate against.
//
// onPTYReady, if non-nil, is called once with the PTY master file right
// after it's created, before the copy loops start - this is how the
// caller gets a handle to write forwarded viewer input into the PTY
// without pty.go needing to know anything about networking.
//
// panicKill, if non-nil, is the local panic-kill trigger: when it's
// closed, the wrapped command and every process it spawned during the
// session are terminated immediately with SIGKILL - the whole process
// group, not just the top-level command, since a background process a
// malicious actor started would otherwise survive killing just the
// shell. The wrapped command is spawned in its own process group
// (Setpgid) specifically to make this possible.
func run(args []string, stdin *os.File, stdout io.Writer, isActiveWriter *atomic.Bool, onPTYReady func(*os.File), onHostSize func(cols, rows int), panicKill <-chan struct{}) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "getsloth: no command given")
		return 2
	}

	cmd := exec.Command(args[0], args[1:]...)
	// No explicit Setpgid here: pty.Start already calls setsid()
	// internally (needed to assign the PTY as the controlling
	// terminal), which makes the child both a session leader and its
	// own process group leader in one step - cmd.Process.Pid already
	// equals that process group's ID. Additionally setting Setpgid
	// would try to setpgid() a session leader on itself, which POSIX
	// disallows (EPERM) - confirmed by this exact failure when it was
	// tried.
	cols, rows := terminalGridSize(stdin)
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	if err != nil {
		fmt.Fprintln(os.Stderr, "getsloth:", err)
		return 1
	}
	defer func() { _ = ptmx.Close() }()

	if panicKill != nil {
		stopPanicWatch := make(chan struct{})
		defer close(stopPanicWatch)
		go func() {
			select {
			case <-panicKill:
				killProcessGroup(cmd.Process.Pid)
			case <-stopPanicWatch:
			}
		}()
	}

	if onPTYReady != nil {
		onPTYReady(ptmx)
	}

	if term.IsTerminal(int(stdin.Fd())) {
		stopResize := watchResize(stdin, ptmx, isActiveWriter, onHostSize)
		defer stopResize()

		if oldState, err := term.MakeRaw(int(stdin.Fd())); err == nil {
			defer func() { _ = term.Restore(int(stdin.Fd()), oldState) }()
		}
	}

	go func() {
		dst := io.Writer(ptmx)
		if isActiveWriter != nil {
			dst = &gatedWriter{dst: ptmx, active: isActiveWriter}
		}
		_, _ = io.Copy(dst, stdin)
	}()
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

// killProcessGroup sends SIGKILL to the entire process group led by
// pid - not just pid itself. Because the wrapped command is spawned
// with Setpgid (see run), its process group ID equals its own PID, and
// syscall.Kill with a negative PID targets the whole group: the wrapped
// command and every child, grandchild, or background job it spawned
// during the session, in one signal.
func killProcessGroup(pid int) {
	_ = syscall.Kill(-pid, syscall.SIGKILL)
}

// gatedWriter drops writes instead of forwarding them to dst whenever
// active reports false - this is how the host's own local keystrokes
// stop reaching the PTY the instant a viewer becomes the active writer,
// mirroring the relay silently dropping a non-active-writer viewer's
// input rather than erroring.
type gatedWriter struct {
	dst    io.Writer
	active *atomic.Bool
}

func (g *gatedWriter) Write(p []byte) (int, error) {
	if !g.active.Load() {
		return len(p), nil
	}
	return g.dst.Write(p)
}

// watchResize keeps ptmx's window size matching stdin's terminal size,
// setting it once immediately and again on every SIGWINCH. The returned
// func stops watching and must be called to avoid leaking the goroutine.
func watchResize(stdin *os.File, ptmx *os.File, isActiveWriter *atomic.Bool, onHostSize func(cols, rows int)) func() {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGWINCH)
	sig <- syscall.SIGWINCH // trigger an initial resize
	done := make(chan struct{})

	go func() {
		for {
			select {
			case <-sig:
				cols, rows := terminalGridSize(stdin)
				if onHostSize != nil {
					onHostSize(cols, rows)
				}
				if isActiveWriter == nil || isActiveWriter.Load() {
					_ = pty.Setsize(ptmx, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
				}
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

func terminalGridSize(input *os.File) (int, int) {
	size, err := pty.GetsizeFull(input)
	if err != nil || !validGridSize(int(size.Cols), int(size.Rows)) {
		return protocol.DefaultCols, protocol.DefaultRows
	}
	return int(size.Cols), int(size.Rows)
}

func validGridSize(cols, rows int) bool {
	return cols >= 2 && rows >= 2 && cols <= 1000 && rows <= 500
}
