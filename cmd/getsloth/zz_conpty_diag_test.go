//go:build windows

package main

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestZZConPTYDiag(t *testing.T) {
	var report strings.Builder
	for _, suspended := range []bool{true, false} {
		conptyCreateSuspended = suspended
		proc, err := startPTY([]string{comspec(t), "/c", "echo hello-conpty & exit 3"}, 80, 24)
		if err != nil {
			fmt.Fprintf(&report, "suspended=%v startPTY error: %v\n", suspended, err)
			continue
		}
		start := time.Now()
		var mu sync.Mutex
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			buf := make([]byte, 4096)
			for {
				n, err := proc.Read(buf)
				mu.Lock()
				fmt.Fprintf(&report, "  [%4dms] read n=%d err=%v %q\n", time.Since(start).Milliseconds(), n, err, buf[:n])
				mu.Unlock()
				if err != nil {
					return
				}
			}
		}()
		code, werr := proc.wait()
		mu.Lock()
		fmt.Fprintf(&report, "suspended=%v exited code=%d err=%v at %dms\n", suspended, code, werr, time.Since(start).Milliseconds())
		mu.Unlock()
		time.Sleep(1500 * time.Millisecond)
		_ = proc.Close()
		wg.Wait()
	}
	conptyCreateSuspended = true
	t.Errorf("DIAGNOSTIC (intentional):\n%s", report.String())
}
