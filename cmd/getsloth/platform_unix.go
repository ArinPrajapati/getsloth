//go:build !windows

package main

import "os"

func defaultShell() string {
	if shell := os.Getenv("SHELL"); shell != "" {
		return shell
	}
	return "/bin/sh"
}

// enableTerminalOutput is a no-op: Unix terminals already interpret the
// escape sequences the PTY emits.
func enableTerminalOutput() func() { return func() {} }

// restrictToCurrentUser limits the host control endpoint to the user
// running the host, so another local user cannot read the session's
// status or invoke reclaim and kill.
func restrictToCurrentUser(path string, isDir bool) error {
	mode := os.FileMode(0o600)
	if isDir {
		mode = 0o700
	}
	return os.Chmod(path, mode)
}
