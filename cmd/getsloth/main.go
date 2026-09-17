package main

import (
	"fmt"
	"io"
	"os"
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

	stdout := io.Writer(os.Stdout)

	ws, created, err := connectHost(relayURL)
	if err != nil {
		// Degrade to local-only rather than fail the whole command - a
		// command wrapped by getsloth should still work exactly like
		// running it directly (B2's guarantee) even if nobody can watch
		// it right now.
		fmt.Fprintf(os.Stderr, "getsloth: could not reach relay at %s: %v\n", relayURL, err)
		fmt.Fprintln(os.Stderr, "getsloth: continuing locally only - nobody can watch this session")
	} else {
		defer func() { _ = ws.Close() }()
		// Share-link/password printing (FR2) arrives in Task B5, once
		// the auth keypair this URL needs actually exists.
		fmt.Fprintf(os.Stderr, "getsloth: session %s is live\n", created.SessionID)
		stdout = io.MultiWriter(os.Stdout, &relayOutputWriter{ws: ws})
	}

	os.Exit(run(os.Args[1:], os.Stdin, stdout))
}
