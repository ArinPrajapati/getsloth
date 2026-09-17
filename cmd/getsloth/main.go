package main

import (
	"fmt"
	"io"
	"os"
	"sync/atomic"

	"github.com/arinprajapati/getsloth/internal/hostauth"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: getsloth <command> [args...]")
		os.Exit(2)
	}

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
		defer func() { _ = ws.Close() }()

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

		go runHostMessageLoop(ws, created.SessionID, password, keys, active, ptmxCh)
		stdout = io.MultiWriter(os.Stdout, &relayOutputWriter{ws: ws})
	}

	os.Exit(run(os.Args[1:], os.Stdin, stdout, isActiveWriter, onPTYReady))
}
