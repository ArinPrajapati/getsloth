package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"time"

	"golang.org/x/term"
)

func runHostControlConsole(args []string, out io.Writer) int {
	socketPath, action, watch, ok := hostControlArguments(args)
	if !ok {
		_, _ = fmt.Fprintln(out, "getsloth control: usage: getsloth control --socket <path> [--watch|--action reclaim|kill]")
		return 2
	}
	if watch {
		return watchHostControlConsole(socketPath, out)
	}

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		_, _ = fmt.Fprintf(out, "getsloth control: connect to host session: %v\n", err)
		return 1
	}
	defer func() { _ = conn.Close() }()

	if err := json.NewEncoder(conn).Encode(hostControlRequest{Action: action}); err != nil {
		_, _ = fmt.Fprintf(out, "getsloth control: request host status: %v\n", err)
		return 1
	}

	var response hostControlResponse
	if err := json.NewDecoder(conn).Decode(&response); err != nil {
		_, _ = fmt.Fprintf(out, "getsloth control: read host status: %v\n", err)
		return 1
	}
	if response.Error != "" {
		_, _ = fmt.Fprintf(out, "getsloth control: %s\n", response.Error)
		return 1
	}
	if response.Snapshot == nil {
		_, _ = fmt.Fprintln(out, "getsloth control: host returned no session status")
		return 1
	}

	renderHostControlSnapshot(out, *response.Snapshot)
	return 0
}

func hostControlArguments(args []string) (socketPath, action string, watch, ok bool) {
	if len(args) == 2 && args[0] == "--socket" && args[1] != "" {
		return args[1], "snapshot", false, true
	}
	if len(args) == 3 && args[0] == "--socket" && args[1] != "" && args[2] == "--watch" {
		return args[1], "snapshot", true, true
	}
	if len(args) == 4 && args[0] == "--socket" && args[1] != "" && args[2] == "--action" && args[3] != "" {
		return args[1], args[3], false, true
	}
	return "", "", false, false
}

func hostControlKeyAction(key byte) string {
	switch key {
	case 'r', 'R':
		return "reclaim"
	case 'k', 'K':
		return "kill"
	case 'q', 'Q':
		return "quit"
	case 'i', 'I':
		return "snapshot"
	default:
		return ""
	}
}

func watchHostControlConsole(socketPath string, out io.Writer) int {
	keypresses := make(chan byte, 1)
	if term.IsTerminal(int(os.Stdin.Fd())) {
		oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
		if err == nil {
			defer func() { _ = term.Restore(int(os.Stdin.Fd()), oldState) }()
			go readHostControlKeys(os.Stdin, keypresses)
		}
	}

	for {
		_, _ = fmt.Fprint(out, "\x1b[H\x1b[2J")
		if code := runHostControlConsole([]string{"--socket", socketPath}, out); code != 0 {
			return code
		}

		select {
		case key := <-keypresses:
			action := hostControlKeyAction(key)
			if action == "quit" {
				return 0
			}
			if action == "reclaim" || action == "kill" {
				_ = runHostControlConsole([]string{"--socket", socketPath, "--action", action}, io.Discard)
			}
		case <-time.After(time.Second):
		}
	}
}

func readHostControlKeys(input *os.File, keypresses chan<- byte) {
	buffer := make([]byte, 64)
	for {
		count, err := input.Read(buffer)
		if err != nil {
			return
		}
		for _, key := range buffer[:count] {
			select {
			case keypresses <- key:
			default:
			}
		}
	}
}

func renderHostControlSnapshot(out io.Writer, snapshot hostControlSnapshot) {
	width := 80
	if terminal, ok := out.(*os.File); ok {
		if cols, _, err := term.GetSize(int(terminal.Fd())); err == nil && cols > 0 {
			width = cols
		}
	}
	_, _ = fmt.Fprintln(out, renderHostControlDashboard(snapshot, width))
}
