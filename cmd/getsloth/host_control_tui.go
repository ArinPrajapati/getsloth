package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	controlAccentStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("51"))
	controlLiveStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
	controlMutedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	controlKeyStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("51"))
	controlDangerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("203"))
	controlPanel       = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("240")).Padding(0, 1)
)

// hostControlButtonRegion is a clickable keybar button's key and the
// terminal columns it occupies on the keybar row (row index 1 in the
// rendered dashboard), so mouse clicks can be mapped back to a key action.
type hostControlButtonRegion struct {
	Key      byte
	StartCol int
	EndCol   int // exclusive
}

func hostControlKeybarButtons(showInvite bool) []struct {
	key      byte
	rendered string
} {
	inviteText := "INVITE"
	if showInvite {
		inviteText = "HIDE"
	}
	return []struct {
		key      byte
		rendered string
	}{
		{'r', controlKeyStyle.Render("[r]") + " RECLAIM"},
		{'k', controlDangerStyle.Render("[k]") + " KILL VIEWERS"},
		{'i', controlKeyStyle.Render("[i]") + " " + inviteText},
		{'c', controlKeyStyle.Render("[c]") + " COPY"},
		{'q', controlMutedStyle.Render("[q] CLOSE")},
	}
}

const hostControlKeybarGap = 3

func renderHostControlKeybar(showInvite bool) string {
	buttons := hostControlKeybarButtons(showInvite)
	rendered := make([]string, len(buttons))
	for idx, button := range buttons {
		rendered[idx] = button.rendered
	}
	return strings.Join(rendered, strings.Repeat(" ", hostControlKeybarGap))
}

// hostControlKeybarRegions computes the clickable column range for each
// keybar button, in the same order and spacing renderHostControlKeybar
// uses, so a mouse click's X coordinate can be matched to a button.
func hostControlKeybarRegions(showInvite bool) []hostControlButtonRegion {
	buttons := hostControlKeybarButtons(showInvite)
	regions := make([]hostControlButtonRegion, 0, len(buttons))
	col := 0
	for idx, button := range buttons {
		if idx > 0 {
			col += hostControlKeybarGap
		}
		width := lipgloss.Width(button.rendered)
		regions = append(regions, hostControlButtonRegion{Key: button.key, StartCol: col, EndCol: col + width})
		col += width
	}
	return regions
}

func renderHostControlDashboard(snapshot hostControlSnapshot, width int, showInvite bool) string {
	if width < 48 {
		width = 48
	}
	contentWidth := width - 2

	live := controlMutedStyle.Render("OFFLINE")
	if snapshot.Live {
		live = controlLiveStyle.Render("● LIVE")
	}
	header := lipgloss.JoinHorizontal(lipgloss.Top,
		controlAccentStyle.Render("GETSLOTH // HOST CONTROL"),
		strings.Repeat(" ", maxDashboardWidth(contentWidth-42, 2)),
		live,
	)
	keybar := renderHostControlKeybar(showInvite)

	controller := "host"
	if snapshot.ControllerRole == "viewer" {
		controller = "viewer"
		for _, viewer := range snapshot.Viewers {
			if viewer.ID == snapshot.ControllerID {
				controller = viewer.Name
				break
			}
		}
	}
	sessionLines := []string{
		controlAccentStyle.Render("SESSION"),
		fmt.Sprintf("state       %s", live),
		fmt.Sprintf("mode        %s", modeLabel(snapshot.Mode)),
		fmt.Sprintf("controller  %s", controller),
	}
	if showInvite {
		sessionLines = append(sessionLines,
			fmt.Sprintf("url         %s", snapshot.InviteURL),
			fmt.Sprintf("password    %s", snapshot.Password),
			controlMutedStyle.Render("Copied to clipboard • [i] to hide"),
		)
	} else {
		sessionLines = append(sessionLines, controlMutedStyle.Render("Invite link ready • [i] to copy/show"))
	}

	viewerLines := []string{controlAccentStyle.Render("LIVE VIEWERS")}
	if len(snapshot.Viewers) == 0 {
		viewerLines = append(viewerLines, controlMutedStyle.Render("waiting for a viewer to join"))
	}
	for _, viewer := range snapshot.Viewers {
		role := "watching"
		marker := controlMutedStyle.Render("○")
		if viewer.IsController {
			role = "controlling"
			marker = controlLiveStyle.Render("●")
		}
		viewerLines = append(viewerLines, marker+" "+truncateDashboardText(viewer.Name, 28)+"  "+role)
	}

	eventLines := []string{controlAccentStyle.Render("ACTIVITY")}
	if len(snapshot.Events) == 0 {
		eventLines = append(eventLines, controlMutedStyle.Render("waiting for activity"))
	}
	for _, event := range snapshot.Events {
		eventLines = append(eventLines, "• "+event)
	}

	var body string
	if contentWidth >= 100 {
		leftWidth := 36
		rightWidth := contentWidth - leftWidth - 3
		session := controlPanel.Width(leftWidth - 2).Render(fitDashboardLines(sessionLines, leftWidth-4))
		viewers := controlPanel.Width(rightWidth - 2).Render(fitDashboardLines(viewerLines, rightWidth-4))
		activity := controlPanel.Width(contentWidth - 2).Render(fitDashboardLines(eventLines, contentWidth-4))
		body = strings.Join([]string{lipgloss.JoinHorizontal(lipgloss.Top, session, " ", viewers), activity}, "\n")
	} else {
		session := controlPanel.Width(contentWidth - 2).Render(fitDashboardLines(sessionLines, contentWidth-4))
		viewers := controlPanel.Width(contentWidth - 2).Render(fitDashboardLines(viewerLines, contentWidth-4))
		activity := controlPanel.Width(contentWidth - 2).Render(fitDashboardLines(eventLines, contentWidth-4))
		body = strings.Join([]string{session, viewers, activity}, "\n")
	}

	return strings.Join([]string{header, keybar, "", body}, "\n")
}

func fitDashboardLines(lines []string, width int) string {
	fitted := make([]string, 0, len(lines))
	for _, line := range lines {
		fitted = append(fitted, truncateDashboardText(line, width))
	}
	return strings.Join(fitted, "\n")
}

func truncateDashboardText(value string, width int) string {
	if width <= 0 || lipgloss.Width(value) <= width {
		return value
	}
	if width <= 1 {
		return "…"
	}
	var builder strings.Builder
	for _, runeValue := range value {
		if lipgloss.Width(builder.String()+string(runeValue)+"…") > width {
			break
		}
		builder.WriteRune(runeValue)
	}
	return builder.String() + "…"
}

func maxDashboardWidth(value, minimum int) int {
	if value < minimum {
		return minimum
	}
	return value
}
