package main

import (
	"errors"
	"os/exec"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestHostControlLauncherCoverage_FindsLaterLinuxCandidate(t *testing.T) {
	got, err := findLinuxTerminal(func(name string) (string, error) {
		if name == "konsole" {
			return "/usr/bin/konsole", nil
		}
		return "", exec.ErrNotFound
	})
	if err != nil || got != "/usr/bin/konsole" {
		t.Fatalf("findLinuxTerminal() = %q, %v, want later candidate", got, err)
	}
}

func TestHostControlLauncherCoverage_LinuxSuccessAndStartFailure(t *testing.T) {
	var startedName string
	var startedArgs []string
	err := launchLinuxHostControlConsole(
		"/opt/Get Sloth/getsloth",
		"/tmp/control.sock",
		func(name string) (string, error) { return "/usr/bin/xterm", nil },
		func(name string, args []string) error {
			startedName = name
			startedArgs = append([]string(nil), args...)
			return nil
		},
	)
	if err != nil {
		t.Fatalf("launchLinuxHostControlConsole success error = %v", err)
	}
	if startedName != "/usr/bin/xterm" || !reflect.DeepEqual(startedArgs, []string{"-e", "/opt/Get Sloth/getsloth", "control", "--socket", "/tmp/control.sock", "--watch"}) {
		t.Fatalf("started Linux console = %q %#v", startedName, startedArgs)
	}

	err = launchLinuxHostControlConsole(
		"/opt/getsloth",
		"/tmp/control.sock",
		func(string) (string, error) { return "/usr/bin/xterm", nil },
		func(string, []string) error { return errors.New("terminal start failed") },
	)
	if err == nil || !strings.Contains(err.Error(), "terminal start failed") {
		t.Fatalf("Linux start failure = %v, want wrapped start error", err)
	}
}

func TestHostControlLauncherCoverage_StartsOnlyHarmlessHelperProcess(t *testing.T) {
	if err := startLinuxHostControlLauncher("sh", []string{"-c", "exit 0"}); err != nil {
		t.Fatalf("startLinuxHostControlLauncher helper: %v", err)
	}
	if err := startLinuxHostControlLauncher("getsloth-command-that-does-not-exist", nil); err == nil {
		t.Fatal("startLinuxHostControlLauncher missing executable succeeded")
	}
}

func TestHostControlLauncherCoverage_FindsLaterWindowsCandidate(t *testing.T) {
	got, err := findWindowsTerminal(func(name string) (string, error) {
		if name == "wt" {
			return `C:\Windows\wt.exe`, nil
		}
		return "", exec.ErrNotFound
	})
	if err != nil || got != `C:\Windows\wt.exe` {
		t.Fatalf("findWindowsTerminal() = %q, %v, want later candidate", got, err)
	}
}

func TestHostControlLauncherCoverage_WindowsTerminalSuccess(t *testing.T) {
	var startedName string
	var startedArgs []string
	err := launchWindowsHostControlConsole(
		`C:\getsloth.exe`,
		`C:\control.sock`,
		func(string) (string, error) { return `C:\Windows\wt.exe`, nil },
		func(name string, args []string) error {
			startedName = name
			startedArgs = append([]string(nil), args...)
			return nil
		},
		func(string, []string) error { return errors.New("fallback must not run") },
	)
	if err != nil {
		t.Fatalf("Windows Terminal success error = %v", err)
	}
	want := []string{"-w", "-1", "new-tab", "--title", "getsloth host control", `C:\getsloth.exe`, "control", "--socket", `C:\control.sock`, "--watch"}
	if startedName != `C:\Windows\wt.exe` || !reflect.DeepEqual(startedArgs, want) {
		t.Fatalf("Windows Terminal invocation = %q %#v, want %q %#v", startedName, startedArgs, `C:\Windows\wt.exe`, want)
	}
}

func TestHostControlLauncherCoverage_PlatformLauncherBehavior(t *testing.T) {
	if runtime.GOOS == "windows" {
		if err := startWindowsHostControlLauncher("cmd.exe", []string{"/c", "exit", "0"}); err != nil {
			t.Fatalf("Windows launcher helper: %v", err)
		}
		if err := startWindowsHostControlFallback("cmd.exe", []string{"/c", "exit", "0"}); err != nil {
			t.Fatalf("Windows fallback helper: %v", err)
		}
	} else {
		if err := startWindowsHostControlLauncher("ignored", nil); err == nil || !strings.Contains(err.Error(), "only available on Windows") {
			t.Fatalf("Windows launcher stub error = %v", err)
		}
		if err := startWindowsHostControlFallback("ignored", nil); err == nil || !strings.Contains(err.Error(), "only available on Windows") {
			t.Fatalf("Windows fallback stub error = %v", err)
		}
	}
}
