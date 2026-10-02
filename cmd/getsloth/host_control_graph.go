package main

import (
	"fmt"
	"strings"

	"github.com/arinprajapati/getsloth/internal/protocol"
	"github.com/charmbracelet/lipgloss"
)

type hostControlView int

const (
	hostControlViewGraph hostControlView = iota
	hostControlViewUsers
	hostControlViewJoin
	hostControlViewCount
)

const (
	// maxRTTHistory is the number of one-second samples kept per viewer.
	maxRTTHistory = 300
	// rttUnknown marks a sample taken while the relay had no RTT for a viewer.
	rttUnknown int64 = -1
	// minChartScaleMs keeps a near-zero link from filling the chart.
	minChartScaleMs int64 = 25
	// chartGutter is the width of the y-axis label column left of a plot.
	chartGutter = 7
)

var (
	chartBlocks    = []rune(" ▁▂▃▄▅▆▇█")
	chartGridStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("22"))
	brailleBits    = [4][2]uint8{{0x01, 0x08}, {0x02, 0x10}, {0x04, 0x20}, {0x40, 0x80}}
	viewerPalette  = []string{"46", "51", "220", "213", "208", "39"}
)

// chartTone is the bright line colour and dimmer fill colour for one sample.
type chartTone struct{ line, fill lipgloss.Style }

var (
	toneGood = chartTone{lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("46")), lipgloss.NewStyle().Foreground(lipgloss.Color("34"))}
	toneWarn = chartTone{lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("220")), lipgloss.NewStyle().Foreground(lipgloss.Color("136"))}
	toneBad  = chartTone{lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("203")), lipgloss.NewStyle().Foreground(lipgloss.Color("124"))}
)

// hostControlHistory holds the recent relay RTT samples per viewer ID so the
// dashboard can draw how each connection has been doing over time.
type hostControlHistory map[string][]int64

// record appends one sample per current viewer and forgets viewers that left.
func (h hostControlHistory) record(snapshot hostControlSnapshot) {
	present := make(map[string]bool, len(snapshot.Viewers))
	for _, viewer := range snapshot.Viewers {
		present[viewer.ID] = true
		sample := rttUnknown
		if viewer.RTTMs != nil {
			sample = *viewer.RTTMs
		}
		samples := append(h[viewer.ID], sample)
		if len(samples) > maxRTTHistory {
			samples = samples[len(samples)-maxRTTHistory:]
		}
		h[viewer.ID] = samples
	}
	for id := range h {
		if !present[id] {
			delete(h, id)
		}
	}
}

func chartToneFor(ms int64) chartTone {
	switch {
	case ms < 80:
		return toneGood
	case ms < 200:
		return toneWarn
	default:
		return toneBad
	}
}

func viewerColor(index int) lipgloss.Style {
	return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(viewerPalette[index%len(viewerPalette)]))
}

// renderRTTChart draws the newest `width` samples as a `rows`-high filled
// bar chart, newest on the right: a bright cap over solid fill, on a faint
// dotted grid. Unknown samples show a red cross on the baseline.
func renderRTTChart(samples []int64, width, rows int, scaleMs int64) []string {
	if width < 1 {
		width = 1
	}
	if rows < 1 {
		rows = 1
	}
	if len(samples) > width {
		samples = samples[len(samples)-width:]
	}
	pad := width - len(samples)
	lines := make([]string, rows)
	for r := 0; r < rows; r++ {
		grid := " "
		if rows >= 3 && (r == 0 || r == rows/2 || r == rows-1) {
			grid = chartGridStyle.Render("┈")
		}
		var b strings.Builder
		for i := 0; i < pad; i++ {
			b.WriteString(grid)
		}
		for _, ms := range samples {
			if ms < 0 {
				if r == rows-1 {
					b.WriteString(toneBad.line.Render("×"))
				} else {
					b.WriteString(grid)
				}
				continue
			}
			level := int((ms*int64(rows*8) + scaleMs - 1) / scaleMs)
			level = min(max(level, 1), rows*8)
			capRow := rows - 1 - (level-1)/8
			tone := chartToneFor(ms)
			switch {
			case r < capRow:
				b.WriteString(grid)
			case r == capRow:
				b.WriteString(tone.line.Render(string(chartBlocks[level-(rows-1-r)*8])))
			default:
				b.WriteString(tone.fill.Render("█"))
			}
		}
		lines[r] = b.String()
	}
	return lines
}

