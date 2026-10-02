package main

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/arinprajapati/getsloth/internal/protocol"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const (
	consoleCardRows     = 4
	consoleActivityRows = 5
	consoleMinPanelRows = 5
	consoleMinPlotRows  = 6
	consoleHeaderRows   = 3
	consoleMinMainRows  = consoleMinPlotRows + consoleMinPanelRows
	consoleMinWidth     = 66
)

var controlBorderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("28"))

// titledPanel draws a box with its title embedded in the top border.
func titledPanel(title string, lines []string, width, innerRows int) string {
	inner := width - 4
	title = ansi.Truncate(title, max(width-6, 1), "…")
	dashes := max(width-5-lipgloss.Width(title), 0)
	bar := controlBorderStyle.Render("│")
	out := make([]string, 0, innerRows+2)
	out = append(out, controlBorderStyle.Render("┌─ ")+title+controlBorderStyle.Render(" "+strings.Repeat("─", dashes)+"┐"))
	for i := 0; i < innerRows; i++ {
		line := ""
		if i < len(lines) {
			line = lines[i]
		}
		out = append(out, bar+" "+padTo(line, inner)+" "+bar)
	}
	return strings.Join(append(out, controlBorderStyle.Render("└"+strings.Repeat("─", width-2)+"┘")), "\n")
}

func padTo(s string, width int) string {
	if gap := width - lipgloss.Width(s); gap > 0 {
		return s + strings.Repeat(" ", gap)
	}
	return ansi.Truncate(s, width, "")
}

func splitRows(total, parts int) []int {
	rows := make([]int, parts)
	for i := range rows {
		rows[i] = total / parts
		if i < total%parts {
			rows[i]++
		}
	}
	return rows
}

func inviteRowCount(showInvite bool) int {
	if showInvite {
		return 3
	}
	return 1
}

// consoleMinHeight is the shortest terminal the full layout fits in.
func consoleMinHeight(showInvite bool) int {
	return consoleHeaderRows + consoleCardRows + inviteRowCount(showInvite) + consoleActivityRows + consoleMinMainRows
}

// renderHostControlConsole draws the full-screen dashboard. Every row of the
// terminal is spent: stat cards and an activity strip are fixed, and the
// overlaid uplink chart and per-viewer panels (or viewer table) stretch to
// take whatever height remains.
func renderHostControlConsole(snapshot hostControlSnapshot, history hostControlHistory, width, height int, showInvite bool, view hostControlView) string {
	width = max(width, consoleMinWidth)
	height = max(height, consoleMinHeight(showInvite))

	live := controlMutedStyle.Render("OFFLINE")
	if snapshot.Live {
		live = controlLiveStyle.Render("● LIVE")
	}
	cursor := " "
	if time.Now().Second()%2 == 0 {
		cursor = "█"
	}
	title := controlAccentStyle.Render("GETSLOTH // HOST CONTROL") + controlLiveStyle.Render(cursor)
	header := title + strings.Repeat(" ", max(width-lipgloss.Width(title)-lipgloss.Width(live), 2)) + live

	tabLabels := []string{"[g] GRAPH", "[u] USERS", "[j] JOIN"}
	tabs := make([]string, len(tabLabels))
	for i, label := range tabLabels {
		tabs[i] = controlMutedStyle.Render(label)
		if hostControlView(i) == view {
			tabs[i] = controlAccentStyle.Render(label)
		}
	}
	tabBar := strings.Join(tabs, "   ") + controlMutedStyle.Render("   [tab] switch view")
	if view == hostControlViewJoin {
		return strings.Join([]string{header, renderHostControlKeybar(showInvite), tabBar, joinMain(snapshot, history, width, height-consoleHeaderRows, showInvite)}, "\n")
	}

	invite := []string{controlMutedStyle.Render("INVITE link ready • [i] to copy/show")}
	if showInvite {
		invite = []string{"url      " + snapshot.InviteURL, "password " + snapshot.Password, controlMutedStyle.Render("Copied to clipboard • [i] to hide")}
	}
	for i, line := range invite {
		invite[i] = padTo(line, width)
	}

	mainRows := height - consoleHeaderRows - consoleCardRows - len(invite) - consoleActivityRows
	var main string
	if view == hostControlViewGraph {
		main = graphMain(snapshot, history, width, mainRows)
	} else {
		main = usersMain(snapshot, history, width, mainRows)
	}

	parts := []string{header, renderHostControlKeybar(showInvite), tabBar, renderStatCards(snapshot, history, width), strings.Join(invite, "\n"), main, activityStrip(snapshot, width)}
	return strings.Join(parts, "\n")
}

