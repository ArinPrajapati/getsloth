//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

// CREATE_NEW_CONSOLE is not exported by Go's syscall package.
const windowsCreateNewConsole uint32 = 0x00000010

func startWindowsHostControlLauncher(name string, args []string) error {
	return startWindowsHostControlProcess(name, args, 0)
}

func startWindowsHostControlFallback(name string, args []string) error {
	return startWindowsHostControlProcess(name, args, windowsCreateNewConsole)
}

func startWindowsHostControlProcess(name string, args []string, creationFlags uint32) error {
	cmd := exec.Command(name, args...)
	if creationFlags != 0 {
		cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: creationFlags}
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
