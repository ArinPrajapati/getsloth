package main

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"

	"github.com/arinprajapati/getsloth/internal/hostauth"
	"github.com/arinprajapati/getsloth/internal/protocol"
)

func main() {
	command := commandFromArgs(os.Args)

	relayURL := os.Getenv("GETSLOTH_RELAY_URL")
	if relayURL == "" {
		relayURL = "ws://localhost:8080"
	}
	webBaseURL := os.Getenv("GETSLOTH_WEB_URL")
	if webBaseURL == "" {
		webBaseURL = "https://getsloth.dev"
	}

	password := os.Getenv("GETSLOTH_PASSWORD")
	if password == "" {
		var err error
		password, err = generatePassword(defaultPasswordLength)
		if err != nil {
			fmt.Fprintln(os.Stderr, "getsloth: could not generate a password:", err)
			os.Exit(1)
		}
	}

	stdout := io.Writer(os.Stdout)
	var isActiveWriter *atomic.Bool
	var onPTYReady func(*os.File)

	ws, created, err := connectHost(relayURL)
	if err != nil {
		// Degrade to local-only rather than fail the whole command - a
		// command wrapped by getsloth should still work exactly like
		// running it directly (B2's guarantee) even if nobody can watch
		// it right now. No relay connection means no shared control
		// concept to gate against, so isActiveWriter/onPTYReady stay
		// nil - run() treats that as "always active."
		fmt.Fprintf(os.Stderr, "getsloth: could not reach relay at %s: %v\n", relayURL, err)
		fmt.Fprintln(os.Stderr, "getsloth: continuing locally only - nobody can watch this session")
	} else {
		keys, err := hostauth.NewKeyPair()
		if err != nil {
			fmt.Fprintln(os.Stderr, "getsloth: could not generate session keys:", err)
			os.Exit(1)
		}

		// Printed separately, per docs/protocol.md's Share link format -
		// the URL carries the auth public key (never a secret on its
		// own), the password is a distinct line and is never part of
		// the URL in either the path or the fragment.
		fmt.Fprintf(os.Stderr, "getsloth: live at %s\n", shareURL(webBaseURL, created.SessionID, keys.PublicKeyBase64URL()))
		fmt.Fprintf(os.Stderr, "getsloth: password: %s\n", password)

		active := &atomic.Bool{}
		active.Store(true) // host starts as the active writer
		isActiveWriter = active

		ptmxCh := make(chan *os.File, 1)
		onPTYReady = func(f *os.File) { ptmxCh <- f }

		go runHostMessageLoop(ws, created.SessionID, password, keys, active, ptmxCh, os.Stderr)
		stdout = io.MultiWriter(os.Stdout, &relayOutputWriter{ws: ws})

		// Host-triggered actions per docs/protocol.md: reclaiming
		// control and the (soft) kill switch. v0 doesn't scan the
		// host's raw keystroke stream for a hotkey (fragile - a byte
		// matching the hotkey could legitimately appear split across
		// two reads from a real program's output); a signal is a
		// simpler, reliable "host-triggered" mechanism for the same
		// intent, scriptable via `kill -USR2 <pid>` (reclaim control)
		// or `kill -USR1 <pid>` (soft kill switch - disconnects
		// viewers, session stays alive).
		signals := make(chan os.Signal, 2)
		signal.Notify(signals, syscall.SIGUSR1, syscall.SIGUSR2)
		go func() {
			for sig := range signals {
				switch sig {
				case syscall.SIGUSR2:
					_ = ws.WriteJSON(protocol.TakeControlMsg{Envelope: protocol.NewEnvelope("take_control")})
				case syscall.SIGUSR1:
					_ = ws.WriteJSON(protocol.KillSwitchMsg{Envelope: protocol.NewEnvelope("kill_switch")})
					fmt.Fprintln(os.Stderr, "getsloth: kill switch triggered - all viewers disconnected, session still live")
				}
			}
		}()
	}

	// Panic kill: a break-glass failsafe for "someone else may have
	// control of my machine right now," distinct from the soft kill
	// switch above. Available regardless of whether a relay connection
	// exists - protecting the host's machine doesn't depend on
	// networking. Triggered by SIGINT or SIGTERM (`kill <pid>` or
	// `kill -INT <pid>`), and does two things in order: first cuts off
	// the sloth session itself (best-effort - the network may be part
	// of the problem, so this must never block the second, guaranteed
	// step), then kills the wrapped command and every process it
	// spawned during the session, not just the top-level one. The
	// process itself then exits entirely - this is not something to
	// casually continue past, a fresh session is started manually
	// afterward if needed.
	panicKill := make(chan struct{})
	panicSignals := make(chan os.Signal, 1)
	signal.Notify(panicSignals, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-panicSignals
		fmt.Fprintln(os.Stderr, "getsloth: PANIC KILL triggered - cutting off remote access, then terminating all session processes")
		if ws != nil {
			_ = ws.WriteJSON(protocol.KillSwitchMsg{Envelope: protocol.NewEnvelope("kill_switch")})
			_ = ws.WriteJSON(protocol.EndSessionMsg{Envelope: protocol.NewEnvelope("end_session")})
			_ = ws.Close()
		}
		close(panicKill)
	}()

	exitCode := run(command, os.Stdin, stdout, isActiveWriter, onPTYReady, panicKill)

	if ws != nil {
		// os.Exit below skips deferred functions, so cleanup happens
		// here explicitly: tell the relay this is a clean end (not an
		// abrupt drop, which would otherwise be indistinguishable) per
		// docs/protocol.md's SessionEndedMsg reasons, then close. A
		// no-op if the panic-kill path above already did this.
		_ = ws.WriteJSON(protocol.EndSessionMsg{Envelope: protocol.NewEnvelope("end_session")})
		_ = ws.Close()
	}

	os.Exit(exitCode)
}

func commandFromArgs(args []string) []string {
	if len(args) > 1 {
		return args[1:]
	}

	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	return []string{shell}
}