func renderStatCards(snapshot hostControlSnapshot, history hostControlHistory, width int) string {
	var sum, known int64
	var worst int64 = -1
	worstName := "-"
	for _, viewer := range snapshot.Viewers {
		if viewer.RTTMs == nil {
			continue
		}
		sum += *viewer.RTTMs
		known++
		if *viewer.RTTMs > worst {
			worst, worstName = *viewer.RTTMs, viewer.Name
		}
	}
	rttText := func(value int64, ok bool) string {
		if !ok {
			return controlMutedStyle.Render("--")
		}
		return chartToneFor(value).line.Render(fmt.Sprintf("%dms", value))
	}

	controller := "HOST"
	if snapshot.ControllerRole == "viewer" {
		controller = "VIEWER"
		for _, viewer := range snapshot.Viewers {
			if viewer.ID == snapshot.ControllerID {
				controller = strings.ToUpper(viewer.Name)
			}
		}
	}
	link := controlMutedStyle.Render("OFFLINE")
	if snapshot.Live {
		link = controlLiveStyle.Render("ONLINE")
	}

	cardWidth := width / 5
	avgSeries := aggregateRTT(history, snapshot.Viewers, cardWidth-4)
	cards := []struct {
		title string
		lines []string
	}{
		{"VIEWERS", []string{controlAccentStyle.Render(fmt.Sprint(len(snapshot.Viewers))), controlMutedStyle.Render(strings.ToUpper(modeLabel(snapshot.Mode)) + " mode")}},
		{"AVG RTT", []string{rttText(sum/max(known, 1), known > 0), renderRTTChart(avgSeries, cardWidth-4, 1, chartScale(avgSeries))[0]}},
		{"WORST", []string{rttText(worst, known > 0), controlMutedStyle.Render(strings.ToUpper(worstName))}},
		{"CONTROL", []string{controlAccentStyle.Render(controller), controlMutedStyle.Render("[r] reclaim")}},
		{"UPLINK", []string{link, controlMutedStyle.Render("relay")}},
	}
	rendered := make([]string, len(cards))
	for i, card := range cards {
		w := cardWidth
		if i == len(cards)-1 {
			w = width - cardWidth*(len(cards)-1)
		}
		rendered[i] = titledPanel(card.title, card.lines, w, consoleCardRows-2)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, rendered...)
}

func activityStrip(snapshot hostControlSnapshot, width int) string {
	lines := []string{controlMutedStyle.Render("waiting for activity")}
	if len(snapshot.Events) > 0 {
		lines = lines[:0]
		for _, event := range snapshot.Events {
			lines = append(lines, "• "+event)
		}
	}
	return titledPanel("ACTIVITY", lines, width, consoleActivityRows-2)
}

func viewerLegend(viewers []hostControlViewer) string {
	parts := make([]string, len(viewers))
	for i, viewer := range viewers {
		parts[i] = viewerColor(i).Render("●") + " " + strings.ToUpper(truncateDashboardText(viewer.Name, 14))
	}
	return strings.Join(parts, "  ")
}

// overlayPanel is one chart with every viewer's RTT drawn on a shared scale.
func overlayPanel(snapshot hostControlSnapshot, history hostControlHistory, width, outerRows int) string {
	chartRows := outerRows - 3
	chartWidth := width - 4 - chartGutter
	series := make([][]int64, len(snapshot.Viewers))
	for i, viewer := range snapshot.Viewers {
		series[i] = history[viewer.ID]
	}
	scale := chartScale(series...)
	body := withTimeAxis(renderOverlayChart(series, chartWidth, chartRows, scale), scale, chartWidth*2, width-4)
	title := controlAccentStyle.Render("UPLINK RTT")
	if len(snapshot.Viewers) == 0 {
		hint := controlMutedStyle.Render("scanning for viewers… their uplink graph appears here")
		body[chartRows/2] = strings.Repeat(" ", chartGutter) + hint
	} else {
		title += "  " + viewerLegend(snapshot.Viewers)
	}
	return titledPanel(title, body, width, outerRows-2)
}

func graphMain(snapshot hostControlSnapshot, history hostControlHistory, width, rows int) string {
	if len(snapshot.Viewers) == 0 {
		return overlayPanel(snapshot, history, width, rows)
	}
	plotRows := max(rows*55/100, consoleMinPlotRows)
	panelRows := rows - plotRows
	if panelRows < consoleMinPanelRows {
		panelRows, plotRows = consoleMinPanelRows, rows-consoleMinPanelRows
	}
	return overlayPanel(snapshot, history, width, plotRows) + "\n" + viewerPanels(snapshot, history, width, panelRows)
}

