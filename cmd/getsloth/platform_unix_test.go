//go:build !windows

package main

import (
	"os"
	"testing"
)

func TestDefaultShell_UsesShellEnv(t *testing.T) {
	t.Setenv("SHELL", "/bin/zsh")
	if got := defaultShell(); got != "/bin/zsh" {
		t.Errorf("defaultShell() = %q, want /bin/zsh", got)
	}
	t.Setenv("SHELL", "")
	if got := defaultShell(); got != "/bin/sh" {
		t.Errorf("defaultShell() without SHELL = %q, want /bin/sh", got)
	}
}

func TestRestrictToCurrentUser_SetsOwnerOnlyModes(t *testing.T) {
	dir := t.TempDir()
	file := dir + "/sock"
	if err := os.WriteFile(file, nil, 0o666); err != nil {
		t.Fatal(err)
	}
	if err := restrictToCurrentUser(dir, true); err != nil {
		t.Fatal(err)
	}
	if err := restrictToCurrentUser(file, false); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]os.FileMode{dir: 0o700, file: 0o600} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != want {
			t.Errorf("%s mode = %o, want %o", path, got, want)
		}
	}
}
