package main

import (
	"reflect"
	"testing"
)

func TestCommandFromArgs_UsesExplicitCommand(t *testing.T) {
	got := commandFromArgs([]string{"getsloth", "nvim", "README.md"})
	want := []string{"nvim", "README.md"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("commandFromArgs() = %#v, want %#v", got, want)
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
