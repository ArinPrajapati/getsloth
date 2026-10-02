package main

import (
	"errors"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

func TestFindLinuxTerminal_PrefersGnomeTerminal(t *testing.T) {
	lookPath := func(name string) (string, error) {
		if name == "gnome-terminal" {
			return "/usr/bin/gnome-terminal", nil
		}
		return "", exec.ErrNotFound
	}

	got, err := findLinuxTerminal(lookPath)
	if err != nil || got != "/usr/bin/gnome-terminal" {
		t.Fatalf("findLinuxTerminal() = %q, %v", got, err)
	}
}

func TestFindLinuxTerminal_ReturnsErrorWhenNoneAreInstalled(t *testing.T) {
	_, err := findLinuxTerminal(func(string) (string, error) { return "", exec.ErrNotFound })
	if err == nil {
		t.Fatal("findLinuxTerminal() error = nil, want unsupported-terminal error")
	}
}

func TestLinuxHostControlLaunchArgs_UsesDirectCommandArguments(t *testing.T) {
	tests := []struct {
		name     string
		terminal string
		want     []string
	}{
		{
			name:     "gnome terminal",
			terminal: "/usr/bin/gnome-terminal",
			want:     []string{"--", "/opt/Get Sloth/getsloth", "control", "--socket", "/tmp/control.sock", "--watch"},
		},
		{
			name:     "xterm",
			terminal: "/usr/bin/xterm",
			want:     []string{"-e", "/opt/Get Sloth/getsloth", "control", "--socket", "/tmp/control.sock", "--watch"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := linuxHostControlLaunchArgs(test.terminal, "/opt/Get Sloth/getsloth", "/tmp/control.sock")
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("args = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestLaunchLinuxHostControlConsole_DoesNotStartWhenNoTerminalExists(t *testing.T) {
	started := false
	err := launchLinuxHostControlConsole(
		"/usr/local/bin/getsloth", "/tmp/control.sock",
		func(string) (string, error) { return "", exec.ErrNotFound },
		func(string, []string) error { started = true; return nil },
	)
	if err == nil || started {
		t.Fatalf("err = %v, started = %t; want fallback error without a process", err, started)
	}
}

func TestFindWindowsTerminal_PrefersWindowsTerminalExecutable(t *testing.T) {
	lookPath := func(name string) (string, error) {
		if name == "wt.exe" {
			return `C:\Windows\System32\wt.exe`, nil
		}
		return "", exec.ErrNotFound
	}

	got, err := findWindowsTerminal(lookPath)
	if err != nil || got != `C:\Windows\System32\wt.exe` {
		t.Fatalf("findWindowsTerminal() = %q, %v", got, err)
	}
}

func TestFindWindowsTerminal_ReturnsErrorWhenUnavailable(t *testing.T) {
	_, err := findWindowsTerminal(func(string) (string, error) { return "", exec.ErrNotFound })
	if err == nil {
		t.Fatal("findWindowsTerminal() error = nil, want unavailable-terminal error")
	}
}

func TestWindowsHostControlLaunchArgsOpenNewWindow(t *testing.T) {
	got := windowsHostControlLaunchArgs(`C:\Program Files\getsloth\getsloth.exe`, `C:\Users\host\control.sock`)
	want := []string{
		"-w", "-1", "new-tab", "--title", "getsloth host control",
		`C:\Program Files\getsloth\getsloth.exe`, "control", "--socket", `C:\Users\host\control.sock`, "--watch",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("windowsHostControlLaunchArgs() = %#v, want %#v", got, want)
	}
}

func TestLaunchWindowsHostControlConsoleFallsBackToNativeConsole(t *testing.T) {
	var started string
	err := launchWindowsHostControlConsole(
		`C:\Program Files\getsloth\getsloth.exe`, `C:\Users\host\control.sock`,
		func(string) (string, error) { return "", exec.ErrNotFound },
		func(string, []string) error {
			t.Fatal("Windows Terminal should not start when unavailable")
			return nil
		},
		func(name string, args []string) error {
			started = name + " " + strings.Join(args, " ")
			return nil
		},
	)
	if err != nil {
		t.Fatalf("launchWindowsHostControlConsole() error = %v", err)
	}
	if want := `C:\Program Files\getsloth\getsloth.exe control --socket C:\Users\host\control.sock --watch`; started != want {
		t.Fatalf("native fallback command = %q, want %q", started, want)
	}
}

func TestLaunchWindowsHostControlConsoleFallsBackWhenWindowsTerminalFails(t *testing.T) {
	var fallbackStarted bool
	err := launchWindowsHostControlConsole(
		`C:\getsloth.exe`, `C:\control.sock`,
		func(string) (string, error) { return `C:\Windows\System32\wt.exe`, nil },
		func(string, []string) error { return errors.New("Windows Terminal unavailable") },
		func(name string, args []string) error {
			fallbackStarted = name == `C:\getsloth.exe` && reflect.DeepEqual(args, []string{"control", "--socket", `C:\control.sock`, "--watch"})
			return nil
		},
	)
	if err != nil || !fallbackStarted {
		t.Fatalf("launchWindowsHostControlConsole() = %v, fallback started = %t", err, fallbackStarted)
	}
}

func TestLaunchWindowsHostControlConsoleReturnsFallbackError(t *testing.T) {
	err := launchWindowsHostControlConsole(
		`C:\getsloth.exe`, `C:\control.sock`,
		func(string) (string, error) { return "", exec.ErrNotFound },
		func(string, []string) error { return nil },
		func(string, []string) error { return errors.New("new console failed") },
	)
	if err == nil || !strings.Contains(err.Error(), "new console failed") {
		t.Fatalf("launchWindowsHostControlConsole() error = %v, want native fallback error", err)
	}
}

func TestHostControlLaunchArgs_PassesExecutableAndPrivateSocketSeparately(t *testing.T) {
	name, args := hostControlLaunchArgs("/Applications/Get Sloth/getsloth", "/private/tmp/control.sock")
	if name != "osascript" {
		t.Fatalf("launcher = %q, want osascript", name)
	}
	if len(args) < 4 || args[len(args)-2] != "/Applications/Get Sloth/getsloth" || args[len(args)-1] != "/private/tmp/control.sock" {
		t.Fatalf("launcher args = %#v", args)
	}
	if !strings.Contains(strings.Join(args, " "), "do script") || !strings.Contains(strings.Join(args, " "), "--watch") {
		t.Errorf("AppleScript = %#v, want a watch console command", args)
	}
}
