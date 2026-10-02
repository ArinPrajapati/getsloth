package main

import (
	"fmt"
	"io"
	"os"
	"sync/atomic"

	"github.com/arinprajapati/getsloth/internal/protocol"
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
// onPTYReady, if non-nil, is called once with the PTY connection right
// after it's created, before the copy loops start - this is how the
// caller gets a handle to write forwarded viewer input into the PTY
// without pty.go needing to know anything about networking.
//
// panicKill, if non-nil, is the local panic-kill trigger: when it's
// closed, the wrapped command and every process it spawned during the
// session are terminated immediately - the whole process tree, not just the
// top-level command, since a background process a
// malicious actor started would otherwise survive killing just the
// shell. How the tree is owned is platform-specific (see startPTY).
func run(args []string, stdin *os.File, stdout io.Writer, isActiveWriter *atomic.Bool, onPTYReady func(ptyConn), onHostSize func(cols, rows int), inputActions *hostInputActions, panicKill <-chan struct{}) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "getsloth: no command given")
		return 2
	}

	cols, rows := terminalGridSize(stdin)
	proc, err := startPTY(args, cols, rows)
	if err != nil {
		fmt.Fprintln(os.Stderr, "getsloth:", err)
		return 1
	}
	defer func() { _ = proc.Close() }()

	if panicKill != nil {
		stopPanicWatch := make(chan struct{})
		defer close(stopPanicWatch)
		go func() {
			select {
			case <-panicKill:
				proc.killTree()
			case <-stopPanicWatch:
			}
		}()
	}

	if onPTYReady != nil {
		onPTYReady(proc)
	}

	if term.IsTerminal(int(stdin.Fd())) {
		defer enableTerminalOutput()()
		stopResize := watchResize(stdin, proc, isActiveWriter, onHostSize)
		defer stopResize()

		if oldState, err := term.MakeRaw(int(stdin.Fd())); err == nil {
			defer func() { _ = term.Restore(int(stdin.Fd()), oldState) }()
		}
	}

	go func() {
		dst := io.Writer(proc)
		if isActiveWriter != nil {
			dst = &gatedWriter{dst: proc, active: isActiveWriter, actions: inputActions}
		}
		_, _ = io.Copy(dst, stdin)
	}()
	// A read error here is expected once the child exits and the PTY's
	// slave side closes (creack/pty's documented behavior) - not a real
	// failure, so it's intentionally not surfaced.
	_, _ = io.Copy(stdout, proc)

	code, err := proc.wait()
	if err != nil {
		fmt.Fprintln(os.Stderr, "getsloth:", err)
		return 1
	}
	return code
}

// gatedWriter drops writes instead of forwarding them to dst whenever
// active reports false - this is how the host's own local keystrokes
// stop reaching the PTY the instant a viewer becomes the active writer,
// mirroring the relay silently dropping a non-active-writer viewer's
// input rather than erroring.
type gatedWriter struct {
	dst           io.Writer
	active        *atomic.Bool
	actions       *hostInputActions
	prefixPending bool
}

func (g *gatedWriter) Write(p []byte) (int, error) {
	if g.actions == nil {
		if !g.active.Load() {
			return len(p), nil
		}
		return g.dst.Write(p)
	}

	forward := make([]byte, 0, len(p))
	for _, value := range p {
		if g.prefixPending {
			g.prefixPending = false
			switch value {
			case 'r', 'R':
				if g.actions.onReclaim != nil {
					g.actions.onReclaim()
				}
				continue
			case 'i', 'I':
				if g.actions.onStatus != nil {
					g.actions.onStatus()
				}
				continue
			default:
				if g.active.Load() {
					forward = append(forward, hostCommandPrefix, value)
				}
				continue
			}
		}

		if value == hostCommandPrefix {
			g.prefixPending = true
			continue
		}
		if g.active.Load() {
			forward = append(forward, value)
		}
	}

	if len(forward) > 0 {
		if _, err := g.dst.Write(forward); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

const hostCommandPrefix byte = 0x1d // Ctrl-]

type hostInputActions struct {
	onReclaim func()
	onStatus  func()
}

// watchResize keeps conn's window size matching stdin's terminal size,
// setting it once immediately and again on every resize event. The
// returned func stops watching and must be called to avoid leaking the
// goroutine.
func watchResize(stdin *os.File, conn ptyConn, isActiveWriter *atomic.Bool, onHostSize func(cols, rows int)) func() {
	events, stopEvents := resizeEvents(stdin)
	done := make(chan struct{})
	stopped := make(chan struct{})

	go func() {
		defer close(stopped)
		for {
			select {
			case <-events:
				cols, rows := terminalGridSize(stdin)
				if onHostSize != nil {
					onHostSize(cols, rows)
				}
				if isActiveWriter == nil || isActiveWriter.Load() {
					_ = conn.resize(cols, rows)
				}
			case <-done:
				return
			}
		}
	}()

	return func() {
		stopEvents()
		close(done)
		<-stopped
	}
}

func terminalGridSize(input *os.File) (int, int) {
	cols, rows, err := term.GetSize(int(input.Fd()))
	if err != nil || !validGridSize(cols, rows) {
		return protocol.DefaultCols, protocol.DefaultRows
	}
	return cols, rows
}

func validGridSize(cols, rows int) bool {
	return cols >= 2 && rows >= 2 && cols <= 1000 && rows <= 500
}
