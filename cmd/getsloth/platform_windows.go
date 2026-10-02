//go:build windows

package main

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// defaultShell prefers COMSPEC, which Windows sets to the command
// interpreter the user is configured to run.
func defaultShell() string {
	if shell := os.Getenv("COMSPEC"); shell != "" {
		return shell
	}
	return "cmd.exe"
}

// enableTerminalOutput switches the host console to UTF-8 and virtual
// terminal processing so the escape sequences ConPTY emits render instead
// of printing literally. The returned func restores the previous modes.
func enableTerminalOutput() func() {
	handle := windows.Handle(os.Stdout.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(handle, &mode); err != nil {
		return func() {}
	}
	var restoreMode, restoreCP bool
	if windows.SetConsoleMode(handle, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING) == nil {
		restoreMode = true
	}
	previousCP, cpErr := getConsoleOutputCP()
	if cpErr == nil && windows.SetConsoleOutputCP(65001) == nil {
		restoreCP = true
	}
	return func() {
		if restoreMode {
			_ = windows.SetConsoleMode(handle, mode)
		}
		if restoreCP {
			_ = windows.SetConsoleOutputCP(previousCP)
		}
	}
}

func getConsoleOutputCP() (uint32, error) {
	proc := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetConsoleOutputCP")
	cp, _, err := proc.Call()
	if cp == 0 {
		return 0, err
	}
	return uint32(cp), nil
}

// restrictToCurrentUser replaces the path's ACL with one granting only the
// current user, with inheritance blocked. On Windows os.Chmod cannot express
// this, and the default %TEMP% ACL is not enough to keep other local
// accounts from connecting to the control socket.
func restrictToCurrentUser(path string, isDir bool) error {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return fmt.Errorf("look up current user: %w", err)
	}
	inheritance := uint32(windows.NO_INHERITANCE)
	if isDir {
		inheritance = windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT
	}
	acl, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{{
		AccessPermissions: windows.GENERIC_ALL,
		AccessMode:        windows.SET_ACCESS,
		Inheritance:       inheritance,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  windows.TRUSTEE_IS_USER,
			TrusteeValue: windows.TrusteeValueFromSID(user.User.Sid),
		},
	}}, nil)
	if err != nil {
		return fmt.Errorf("build access list: %w", err)
	}
	return windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil)
}