// viewerPanels lays one filled-bar panel per viewer in a grid that uses the
// whole height, titled with the viewer's live numbers.
func viewerPanels(snapshot hostControlSnapshot, history hostControlHistory, width, rows int) string {
	perRow := min(max(width/36, 1), len(snapshot.Viewers))
	gridRows := min((len(snapshot.Viewers)+perRow-1)/perRow, rows/consoleMinPanelRows)
	viewers := snapshot.Viewers[:min(len(snapshot.Viewers), perRow*gridRows)]
	heights := splitRows(rows, gridRows)
	var out []string
	for g := 0; g < gridRows; g++ {
		start := g * perRow
		end := min(start+perRow, len(viewers))
		if start >= end {
			break
		}
		cells := make([]string, 0, perRow)
		count := end - start
		for i := start; i < end; i++ {
			w := width / count
			if i == end-1 {
				w = width - (width/count)*(count-1)
			}
			cells = append(cells, viewerPanel(viewers[i], i, history[viewers[i].ID], w, heights[g]))
		}
		out = append(out, lipgloss.JoinHorizontal(lipgloss.Top, cells...))
	}
	return strings.Join(out, "\n")
}

func viewerPanel(viewer hostControlViewer, index int, samples []int64, width, outerRows int) string {
	role := ""
	if viewer.IsController {
		role = controlLiveStyle.Render(" CTRL")
	}
	title := viewerColor(index).Render("●") + " " + strings.ToUpper(truncateDashboardText(viewer.Name, 16)) + role
	if viewer.RTTMs != nil {
		title += fmt.Sprintf("  %dms", *viewer.RTTMs)
	}
	title += " " + signalBars(viewer.RTTMs) + " " + strings.ToUpper(viewerQuality(viewer))
	return titledPanel(title, renderRTTChart(samples, width-4, outerRows-2, chartScale(samples)), width, outerRows-2)
}

// usersMain shows a table of viewers above the overlay chart.
func usersMain(snapshot hostControlSnapshot, history hostControlHistory, width, rows int) string {
	viewers := snapshot.Viewers
	tableRows := min(len(viewers), rows-consoleMinPlotRows-3)
	lines := []string{controlMutedStyle.Render(padTo("VIEWER", 18) + padTo("ROLE", 10) + padTo("SIG", 6) + padTo("RTT", 8) + padTo("QUALITY", 9) + padTo("MIN/AVG/MAX", 14) + "TREND")}
	if len(viewers) == 0 {
		lines = append(lines, controlMutedStyle.Render("waiting for a viewer to join — share the invite with [i]"))
	}
	trendWidth := width - 4 - 65
	for i := 0; i < tableRows; i++ {
		viewer := viewers[i]
		samples := history[viewer.ID]
		role := controlMutedStyle.Render("WATCH")
		if viewer.IsController {
			role = controlLiveStyle.Render("CTRL")
		}
		rtt := controlMutedStyle.Render("--")
		if viewer.RTTMs != nil {
			rtt = chartToneFor(*viewer.RTTMs).line.Render(fmt.Sprintf("%dms", *viewer.RTTMs))
		}
		stats := controlMutedStyle.Render("--")
		if lo, avg, hi, ok := rttStats(samples); ok {
			stats = fmt.Sprintf("%d/%d/%d", lo, avg, hi)
		}
		row := viewerColor(i).Render("●") + " " + padTo(strings.ToUpper(truncateDashboardText(viewer.Name, 16)), 16) +
			padTo(role, 10) + padTo(signalBars(viewer.RTTMs), 6) + padTo(rtt, 8) + padTo(strings.ToUpper(viewerQuality(viewer)), 9) + padTo(stats, 14)
		if trendWidth >= 8 {
			row += renderRTTChart(samples, trendWidth, 1, chartScale(samples))[0]
		}
		lines = append(lines, row)
	}
	tableOuter := max(len(lines), 2) + 2
	table := titledPanel(controlAccentStyle.Render("LIVE VIEWERS"), lines, width, tableOuter-2)
	return table + "\n" + overlayPanel(snapshot, history, width, rows-tableOuter)
}

var qrMemo struct {
	sync.Mutex
	data, rendered string
}

// cachedQRCode renders the QR once per URL: the dashboard redraws on every
// mouse movement, and a full QR is far too many styled cells to rebuild each time.
func cachedQRCode(data string) (string, error) {
	qrMemo.Lock()
	defer qrMemo.Unlock()
	if qrMemo.data == data && qrMemo.rendered != "" {
		return qrMemo.rendered, nil
	}
	rendered, err := renderQRCode(data)
	if err != nil {
		return "", err
	}
	qrMemo.data, qrMemo.rendered = data, rendered
	return rendered, nil
}

