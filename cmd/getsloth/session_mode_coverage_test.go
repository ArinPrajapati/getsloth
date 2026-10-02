package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/arinprajapati/getsloth/internal/protocol"
	tea "github.com/charmbracelet/bubbletea"
)

type modePickerFailingReader struct {
	err error
}

func (r modePickerFailingReader) Read([]byte) (int, error) {
	return 0, r.err
}

func TestChooseSessionMode_ReportsInputFailure(t *testing.T) {
	want := errors.New("mode picker input disconnected")
	var output bytes.Buffer
	mode, confirmed, err := chooseSessionMode(modePickerFailingReader{err: want}, &output)
	if !errors.Is(err, want) {
		t.Fatalf("input failure = %v, want %v", err, want)
	}
	if mode != "" || confirmed {
		t.Fatalf("input failure confirmed mode %q", mode)
	}
}

func TestChooseSessionMode_KeyboardSelectionAndCancellation(t *testing.T) {
	for _, test := range []struct {
		name      string
		input     string
		mode      string
		confirmed bool
	}{
		{name: "confirm default remote", input: "\r", mode: protocol.SessionModeRemote, confirmed: true},
		{name: "select group", input: "j\r", mode: protocol.SessionModeGroup, confirmed: true},
		{name: "cancel selected group", input: "\x1b[Bq"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			mode, confirmed, err := chooseSessionMode(strings.NewReader(test.input), &output)
			if err != nil {
				t.Fatalf("chooseSessionMode: %v", err)
			}
			if mode != test.mode || confirmed != test.confirmed {
				t.Fatalf("selection = %q/%t, want %q/%t", mode, confirmed, test.mode, test.confirmed)
			}
			if !strings.Contains(output.String(), "REMOTE") || !strings.Contains(output.String(), "GROUP") {
				t.Fatalf("mode choices were not rendered: %q", output.String())
			}
		})
	}
}

func TestSessionModePicker_SelectionStaysWithinChoices(t *testing.T) {
	model := newSessionModePicker(80)
	for range 3 {
		updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyUp})
		model = updated.(sessionModePicker)
	}
	if model.selectedMode() != protocol.SessionModeRemote {
		t.Fatal("moving above the first choice changed the selected mode")
	}
	for range 3 {
		updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
		model = updated.(sessionModePicker)
	}
	if model.selectedMode() != protocol.SessionModeGroup {
		t.Fatal("moving below the last choice changed the selected mode")
	}
}

func TestSessionModePicker_IgnoresUnavailableWindowDimensions(t *testing.T) {
	model := newSessionModePicker(80)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 48, Height: 24})
	model = updated.(sessionModePicker)
	updated, _ = model.Update(tea.WindowSizeMsg{})
	model = updated.(sessionModePicker)
	if model.width != 48 || model.height != 24 {
		t.Fatalf("unavailable dimensions replaced the last known size: %+v", model)
	}
}

func TestShouldPromptForSessionMode_RejectsMissingOrDumbTerminal(t *testing.T) {
	t.Setenv("GETSLOTH_NO_TUI", "")
	t.Setenv("TERM", "xterm")
	if shouldPromptForSessionMode([]string{"getsloth"}, nil, nil) {
		t.Fatal("missing terminal handles enabled the mode picker")
	}
	t.Setenv("TERM", "dumb")
	if shouldPromptForSessionMode([]string{"getsloth"}, nil, nil) {
		t.Fatal("dumb terminal enabled the mode picker")
	}
}
