package main

import (
	"os"
	"strings"
	"testing"

	"github.com/arinprajapati/getsloth/internal/protocol"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestSessionModePicker_DefaultsToUsableWidth(t *testing.T) {
	model := newSessionModePicker(0)
	if model.width != 80 {
		t.Fatalf("default width = %d, want 80", model.width)
	}
	if model.Init() != nil {
		t.Fatal("mode picker Init should not schedule a command")
	}
	if got := (sessionModePicker{selected: -1}).selectedMode(); got != protocol.SessionModeRemote {
		t.Fatalf("invalid selected index returned mode %q, want remote", got)
	}
}

func TestSessionModePicker_ViewFitsTerminalWidth(t *testing.T) {
	for _, width := range []int{32, 48, 80} {
		model := newSessionModePicker(width)
		view := model.View()
		for _, line := range strings.Split(view, "\n") {
			if got := lipgloss.Width(line); got > width {
				t.Errorf("width %d: rendered line is %d columns: %q", width, got, line)
			}
		}
	}
}

func TestSessionModePicker_ViewShowsModeCards(t *testing.T) {
	view := newSessionModePicker(80).View()
	for _, want := range []string{"GETSLOTH", "REMOTE", "GROUP", "↑↓", "enter"} {
		if !strings.Contains(view, want) {
			t.Errorf("mode picker does not contain %q: %q", want, view)
		}
	}
}

func TestSessionModePicker_SelectsGroupAndQuits(t *testing.T) {
	model := newSessionModePicker(80)

	updated, cmd := model.Update(tea.KeyMsg(tea.Key{Type: tea.KeyDown}))
	model = updated.(sessionModePicker)
	if model.selected != 1 {
		t.Fatalf("selected option = %d, want group option 1", model.selected)
	}
	if cmd != nil {
		t.Fatal("moving selection should not produce a command")
	}

	updated, cmd = model.Update(tea.KeyMsg(tea.Key{Type: tea.KeyEnter}))
	model = updated.(sessionModePicker)
	if !model.confirmed {
		t.Fatal("enter did not confirm the selection")
	}
	if got := model.selectedMode(); got != protocol.SessionModeGroup {
		t.Fatalf("selected mode = %q, want group", got)
	}
	if cmd == nil {
		t.Fatal("enter should quit the picker")
	}
}

func TestSessionModePicker_SupportsVimSelectionKeys(t *testing.T) {
	model := newSessionModePicker(80)

	for _, key := range []string{"j", "k"} {
		updated, _ := model.Update(tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune(key)}))
		model = updated.(sessionModePicker)
	}
	if model.selected != 0 {
		t.Fatalf("selected option after j/k = %d, want remote option 0", model.selected)
	}
}

func TestSessionModePicker_UnknownKeyDoesNothing(t *testing.T) {
	model := newSessionModePicker(80)
	updated, cmd := model.Update(tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune("x")}))
	model = updated.(sessionModePicker)
	if model.selected != 0 || model.confirmed || model.cancelled || cmd != nil {
		t.Fatalf("unknown key changed picker state: %+v, command = %v", model, cmd)
	}
}

func TestSessionModePicker_CancelQuitsWithoutConfirmation(t *testing.T) {
	model := newSessionModePicker(80)
	updated, cmd := model.Update(tea.KeyMsg(tea.Key{Type: tea.KeyEsc}))
	model = updated.(sessionModePicker)

	if model.confirmed {
		t.Fatal("escape should not confirm a mode")
	}
	if !model.cancelled {
		t.Fatal("escape should mark the picker as cancelled")
	}
	if cmd == nil {
		t.Fatal("escape should quit the picker")
	}
}

func TestHasExplicitSessionMode(t *testing.T) {
	for _, test := range []struct {
		args []string
		want bool
	}{
		{args: []string{"getsloth", "claude"}, want: false},
		{args: []string{"getsloth", "--remote", "claude"}, want: true},
		{args: []string{"getsloth", "--group", "claude"}, want: true},
		{args: []string{"getsloth", "--", "--group"}, want: false},
	} {
		if got := hasExplicitSessionMode(test.args); got != test.want {
			t.Errorf("hasExplicitSessionMode(%#v) = %v, want %v", test.args, got, test.want)
		}
	}
}

func TestShouldPromptForSessionModeRequiresInteractiveTerminals(t *testing.T) {
	input, err := os.CreateTemp(t.TempDir(), "input")
	if err != nil {
		t.Fatalf("create input: %v", err)
	}
	output, err := os.CreateTemp(t.TempDir(), "output")
	if err != nil {
		t.Fatalf("create output: %v", err)
	}

	if shouldPromptForSessionMode([]string{"getsloth", "claude"}, input, output) {
		t.Fatal("regular files should not enable the interactive picker")
	}
	if shouldPromptForSessionMode([]string{"getsloth", "--group", "claude"}, input, output) {
		t.Fatal("explicit group mode should skip the interactive picker")
	}

	t.Setenv("GETSLOTH_NO_TUI", "1")
	if shouldPromptForSessionMode([]string{"getsloth", "claude"}, input, output) {
		t.Fatal("GETSLOTH_NO_TUI should disable the interactive picker")
	}
}
