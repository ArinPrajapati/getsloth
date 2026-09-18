package main

import (
	"strings"
	"testing"
)

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
