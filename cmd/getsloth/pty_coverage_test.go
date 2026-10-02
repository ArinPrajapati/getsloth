//go:build !windows

package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"
	"golang.org/x/term"
)

type coverageGrid struct {
	cols int
	rows int
}

func waitForCoverageGrid(t *testing.T, updates <-chan coverageGrid) coverageGrid {
	t.Helper()
	select {
	case got := <-updates:
		return got
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for PTY resize callback")
		return coverageGrid{}
	}
}

func readCoverageGrid(t *testing.T, file *os.File) coverageGrid {
	t.Helper()
	size, err := pty.GetsizeFull(file)
	if err != nil {
		t.Fatalf("pty.GetsizeFull: %v", err)
	}
	return coverageGrid{cols: int(size.Cols), rows: int(size.Rows)}
}

func waitForCoverageGridSize(t *testing.T, file *os.File, want coverageGrid) {
	t.Helper()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		if got := readCoverageGrid(t, file); got == want {
			return
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("PTY size did not become %#v before deadline; last size was %#v", want, readCoverageGrid(t, file))
		}
	}
}

func signalWindowResize(t *testing.T) {
	t.Helper()
	process, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("os.FindProcess: %v", err)
	}
	if err := process.Signal(syscall.SIGWINCH); err != nil {
		t.Fatalf("SIGWINCH: %v", err)
	}
}

func TestRun_InteractiveTerminalRestoresRawMode(t *testing.T) {
	master, stdin, err := pty.Open()
	if err != nil {
		t.Fatalf("pty.Open: %v", err)
	}
	defer func() { _ = master.Close() }()
	defer func() { _ = stdin.Close() }()

	before, err := term.GetState(int(stdin.Fd()))
	if err != nil {
		t.Fatalf("term.GetState before run: %v", err)
	}

	var output stringBuffer
	done := make(chan int, 1)
	go func() {
		done <- run([]string{"sh", "-c", "printf 'interactive-ready\\n'; read reply; test \"$reply\" = exit"}, stdin, &output, nil, nil, nil, nil, nil)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for !strings.Contains(output.String(), "interactive-ready") {
		if time.Now().After(deadline) {
			t.Fatal("interactive command did not become ready")
		}
		time.Sleep(time.Millisecond)
	}
	during, err := term.GetState(int(stdin.Fd()))
	if err != nil {
		t.Fatalf("term.GetState during run: %v", err)
	}
	if reflect.DeepEqual(before, during) {
		t.Fatal("interactive stdin was not put into raw mode")
	}
	if _, err := master.Write([]byte("exit\n")); err != nil {
		t.Fatalf("write interactive command input: %v", err)
	}

	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("run exit code = %d, want 0", code)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("interactive run did not exit after shell command")
	}

	after, err := term.GetState(int(stdin.Fd()))
	if err != nil {
		t.Fatalf("term.GetState after run: %v", err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("stdin terminal state was not restored: before %#v, after %#v", before, after)
	}
}

func TestWatchResize_UpdatesActivePTYAndReportsInactiveHostSize(t *testing.T) {
	stdinMaster, stdin, err := pty.Open()
	if err != nil {
		t.Fatalf("pty.Open stdin: %v", err)
	}
	defer func() { _ = stdinMaster.Close() }()
	defer func() { _ = stdin.Close() }()

	ptmx, ptmxSlave, err := pty.Open()
	if err != nil {
		t.Fatalf("pty.Open target: %v", err)
	}
	defer func() { _ = ptmx.Close() }()
	defer func() { _ = ptmxSlave.Close() }()

	if err := pty.Setsize(stdinMaster, &pty.Winsize{Cols: 101, Rows: 41}); err != nil {
		t.Fatalf("set initial stdin size: %v", err)
	}
	if err := pty.Setsize(ptmx, &pty.Winsize{Cols: 77, Rows: 22}); err != nil {
		t.Fatalf("set initial target size: %v", err)
	}

	updates := make(chan coverageGrid, 4)
	var active atomic.Bool
	active.Store(false)
	stop := watchResize(stdin, newUnixPTY(ptmx), &active, func(cols, rows int) {
		updates <- coverageGrid{cols: cols, rows: rows}
	})
	defer stop()
	initial := waitForCoverageGrid(t, updates)
	if initial != (coverageGrid{cols: 101, rows: 41}) {
		t.Fatalf("initial inactive-host callback = %#v, want 101x41", initial)
	}
	if got := readCoverageGrid(t, ptmx); got != (coverageGrid{cols: 77, rows: 22}) {
		t.Fatalf("inactive host resized target to %#v, want 77x22", got)
	}

	active.Store(true)
	if err := pty.Setsize(stdinMaster, &pty.Winsize{Cols: 123, Rows: 45}); err != nil {
		t.Fatalf("set active stdin size: %v", err)
	}
	signalWindowResize(t)
	updated := waitForCoverageGrid(t, updates)
	if updated != (coverageGrid{cols: 123, rows: 45}) {
		t.Fatalf("active resize callback = %#v, want 123x45", updated)
	}
	waitForCoverageGridSize(t, ptmx, coverageGrid{cols: 123, rows: 45})

}

func TestWatchResize_StopWaitsForInFlightCallback(t *testing.T) {
	master, stdin, err := pty.Open()
	if err != nil {
		t.Fatalf("pty.Open: %v", err)
	}
	defer func() { _ = master.Close() }()
	defer func() { _ = stdin.Close() }()

	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce, stopOnce, startedOnce sync.Once
	releaseCallback := func() { releaseOnce.Do(func() { close(release) }) }
	stop := watchResize(stdin, newUnixPTY(master), nil, func(int, int) {
		startedOnce.Do(func() { close(started) })
		<-release
	})
	stopWatcher := func() { stopOnce.Do(stop) }
	defer func() {
		releaseCallback()
		stopWatcher()
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("resize callback did not start")
	}

	stopped := make(chan struct{})
	go func() {
		stopWatcher()
		close(stopped)
	}()
	select {
	case <-stopped:
		t.Fatal("watcher shutdown returned while its callback was still running")
	case <-time.After(50 * time.Millisecond):
	}
	releaseCallback()
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("watcher shutdown did not finish after the callback returned")
	}
}