// renderOverlayChart plots every series as a braille line in its own colour on
// one shared scale, so connections can be compared at a glance. Each cell holds
// a 2x4 dot grid; consecutive samples are joined vertically so a spike reads as
// a continuous stroke.
func renderOverlayChart(series [][]int64, width, rows int, scaleMs int64) []string {
	dotsW, dotsH := width*2, rows*4
	bits := make([][]uint8, rows)
	owner := make([][]int, rows)
	for r := range bits {
		bits[r] = make([]uint8, width)
		owner[r] = make([]int, width)
	}
	for index, samples := range series {
		if len(samples) > dotsW {
			samples = samples[len(samples)-dotsW:]
		}
		prev := -1
		for k, ms := range samples {
			if ms < 0 {
				prev = -1
				continue
			}
			x := dotsW - len(samples) + k
			y := dotsH - 1 - int(ms*int64(dotsH-1)/scaleMs)
			y = min(max(y, 0), dotsH-1)
			lo, hi := y, y
			if prev >= 0 {
				lo, hi = min(prev, y), max(prev, y)
			}
			for dy := lo; dy <= hi; dy++ {
				bits[dy/4][x/2] |= brailleBits[dy%4][x%2]
				owner[dy/4][x/2] = index
			}
			prev = y
		}
	}
	lines := make([]string, rows)
	for r := range lines {
		grid := " "
		if rows >= 3 && (r == 0 || r == rows/2 || r == rows-1) {
			grid = chartGridStyle.Render("┈")
		}
		var b strings.Builder
		for c := 0; c < width; c++ {
			if bits[r][c] == 0 {
				b.WriteString(grid)
				continue
			}
			b.WriteString(viewerColor(owner[r][c]).Render(string(rune(0x2800 + int(bits[r][c])))))
		}
		lines[r] = b.String()
	}
	return lines
}

// withTimeAxis puts a millisecond gutter on the left of chart rows and a
// "-Ns … now" line underneath, assuming one sample per second per dot column.
func withTimeAxis(body []string, scaleMs int64, seconds, width int) []string {
	out := make([]string, 0, len(body)+1)
	last := len(body) - 1
	for r, line := range body {
		label := strings.Repeat(" ", chartGutter)
		switch {
		case r == 0:
			label = fmt.Sprintf("%4dms ", scaleMs)
		case r == last:
			label = "   0ms "
		case len(body) >= 3 && r == len(body)/2:
			label = fmt.Sprintf("%4dms ", scaleMs/2)
		}
		out = append(out, controlMutedStyle.Render(label)+line)
	}
	left := fmt.Sprintf("-%ds", seconds)
	gap := max(width-chartGutter-len(left)-3, 1)
	return append(out, controlMutedStyle.Render(strings.Repeat(" ", chartGutter)+left+strings.Repeat(" ", gap)+"now"))
}

// signalBars turns a round-trip time into a 0-4 bar signal-strength meter.
func signalBars(rtt *int64) string {
	if rtt == nil {
		return controlMutedStyle.Render("▂▄▆█")
	}
	strength := 1
	switch {
	case *rtt < 50:
		strength = 4
	case *rtt < 100:
		strength = 3
	case *rtt < 200:
		strength = 2
	}
	bars := []rune("▂▄▆█")
	tone := chartToneFor(*rtt)
	return tone.line.Render(string(bars[:strength])) + controlMutedStyle.Render(string(bars[strength:]))
}

func chartScale(series ...[]int64) int64 {
	scale := minChartScaleMs
	for _, samples := range series {
		for _, ms := range samples {
			scale = max(scale, ms)
		}
	}
	return (scale + 4) / 5 * 5
}

func rttStats(samples []int64) (minMs, avgMs, maxMs int64, ok bool) {
	var sum, count int64
	for _, ms := range samples {
		if ms < 0 {
			continue
		}
		if count == 0 || ms < minMs {
			minMs = ms
		}
		maxMs = max(maxMs, ms)
		sum += ms
		count++
	}
	if count == 0 {
		return 0, 0, 0, false
	}
	return minMs, sum / count, maxMs, true
}

// aggregateRTT averages the newest `n` samples across viewers, oldest first,
// for the dashboard's headline sparkline.
func aggregateRTT(history hostControlHistory, viewers []hostControlViewer, n int) []int64 {
	out := make([]int64, n)
	for i := range out {
		offset := n - 1 - i
		var sum, count int64
		for _, viewer := range viewers {
			samples := history[viewer.ID]
			if idx := len(samples) - 1 - offset; idx >= 0 && samples[idx] >= 0 {
				sum += samples[idx]
				count++
			}
		}
		out[i] = rttUnknown
		if count > 0 {
			out[i] = sum / count
		}
	}
	return out
}

func viewerQuality(viewer hostControlViewer) string {
	if viewer.Quality == "" {
		return protocol.ViewerQualityUnknown
	}
	return viewer.Quality
}
