package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	controlTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86"))
	controlLiveStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
	controlMutedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	controlButton     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("62")).Padding(0, 1)
	controlDanger     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("160")).Padding(0, 1)
	controlPanel      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("62")).Padding(0, 1)
)

func renderHostControlDashboard(snapshot hostControlSnapshot, width int) string {
	if width < 44 {
		width = 44
	}
	contentWidth := width - 4

	live := controlMutedStyle.Render("OFFLINE")
	if snapshot.Live {
		live = controlLiveStyle.Render("LIVE")
	}
	controller := "Host controls"
	if snapshot.ControllerRole == "viewer" {
		controller = "Viewer controls"
		for _, viewer := range snapshot.Viewers {
			if viewer.ID == snapshot.ControllerID {
				controller = viewer.Name + " controls"
				break
			}
		}
	}

	actions := lipgloss.JoinHorizontal(lipgloss.Top,
		controlButton.Render("[r] RECLAIM"), " ",
		controlDanger.Render("[k] KILL VIEWERS"), " ",
		controlButton.Render("[i] INVITE"), " ",
		controlButton.Render("[q] QUIT"),
	)
	header := lipgloss.NewStyle().Width(contentWidth).Render(actions)

	summary := strings.Join([]string{
		controlTitleStyle.Render("GETSLOTH CONTROL"),
		"",
		fmt.Sprintf("%-12s %s", "Session", live),
		fmt.Sprintf("%-12s %s", "Mode", modeLabel(snapshot.Mode)),
		fmt.Sprintf("%-12s %d connected", "Viewers", len(snapshot.Viewers)),
		fmt.Sprintf("%-12s %s", "Controller", controller),
		fmt.Sprintf("%-12s %s", "URL", truncateDashboardText(snapshot.InviteURL, contentWidth-12)),
		fmt.Sprintf("%-12s %s", "Password", truncateDashboardText(snapshot.Password, contentWidth-12)),
	}, "\n")

	viewerLines := []string{"VIEWERS"}
	if len(snapshot.Viewers) == 0 {
		viewerLines = append(viewerLines, controlMutedStyle.Render("No viewers connected"))
	}
	for _, viewer := range snapshot.Viewers {
		role := "watching"
		if viewer.IsController {
			role = "controlling"
		}
		viewerLines = append(viewerLines, truncateDashboardText("● "+viewer.Name+"  "+role, contentWidth-4))
	}

	eventLines := []string{"EVENTS"}
	if len(snapshot.Events) == 0 {
		eventLines = append(eventLines, controlMutedStyle.Render("Waiting for activity"))
	}
	for _, event := range snapshot.Events {
		eventLines = append(eventLines, truncateDashboardText("• "+event, contentWidth-4))
	}

	if contentWidth < 100 {
		viewers := controlPanel.Width(contentWidth - 2).Render(strings.Join(viewerLines, "\n"))
		events := controlPanel.Width(contentWidth - 2).Render(strings.Join(eventLines, "\n"))
		return strings.Join([]string{header, "", summary, "", viewers, events}, "\n")
	}

	panelWidth := contentWidth / 2
	viewers := controlPanel.Width(panelWidth - 2).Render(strings.Join(viewerLines, "\n"))
	events := controlPanel.Width(contentWidth - panelWidth - 2).Render(strings.Join(eventLines, "\n"))
	return strings.Join([]string{header, "", summary, "", lipgloss.JoinHorizontal(lipgloss.Top, viewers, " ", events)}, "\n")
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