func TestGatedWriter_PrefixStateSpansWrites(t *testing.T) {
	var out bytes.Buffer
	var active atomic.Bool
	active.Store(true)
	w := &gatedWriter{
		dst:    &out,
		active: &active,
		actions: &hostInputActions{
			onReclaim: func() { t.Fatal("unknown prefix byte was treated as reclaim") },
		},
	}

	if _, err := w.Write([]byte{hostCommandPrefix}); err != nil {
		t.Fatalf("prefix Write: %v", err)
	}
	if !w.prefixPending {
		t.Fatal("prefix byte did not remain pending across writes")
	}
	if _, err := w.Write([]byte{'x'}); err != nil {
		t.Fatalf("continuation Write: %v", err)
	}
	if w.prefixPending {
		t.Fatal("prefix remained pending after continuation")
	}
	if !bytes.Equal(out.Bytes(), []byte{hostCommandPrefix, 'x'}) {
		t.Fatalf("forwarded bytes = %v, want %v", out.Bytes(), []byte{hostCommandPrefix, 'x'})
	}
}

type coverageErrorWriter struct{}

func (coverageErrorWriter) Write([]byte) (int, error) {
	return 0, errCoverageDestination
}

func TestGatedWriter_ReturnsDestinationError(t *testing.T) {
	var active atomic.Bool
	active.Store(true)
	w := &gatedWriter{dst: coverageErrorWriter{}, active: &active}

	n, err := w.Write([]byte("input"))
	if !errors.Is(err, errCoverageDestination) {
		t.Fatalf("Write error = %v, want destination error", err)
	}
	if n != 0 {
		t.Fatalf("Write n = %d, want 0 when destination fails", n)
	}
}

var errCoverageDestination = errors.New("coverage destination failure")

func TestTerminalGridSize_RejectsInvalidTerminalGeometry(t *testing.T) {
	for _, grid := range []coverageGrid{
		{cols: 1, rows: 24},
		{cols: 80, rows: 1},
		{cols: 1001, rows: 24},
		{cols: 80, rows: 501},
	} {
		if validGridSize(grid.cols, grid.rows) {
			t.Errorf("validGridSize(%d, %d) = true, want false", grid.cols, grid.rows)
		}
	}
	if !validGridSize(2, 2) || !validGridSize(1000, 500) {
		t.Fatal("validGridSize rejected an allowed boundary")
	}

	stdin := nonTerminalStdin(t)
	if cols, rows := terminalGridSize(stdin); cols != 80 || rows != 24 {
		t.Fatalf("terminalGridSize(non-terminal) = (%d, %d), want (80, 24)", cols, rows)
	}
}

func TestRelayOutputWriter_EmptyWriteIsNoOp(t *testing.T) {
	w := &relayOutputWriter{}
	if n, err := io.WriteString(w, ""); n != 0 || err != nil {
		t.Fatalf("empty relay output write = (%d, %v), want (0, nil)", n, err)
	}
}
