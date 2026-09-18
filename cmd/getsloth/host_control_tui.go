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
	keybar := strings.Join([]string{
		controlKeyStyle.Render("[r]") + " RECLAIM",
		controlDangerStyle.Render("[k]") + " KILL VIEWERS",
		controlKeyStyle.Render("[i]") + " INVITE",
		controlMutedStyle.Render("[q] CLOSE"),
	}, "   ")

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
