//go:build windows

package main

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

type diagRun struct {
	proc  *winPTY
	start time.Time
	mu    sync.Mutex
	log   strings.Builder
	done  chan struct{}
}

func startDiag(t *testing.T, args []string, name string) *diagRun {
	t.Helper()
	p, err := startPTY(args, 80, 24)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	d := &diagRun{proc: p.(*winPTY), start: time.Now(), done: make(chan struct{})}
	fmt.Fprintf(&d.log, "=== %s %v\n", name, args[1:])
	go func() {
		defer close(d.done)
		buf := make([]byte, 4096)
		for {
			n, err := d.proc.out.Read(buf)
			d.mu.Lock()
			fmt.Fprintf(&d.log, "  [%4dms] read n=%d err=%v %q\n", time.Since(d.start).Milliseconds(), n, err, buf[:n])
			d.mu.Unlock()
			if err != nil {
				return
			}
		}
	}()
	return d
}

func (d *diagRun) note(format string, a ...any) {
	d.mu.Lock()
	defer d.mu.Unlock()
	fmt.Fprintf(&d.log, "  [%4dms] %s\n", time.Since(d.start).Milliseconds(), fmt.Sprintf(format, a...))
}

func (d *diagRun) waitExit(timeout time.Duration) {
	ev, _ := windows.WaitForSingleObject(d.proc.process, uint32(timeout.Milliseconds()))
	d.note("WaitForSingleObject=%d (0=exited)", ev)
}

func (d *diagRun) finish() string {
	_ = d.proc.Close()
	<-d.done
	return d.log.String()
}

func TestZZConPTYDiag(t *testing.T) {
	var report strings.Builder
	sh := comspec(t)

	// S1: quick command, process really exits, then wait 2s with console still open.
	d := startDiag(t, []string{sh, "/c", "echo hello-s1"}, "S1 quick command, wait 2s after exit before close")
	d.waitExit(5 * time.Second)
	time.Sleep(2 * time.Second)
	report.WriteString(d.finish())

	// S2: interactive shell kept alive; type a command.
	d = startDiag(t, []string{sh}, "S2 interactive cmd.exe")
	time.Sleep(700 * time.Millisecond)
	_, _ = d.proc.Write([]byte("echo hello-s2\r\n"))
	d.note("wrote echo")
	time.Sleep(2 * time.Second)
	_, _ = d.proc.Write([]byte("exit\r\n"))
	d.waitExit(3 * time.Second)
	report.WriteString(d.finish())

	// S3: quick command, answer terminal capability queries up front.
	d = startDiag(t, []string{sh, "/c", "echo hello-s3 & ping -n 2 127.0.0.1 >nul"}, "S3 replies to DSR and DA1 on input")
	_, _ = d.proc.Write([]byte("\x1b[1;1R\x1b[?61;6;7;22;23;24;28;32;42c"))
	d.note("wrote DSR+DA1 replies")
	d.waitExit(6 * time.Second)
	time.Sleep(500 * time.Millisecond)
	report.WriteString(d.finish())

	t.Errorf("DIAGNOSTIC (intentional):\n%s", report.String())
}
