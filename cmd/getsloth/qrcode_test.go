package main

import (
	"regexp"
	"strings"
	"testing"

	qrcode "github.com/skip2/go-qrcode"
)

// ansiSGR matches ANSI SGR escape sequences (e.g. "\x1b[48;5;0m") so tests
// can measure the visible module count per line independently of which
// color code (and therefore byte length) each module happened to use.
var ansiSGR = regexp.MustCompile("\x1b\\[[0-9;]*m")

func TestRenderQRCode(t *testing.T) {
	url := "https://getsloth.dev/s/x7k2#k=abc123"

	art, err := renderQRCode(url)
	if err != nil {
		t.Fatalf("renderQRCode returned error: %v", err)
	}

	if art == "" {
		t.Fatal("renderQRCode returned empty output")
	}

	lines := strings.Split(strings.TrimRight(art, "\n"), "\n")
	if len(lines) < 5 {
		t.Fatalf("expected a multi-line QR code, got %d line(s):\n%s", len(lines), art)
	}

	width := len([]rune(ansiSGR.ReplaceAllString(lines[0], "")))
	for i, line := range lines {
		visible := ansiSGR.ReplaceAllString(line, "")
		if got := len([]rune(visible)); got != width {
			t.Fatalf("line %d has visible width %d, want %d (QR output should be rectangular):\n%s", i, got, width, art)
		}
	}
}

func TestRenderQRCodePacksTwoRowsPerLine(t *testing.T) {
	// Regression guard for the compactness fix: two QR rows should share
	// one terminal line (via the half-block glyph), not one row per line -
	// otherwise the output is roughly twice as tall as it needs to be.
	qr, err := qrcode.New("https://getsloth.dev/s/x7k2#k=abc123", qrcode.Low)
	if err != nil {
		t.Fatalf("qrcode.New returned error: %v", err)
	}
	qr.DisableBorder = true                               // must match renderQRCode's own setting
	matrixHeight := len(qr.Bitmap()) + quietZoneModules*2 // renderQRCode pads the margin back in

	art, err := renderQRCode("https://getsloth.dev/s/x7k2#k=abc123")
	if err != nil {
		t.Fatalf("renderQRCode returned error: %v", err)
	}
	lines := strings.Split(strings.TrimRight(art, "\n"), "\n")

	wantLines := (matrixHeight + 1) / 2 // ceil(matrixHeight / 2)
	if len(lines) != wantLines {
		t.Fatalf("got %d rendered line(s) for a %d-row matrix, want %d (two QR rows per line)", len(lines), matrixHeight, wantLines)
	}
}

func TestRenderQRCodeShrinksBuiltInBorder(t *testing.T) {
	url := "https://getsloth.dev/s/x7k2#k=abc123"

	withBorder, err := qrcode.New(url, qrcode.Low)
	if err != nil {
		t.Fatalf("qrcode.New returned error: %v", err)
	}
	borderedHeight := len(withBorder.Bitmap())

	art, err := renderQRCode(url)
	if err != nil {
		t.Fatalf("renderQRCode returned error: %v", err)
	}
	lines := strings.Split(strings.TrimRight(art, "\n"), "\n")

	// The library's default border is a 4-module quiet zone per side (8
	// modules total per axis); renderQRCode replaces it with a
	// quietZoneModules-per-side margin of its own. Two rows pack into
	// each rendered line.
	netReductionPerAxis := (4 - quietZoneModules) * 2
	wantLines := (borderedHeight - netReductionPerAxis + 1) / 2
	if len(lines) != wantLines {
		t.Fatalf("got %d rendered line(s), want %d - renderQRCode should use a %d-module margin, not the library's default 4", len(lines), wantLines, quietZoneModules)
	}
}

func TestPadQuietZone(t *testing.T) {
	bits := [][]bool{
		{true, false},
		{false, true},
	}

	padded := padQuietZone(bits, 2)

	if len(padded) != 6 || len(padded[0]) != 6 {
		t.Fatalf("padded matrix is %dx%d, want 6x6", len(padded), len(padded[0]))
	}

	for y, row := range padded {
		for x, dark := range row {
			inOriginal := y >= 2 && y < 4 && x >= 2 && x < 4
			if !inOriginal && dark {
				t.Fatalf("padded[%d][%d] = true, want false (margin must be light)", y, x)
			}
		}
	}

	if padded[2][2] != true || padded[2][3] != false || padded[3][2] != false || padded[3][3] != true {
		t.Fatal("original bits were not copied to the correct offset within the padded matrix")
	}
}

func TestRenderQRCodeEmptyInput(t *testing.T) {
	// The library treats an empty payload as invalid content rather than
	// producing a degenerate code - renderQRCode should surface that as
	// an error, not panic or return blank output silently.
	if _, err := renderQRCode(""); err == nil {
		t.Fatal("expected an error for empty input, got nil")
	}
}
