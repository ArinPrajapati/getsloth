package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	qrcode "github.com/skip2/go-qrcode"
)

// quietZoneModules is the light margin explicitly rendered around the
// code, replacing the go-qrcode library's default 4-module border. Zero
// margin measurably breaks decoding - verified with OpenCV's
// QRCodeDetector, which failed to read a zero-margin render even on a
// plain white background, let alone the terminal's own (often dark and
// therefore *wrong-colored* for a quiet zone) background. A 1-module
// margin decoded reliably in that same test and is what's used here -
// the founder explicitly chose the smaller-but-still-tested margin over
// the extra safety margin a 2-module version would give against pickier
// scanners this agent has no way to test against.
const quietZoneModules = 1

// renderQRCode returns a compact ASCII-art QR code for data, packing two
// QR rows into each terminal line with the Unicode upper-half-block glyph
// (▀): its foreground color paints the top module, its background color
// paints the bottom one. Colors are set explicitly via lipgloss (the same
// terminal-styling library used elsewhere in this codebase, so color
// capability / NO_COLOR detection stays consistent) when the terminal can
// render ANSI. An earlier version used the go-qrcode library's own half-block
// renderer, which leaves color to the terminal's defaults and corrupted
// visibly on at least one real terminal. ASCII/no-color terminals use the
// matching plain half-block glyphs instead of collapsing every cell to `▀`.
// Recovery level Low keeps the code physically as small as possible -
// session URLs are already long since they carry the host's auth public
// key in the fragment, per docs/protocol.md.
func renderQRCode(data string) (string, error) {
	return renderQRCodeWithRenderer(data, lipgloss.NewRenderer(os.Stderr))
}

func renderQRCodeWithRenderer(data string, renderer *lipgloss.Renderer) (string, error) {
	qr, err := qrcode.New(data, qrcode.Low)
	if err != nil {
		return "", fmt.Errorf("render QR code: %w", err)
	}
	// Must be set before the first Bitmap() call - go-qrcode caches the
	// built symbol on first use, so setting this afterward has no effect.
	qr.DisableBorder = true

	bits := padQuietZone(qr.Bitmap(), quietZoneModules)
	if renderer.ColorProfile() == termenv.Ascii {
		return renderPlainHalfBlocks(bits), nil
	}

	black := lipgloss.Color("0")
	white := lipgloss.Color("15")

	colorFor := func(dark bool) lipgloss.Color {
		if dark {
			return black
		}
		return white
	}

	var b strings.Builder
	for y := 0; y < len(bits); y += 2 {
		for x, top := range bits[y] {
			// The final row of an odd-height matrix has no paired row
			// below it - treat the missing bottom half as light, matching
			// the light quiet zone that borders every side of the code.
			bottom := false
			if y+1 < len(bits) {
				bottom = bits[y+1][x]
			}
			style := renderer.NewStyle().Foreground(colorFor(top)).Background(colorFor(bottom))
			b.WriteString(style.Render("▀"))
		}
		b.WriteString("\n")
	}
	return b.String(), nil
}

// renderPlainHalfBlocks preserves the QR matrix when ANSI colors are disabled.
// A colored upper-half block has two independently painted modules; these
// glyphs encode the same four combinations without relying on terminal state.
func renderPlainHalfBlocks(bits [][]bool) string {
	var b strings.Builder
	for y := 0; y < len(bits); y += 2 {
		for x, top := range bits[y] {
			bottom := false
			if y+1 < len(bits) {
				bottom = bits[y+1][x]
			}
			switch {
			case top && bottom:
				b.WriteRune('█')
			case top:
				b.WriteRune('▀')
			case bottom:
				b.WriteRune('▄')
			default:
				b.WriteRune(' ')
			}
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// padQuietZone returns a copy of bits surrounded by margin modules of
// light (false) padding on every side.
func padQuietZone(bits [][]bool, margin int) [][]bool {
	height := len(bits)
	width := len(bits[0])
	padded := make([][]bool, height+margin*2)
	for y := range padded {
		padded[y] = make([]bool, width+margin*2)
	}
	for y, row := range bits {
		copy(padded[y+margin][margin:margin+width], row)
	}
	return padded
}
