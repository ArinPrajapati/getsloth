//go:build !windows

package main

import (
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"

	"github.com/creack/pty"
)

type unixPTY struct {
	*os.File
	cmd *exec.Cmd

	// resize reads the descriptor via File.Fd while the message loop may be
	// tearing the session down, and os.File's Close is not safe against a
	// concurrent Fd call, so both go through mu.
	mu     sync.Mutex
	closed bool
}

// startPTY spawns args on a new pseudo-terminal. pty.Start calls setsid()
// internally (needed to make the PTY the controlling terminal), which
// makes the child both a session leader and its own process group
// leader, so cmd.Process.Pid equals the group ID that killTree signals.
// Setting Setpgid as well would try to setpgid() a session leader, which
// POSIX rejects with EPERM.
func startPTY(args []string, cols, rows int) (ptyProcess, error) {
	cmd := exec.Command(args[0], args[1:]...)
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	if err != nil {
		return nil, err
	}
	return &unixPTY{File: ptmx, cmd: cmd}, nil
}

func newUnixPTY(f *os.File) ptyConn { return &unixPTY{File: f} }

func (p *unixPTY) resize(cols, rows int) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return os.ErrClosed
	}
	return pty.Setsize(p.File, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
}

func (p *unixPTY) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	return p.File.Close()
}

func (p *unixPTY) wait() (int, error) {
	if err := p.cmd.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode(), nil
		}
		return 0, err
	}
	return 0, nil
}

// killTree sends SIGKILL to the whole process group, so a background
// process the session started cannot outlive killing just the shell.
func (p *unixPTY) killTree() {
	_ = syscall.Kill(-p.cmd.Process.Pid, syscall.SIGKILL)
}

func resizeEvents(*os.File) (<-chan struct{}, func()) {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGWINCH)
	events := make(chan struct{}, 1)
	events <- struct{}{}
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-sig:
				select {
				case events <- struct{}{}:
				default:
				}
			case <-done:
				return
			}
		}
	}()
	return events, func() {
		signal.Stop(sig)
		close(done)
	}
}

func notifySoftSignals(ch chan<- os.Signal) {
	signal.Notify(ch, syscall.SIGUSR1, syscall.SIGUSR2)
}

func softSignalAction(sig os.Signal) softAction {
	switch sig {
	case syscall.SIGUSR2:
		return softReclaim
	case syscall.SIGUSR1:
		return softKillViewers
	}
	return softNone
}
