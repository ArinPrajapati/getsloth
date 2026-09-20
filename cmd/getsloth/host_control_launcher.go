package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const hostControlAppleScript = `on run argv
	set executablePath to item 1 of argv
	set socketPath to item 2 of argv
	set commandText to quoted form of executablePath & " control --socket " & quoted form of socketPath & " --watch"
	tell application "Terminal"
		activate
		do script commandText
	end tell
end run`

func hostControlLaunchArgs(executablePath, socketPath string) (string, []string) {
	return "osascript", []string{"-e", hostControlAppleScript, executablePath, socketPath}
}

var linuxTerminalCandidates = []string{"gnome-terminal", "konsole", "xterm", "x-terminal-emulator"}

func findLinuxTerminal(lookPath func(string) (string, error)) (string, error) {
	for _, name := range linuxTerminalCandidates {
		if path, err := lookPath(name); err == nil {
			return path, nil
		}
	}
	return "", errors.New("no supported Linux terminal emulator found")
}

func linuxHostControlLaunchArgs(terminalPath, executablePath, socketPath string) []string {
	command := []string{executablePath, "control", "--socket", socketPath, "--watch"}
	if strings.TrimSuffix(filepath.Base(terminalPath), filepath.Ext(terminalPath)) == "gnome-terminal" {
		return append([]string{"--"}, command...)
	}
	return append([]string{"-e"}, command...)
}

func launchLinuxHostControlConsole(executablePath, socketPath string, lookPath func(string) (string, error), start func(string, []string) error) error {
	terminalPath, err := findLinuxTerminal(lookPath)
	if err != nil {
		return err
	}
	if err := start(terminalPath, linuxHostControlLaunchArgs(terminalPath, executablePath, socketPath)); err != nil {
		return fmt.Errorf("open Linux host control console: %w", err)
	}
	return nil
}

func startLinuxHostControlLauncher(name string, args []string) error {
	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

func launchHostControlConsole(socketPath string) error {
	executablePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("find getsloth executable: %w", err)
	}

	switch runtime.GOOS {
	case "darwin":
		name, args := hostControlLaunchArgs(executablePath, socketPath)
		if err := exec.Command(name, args...).Run(); err != nil {
			return fmt.Errorf("open Terminal host control console: %w", err)
		}
		return nil
	case "linux":
		return launchLinuxHostControlConsole(executablePath, socketPath, exec.LookPath, startLinuxHostControlLauncher)
	default:
		return fmt.Errorf("automatic host control console is not supported on %s", runtime.GOOS)
	}
}
