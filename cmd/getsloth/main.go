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

// version is set at build time via -ldflags "-X main.version=...";
// goreleaser sets it from the release tag. "dev" covers `go run`/`go
// install` builds that skip that step.
var version = "dev"

const usageText = `Usage:
  getsloth [--remote] [command [args...]]
  getsloth --group [command [args...]]

Modes:
  Remote mode (default)  One remote viewer can watch and take control.
  Group mode (--group)   Multiple viewers can view and chat; the host keeps control.

  Interactive terminals show a mode picker when no mode flag is supplied.
  Non-interactive terminals default to Remote mode.
`

func main() {
	os.Exit(runCLI(os.Args, os.Stdin, os.Stdout, os.Stderr))
}

func runCLI(args []string, stdin *os.File, stdout, stderr io.Writer) int {
	return runCLIWithLauncher(args, stdin, stdout, stderr, launchHostControlConsole)
}

// Terminal-window launch is separate from session startup so headless callers
// can retain the local control endpoint without opening a desktop window.
func runCLIWithLauncher(args []string, stdin *os.File, stdout, stderr io.Writer, launchControl func(string) error) int {
	if controlArgs, ok := hostControlCommandArgs(args); ok {
		return runHostControlConsole(controlArgs, stdout)
	}
	if wantsHelp(args) {
		if _, err := io.WriteString(stdout, usageText); err != nil {
			return 1
		}
		return 0
	}
	if wantsVersion(args) {
		_, _ = fmt.Fprintf(stdout, "getsloth %s\n", version)
		return 0
	}
	if stdin == nil {
		stdin = os.Stdin
	}

	mode, command := launchFromArgs(args)
	var promptOutput *os.File
	if file, ok := stderr.(*os.File); ok {
		promptOutput = file
	}
	if shouldPromptForSessionMode(args, stdin, promptOutput) {
		selectedMode, confirmed, err := chooseSessionMode(stdin, stderr)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, "getsloth: could not run session mode picker:", err)
			return 1
		}
		if !confirmed {
			return 0
		}
		mode = selectedMode
	}
	hostCols, hostRows := terminalGridSize(stdin)

	// Both default to the hosted production service, matching the
	// README's zero-config promise ("one command on the host, no
	// configure on the viewing device"). Self-hosters override both via
	// env vars - see README's "Self-hosting the relay" section.
	relayURL := os.Getenv("GETSLOTH_RELAY_URL")
	if relayURL == "" {
		relayURL = "wss://relay.getsloth.dev"
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
			_, _ = fmt.Fprintln(stderr, "getsloth: could not generate a password:", err)
			return 1
		}
	}

	localOutput := io.Writer(stdout)
	var isActiveWriter *atomic.Bool
	var onPTYReady func(ptyConn)
	var onHostSize func(cols, rows int)
	var inputActions *hostInputActions
	var controlServer *hostControlServer
	var messageLoopDone chan struct{}
	var softSignals chan os.Signal
	var softStop chan struct{}
	var softDone chan struct{}

	ws, created, err := connectHost(relayURL, protocol.SessionConfigMsg{
		Envelope: protocol.NewEnvelope("session_config"),
		Mode:     mode,
		HostCols: hostCols,
		HostRows: hostRows,
	})
	if err != nil {
		// Degrade to local-only rather than fail the whole command - a
		// command wrapped by getsloth should still work exactly like
		// running it directly (B2's guarantee) even if nobody can watch
		// it right now. No relay connection means no shared control
		// concept to gate against, so isActiveWriter/onPTYReady stay
		// nil - run() treats that as "always active."
		_, _ = fmt.Fprintf(stderr, "getsloth: could not reach relay at %s: %v\n", relayURL, err)
		_, _ = fmt.Fprintln(stderr, "getsloth: continuing locally only - nobody can watch this session")
	} else {
		keys, err := hostauth.NewKeyPair()
		if err != nil {
			_, _ = fmt.Fprintln(stderr, "getsloth: could not generate session keys:", err)
			_ = ws.Close()
			return 1
		}

		// Printed separately, per docs/protocol.md's Share link format -
		// the URL carries the auth public key (never a secret on its
		// own), the password is a distinct line and is never part of
		// the URL in either the path or the fragment. This is the link
		// meant for copy/paste sharing (Slack, SMS) - it stays
		// password-free since it can reach someone who never saw the
		// terminal.
		inviteURL := shareURL(webBaseURL, created.SessionID, keys.PublicKeyBase64URL())
		_, _ = fmt.Fprintf(stderr, "getsloth: live at %s\n", inviteURL)
		_, _ = fmt.Fprintf(stderr, "getsloth: password: %s\n", password)
		// The QR code encodes a *different* URL that also carries the
		// password, so scanning it skips the manual password prompt -
		// see qrShareURL's doc comment and docs/ideas/getsloth.md's "QR
		// bypasses the password prompt" decision for why that's safe
		// specifically for the QR (read off the host's own terminal)
		// but not for the plain link above. It also uses the compressed
		// public key encoding (half the bytes of the plain link's key)
		// since the QR is the one place fewer bytes actually matters -
		// see PublicKeyCompressedBase64URL's doc comment. A rendering
		// failure here should never block the session itself, so it's
		// reported and skipped rather than treated as fatal.
		qrURL := qrShareURL(webBaseURL, created.SessionID, keys.PublicKeyCompressedBase64URL(), password)
		if qr, err := renderQRCode(qrURL); err != nil {
			_, _ = fmt.Fprintln(stderr, "getsloth: could not render QR code:", err)
		} else {
			_, _ = fmt.Fprintln(stderr, "getsloth: scan to open on your phone:")
			_, _ = fmt.Fprint(stderr, qr)
		}

		active := &atomic.Bool{}
		active.Store(true) // host starts as the active writer
		isActiveWriter = active
		status := newHostSessionStatus(mode, stderr)
		status.setInvite(inviteURL, password)
		status.setQRInvite(qrURL)
		controlServer, err = startHostControlServer(status.snapshot)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, "getsloth: host control console unavailable; use Ctrl-] i for status and Ctrl-] r to reclaim")
		}
		_, _ = fmt.Fprintln(stderr, "getsloth: host controls: Ctrl-] r reclaim · Ctrl-] i status")

		ptmxCh := make(chan ptyConn, 1)
		onPTYReady = func(f ptyConn) {
			ptmxCh <- f
			messageLoopDone = make(chan struct{})
			go func() {
				defer close(messageLoopDone)
				runHostMessageLoop(ws, created.SessionID, password, keys, active, ptmxCh, status)
			}()
		}
		onHostSize = func(cols, rows int) {
			_ = ws.WriteJSON(protocol.HostSizeMsg{
				Envelope: protocol.NewEnvelope("host_size"),
				Cols:     cols,
				Rows:     rows,
			})
		}

		reclaim := func() error {
			return ws.WriteJSON(protocol.TakeControlMsg{Envelope: protocol.NewEnvelope("take_control")})
		}
		killViewers := func() error {
			err := ws.WriteJSON(protocol.KillSwitchMsg{Envelope: protocol.NewEnvelope("kill_switch")})
			if err == nil {
				_, _ = fmt.Fprintln(stderr, "getsloth: kill switch triggered - all viewers disconnected, session still live")
				status.noteKillSwitch()
			}
			return err
		}
		if controlServer != nil {
			controlServer.setActions(reclaim, killViewers)
			if launchControl == nil {
				launchControl = launchHostControlConsole
			}
			if err := launchControl(controlServer.socketPath); err != nil {
				_, _ = fmt.Fprintln(stderr, "getsloth: host control console unavailable; use Ctrl-] i for status and Ctrl-] r to reclaim")
			} else {
				_, _ = fmt.Fprintln(stderr, "getsloth: host control console opened in a separate Terminal window")
			}
		}
		inputActions = &hostInputActions{
			onReclaim: func() { _ = reclaim() },
			onStatus:  status.print,
		}
		localOutput = io.MultiWriter(stdout, &relayOutputWriter{ws: ws})

		// Signals remain as scriptable alternatives to the host's local
		// Ctrl-] command prefix: USR2 reclaims control and USR1 triggers
		// the soft kill switch without ending the wrapped process.
		softSignals = make(chan os.Signal, 2)
		softStop = make(chan struct{})
		softDone = make(chan struct{})
		notifySoftSignals(softSignals)
		go func() {
			defer close(softDone)
			for {
				select {
				case sig := <-softSignals:
					switch softSignalAction(sig) {
					case softReclaim:
						_ = reclaim()
					case softKillViewers:
						_ = killViewers()
					}
				case <-softStop:
					return
				}
			}
		}()
	}

	// Signal subscriptions belong to one session; returning must not leave
	// handlers that could target a later session's process group.
	panicKill := make(chan struct{})
	panicSignals := make(chan os.Signal, 1)
	panicStop := make(chan struct{})
	panicDone := make(chan struct{})
	signal.Notify(panicSignals, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		defer close(panicDone)
		select {
		case <-panicSignals:
			_, _ = fmt.Fprintln(stderr, "getsloth: PANIC KILL triggered - cutting off remote access, then terminating all session processes")
			if ws != nil {
				_ = ws.WriteJSON(protocol.KillSwitchMsg{Envelope: protocol.NewEnvelope("kill_switch")})
				_ = ws.WriteJSON(protocol.EndSessionMsg{Envelope: protocol.NewEnvelope("end_session")})
				_ = ws.Close()
			}
			close(panicKill)
		case <-panicStop:
			return
		}
	}()

	cleanup := func() {
		if controlServer != nil {
			_ = controlServer.Close()
		}
		if softSignals != nil {
			signal.Stop(softSignals)
			close(softStop)
			<-softDone
		}
		signal.Stop(panicSignals)
		close(panicStop)
		<-panicDone
		if ws != nil {
			// Tell the relay this is a clean end before closing. If panic
			// kill already did this, the write is harmlessly rejected.
			_ = ws.WriteJSON(protocol.EndSessionMsg{Envelope: protocol.NewEnvelope("end_session")})
			_ = ws.Close()
		}
		if messageLoopDone != nil {
			<-messageLoopDone
		}
	}
	defer cleanup()

	return run(command, stdin, localOutput, isActiveWriter, onPTYReady, onHostSize, inputActions, panicKill)
}

func hostControlCommandArgs(args []string) ([]string, bool) {
	if len(args) > 1 && args[1] == "control" {
		return args[2:], true
	}
	return nil, false
}

func wantsHelp(args []string) bool {
	return len(args) == 2 && (args[1] == "--help" || args[1] == "-h")
}

func wantsVersion(args []string) bool {
	return len(args) == 2 && (args[1] == "--version" || args[1] == "-v")
}

func commandFromArgs(args []string) []string {
	_, command := launchFromArgs(args)
	return command
}

func launchFromArgs(args []string) (string, []string) {
	mode := protocol.SessionModeRemote
	commandStart := 1
	if len(args) > 1 && (args[1] == "--group" || args[1] == "--remote") {
		if args[1] == "--group" {
			mode = protocol.SessionModeGroup
		}
		commandStart = 2
	}
	if len(args) > commandStart && args[commandStart] == "--" {
		commandStart++
	}
	if len(args) > commandStart {
		return mode, args[commandStart:]
	}

	return mode, []string{defaultShell()}
}
