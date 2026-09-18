package main

import (
	"reflect"
	"strings"
	"testing"

	"github.com/arinprajapati/getsloth/internal/protocol"
)

func TestUsageExplainsSessionModePermissions(t *testing.T) {
	usage := strings.ToLower(usageText)
	for _, phrase := range []string{"Remote mode", "one remote viewer", "Group mode", "view and chat", "host keeps control"} {
		if !strings.Contains(usage, strings.ToLower(phrase)) {
			t.Errorf("usage text does not explain %q", phrase)
		}
	}
}

func TestWantsHelpOnlyBeforeTheWrappedCommand(t *testing.T) {
	if !wantsHelp([]string{"getsloth", "--help"}) || !wantsHelp([]string{"getsloth", "-h"}) {
		t.Fatal("top-level help flags were not recognized")
	}
	if wantsHelp([]string{"getsloth", "claude", "--help"}) {
		t.Fatal("a wrapped command's --help flag must remain command input")
	}
}

func TestCommandFromArgs_UsesExplicitCommand(t *testing.T) {
	got := commandFromArgs([]string{"getsloth", "nvim", "README.md"})
	want := []string{"nvim", "README.md"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("commandFromArgs() = %#v, want %#v", got, want)
	}
}

func TestLaunchFromArgs_GroupModeKeepsWrappedCommand(t *testing.T) {
	mode, command := launchFromArgs([]string{"getsloth", "--group", "claude", "--resume"})

	if mode != protocol.SessionModeGroup {
		t.Fatalf("mode = %q, want group", mode)
	}
	if want := []string{"claude", "--resume"}; !reflect.DeepEqual(command, want) {
		t.Fatalf("command = %#v, want %#v", command, want)
	}
}

func TestLaunchFromArgs_DefaultsToRemoteMode(t *testing.T) {
	mode, _ := launchFromArgs([]string{"getsloth", "claude"})
	if mode != protocol.SessionModeRemote {
		t.Fatalf("mode = %q, want remote", mode)
	}
}

func TestCommandFromArgs_DefaultsToUserShell(t *testing.T) {
	t.Setenv("SHELL", "/bin/zsh")

	got := commandFromArgs([]string{"getsloth"})
	want := []string{"/bin/zsh"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("commandFromArgs() = %#v, want %#v", got, want)
	}
}

func TestCommandFromArgs_FallsBackToSh(t *testing.T) {
	t.Setenv("SHELL", "")

	got := commandFromArgs([]string{"getsloth"})
	want := []string{"/bin/sh"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("commandFromArgs() = %#v, want %#v", got, want)
	}
}
