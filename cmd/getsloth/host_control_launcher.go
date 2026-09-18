package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
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

func launchHostControlConsole(socketPath string) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("automatic host control console is currently supported on macOS only")
	}
	executablePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("find getsloth executable: %w", err)
	}
	name, args := hostControlLaunchArgs(executablePath, socketPath)
	if err := exec.Command(name, args...).Run(); err != nil {
		return fmt.Errorf("open Terminal host control console: %w", err)
	}
	return nil
}