func wrapText(text string, width int) []string {
	var lines []string
	current := ""
	for _, word := range strings.Fields(text) {
		if current != "" && len(current)+1+len(word) > width {
			lines = append(lines, current)
			current = word
			continue
		}
		if current != "" {
			current += " "
		}
		current += word
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

func hardWrap(text string, width int) []string {
	var lines []string
	for len(text) > width && width > 0 {
		lines = append(lines, text[:width])
		text = text[width:]
	}
	return append(lines, text)
}

func joinInfoLines(snapshot hostControlSnapshot, width int, showInvite bool) []string {
	capacity := "GROUP mode: any number of viewers can join, early or late."
	if snapshot.Mode != protocol.SessionModeGroup {
		capacity = "REMOTE mode: one viewer at a time."
		if len(snapshot.Viewers) > 0 {
			capacity += " The seat is taken right now, so a second viewer is refused until it frees up."
		}
	}
	lines := wrapText(capacity, width)
	lines = append(lines, "")
	lines = append(lines, wrapText("1  Scan the QR with a phone camera. It carries the session key and password, so there is nothing to type.", width)...)
	lines = append(lines, "", controlAccentStyle.Render("2  Or open this link and enter the password:"))
	lines = append(lines, hardWrap(snapshot.InviteURL, width)...)
	password := controlMutedStyle.Render("••••••••  [i] to reveal")
	if showInvite {
		password = snapshot.Password
	}
	lines = append(lines, "", "password  "+password, controlMutedStyle.Render("[c] copies the link and password to your clipboard"))
	lines = append(lines, "", controlAccentStyle.Render(fmt.Sprintf("IN THE SESSION NOW (%d)", len(snapshot.Viewers))))
	if len(snapshot.Viewers) == 0 {
		lines = append(lines, controlMutedStyle.Render("nobody yet; the first scan shows up here"))
	}
	for i, viewer := range snapshot.Viewers {
		role := controlMutedStyle.Render("WATCH")
		if viewer.IsController {
			role = controlLiveStyle.Render("CTRL")
		}
		lines = append(lines, viewerColor(i).Render("●")+" "+padTo(strings.ToUpper(truncateDashboardText(viewer.Name, 18)), 19)+padTo(role, 7)+signalBars(viewer.RTTMs))
	}
	return lines
}

func eventLines(snapshot hostControlSnapshot) []string {
	if len(snapshot.Events) == 0 {
		return []string{controlMutedStyle.Render("waiting for activity")}
	}
	lines := make([]string, len(snapshot.Events))
	for i, event := range snapshot.Events {
		lines[i] = "• " + event
	}
	return lines
}

// joinMain gives late joiners everything in one place: the QR on the left
// and the link, password and seat availability beside it. When the QR does
// not fit it says so instead of drawing a code that cannot be scanned.
func joinMain(snapshot hostControlSnapshot, history hostControlHistory, width, rows int, showInvite bool) string {
	var qrLines []string
	if snapshot.QRInviteURL != "" {
		if qr, err := cachedQRCode(snapshot.QRInviteURL); err == nil {
			qrLines = strings.Split(strings.TrimRight(qr, "\n"), "\n")
		}
	}
	qrWidth := 0
	if len(qrLines) > 0 {
		qrWidth = lipgloss.Width(qrLines[0])
	}
	const minInfoWidth = 34
	if len(qrLines) == 0 || len(qrLines)+2 > rows || qrWidth+4+minInfoWidth > width {
		lines := joinInfoLines(snapshot, width-4, showInvite)
		if len(qrLines) > 0 {
			lines = append([]string{controlDangerStyle.Render(fmt.Sprintf("QR needs %d columns x %d rows; enlarge the terminal to show it.", qrWidth+4+minInfoWidth, len(qrLines)+2+consoleHeaderRows)), ""}, lines...)
		}
		return titledPanel(controlAccentStyle.Render("JOIN"), lines, width, rows-2)
	}
	qrOuter := len(qrLines) + 2
	left := titledPanel(controlAccentStyle.Render("SCAN TO JOIN"), qrLines, qrWidth+4, len(qrLines))
	if spare := rows - qrOuter; spare >= consoleMinPanelRows {
		left += "\n" + titledPanel(controlAccentStyle.Render("ACTIVITY"), eventLines(snapshot), qrWidth+4, spare-2)
	} else if spare > 0 {
		left = titledPanel(controlAccentStyle.Render("SCAN TO JOIN"), append(make([]string, spare/2), qrLines...), qrWidth+4, rows-2)
	}
	rightWidth := width - qrWidth - 4
	info := joinInfoLines(snapshot, rightWidth-4, showInvite)
	infoOuter := len(info) + 2
	right := titledPanel(controlAccentStyle.Render("JOIN"), info, rightWidth, rows-2)
	if spare := rows - infoOuter; spare >= consoleMinPlotRows && len(snapshot.Viewers) > 0 {
		right = titledPanel(controlAccentStyle.Render("JOIN"), info, rightWidth, len(info)) + "\n" + overlayPanel(snapshot, history, rightWidth, spare)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}
