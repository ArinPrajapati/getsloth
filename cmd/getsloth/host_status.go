package main

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"unicode"

	"github.com/arinprajapati/getsloth/internal/protocol"
)

type hostControlViewer struct {
	ID           string
	Name         string
	IsController bool
}

type hostControlSnapshot struct {
	Live           bool
	Mode           string
	InviteURL      string
	Password       string
	ControllerID   string
	ControllerRole string
	Viewers        []hostControlViewer
}

type hostSessionStatus struct {
	mu               sync.Mutex
	mode             string
	inviteURL        string
	password         string
	viewers          map[string]string
	activeWriterID   string
	activeWriterRole string
	live             bool
	out              io.Writer
}

func newHostSessionStatus(mode string, out io.Writer) *hostSessionStatus {
	status := &hostSessionStatus{
		mode:             mode,
		viewers:          make(map[string]string),
		activeWriterID:   "host",
		activeWriterRole: "host",
		live:             true,
		out:              out,
	}
	status.renderTitleLocked()
	return status
}

func (s *hostSessionStatus) setInvite(inviteURL, password string) {
	s.mu.Lock()
	s.inviteURL = inviteURL
	s.password = password
	s.mu.Unlock()
}

func (s *hostSessionStatus) updatePresence(msg protocol.PresenceMsg) {
	s.mu.Lock()
	s.viewers = make(map[string]string)
	for _, connection := range msg.Connections {
		if connection.Role == "viewer" {
			name := connection.DisplayName
			if name == "" {
				name = "viewer"
			}
			s.viewers[connection.ID] = safeTerminalLabel(name)
		}
		if connection.IsActiveWriter {
			s.activeWriterID = connection.ID
			s.activeWriterRole = connection.Role
		}
	}
	s.renderTitleLocked()
	s.mu.Unlock()
}

func (s *hostSessionStatus) updateControl(msg protocol.ControlChangedMsg) {
	s.mu.Lock()
	s.activeWriterID = msg.ActiveWriterID
	s.activeWriterRole = msg.ActiveWriterRole
	s.renderTitleLocked()
	s.mu.Unlock()
}

func (s *hostSessionStatus) setDisconnected() {
	s.mu.Lock()
	s.live = false
	s.renderTitleLocked()
	s.mu.Unlock()
}

func (s *hostSessionStatus) snapshot() hostControlSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()

	viewers := make([]hostControlViewer, 0, len(s.viewers))
	for id, name := range s.viewers {
		viewers = append(viewers, hostControlViewer{
			ID:           id,
			Name:         name,
			IsController: s.activeWriterRole == "viewer" && s.activeWriterID == id,
		})
	}
	sort.Slice(viewers, func(i, j int) bool {
		return viewers[i].Name < viewers[j].Name
	})

	return hostControlSnapshot{
		Live:           s.live,
		Mode:           s.mode,
		InviteURL:      s.inviteURL,
		Password:       s.password,
		ControllerID:   s.activeWriterID,
		ControllerRole: s.activeWriterRole,
		Viewers:        viewers,
	}
}

func (s *hostSessionStatus) summary() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.summaryLocked()
}

func (s *hostSessionStatus) print() {
	_, _ = fmt.Fprintf(s.out, "\r\ngetsloth: %s\r\n", s.summary())
}

func (s *hostSessionStatus) summaryLocked() string {
	liveLabel := "OFFLINE"
	if s.live {
		liveLabel = "LIVE"
	}

	viewerCount := len(s.viewers)
	viewerLabel := "viewers"
	if viewerCount == 1 {
		viewerLabel = "viewer"
	}

	controller := "host controls"
	if s.activeWriterRole == "viewer" {
		name := s.viewers[s.activeWriterID]
		if name == "" {
			name = "viewer"
		}
		controller = name + " controls"
	}

	return fmt.Sprintf("%s · %s · %d %s%s · %s", liveLabel, modeLabel(s.mode), viewerCount, viewerLabel, s.viewerNamesLocked(), controller)
}

func (s *hostSessionStatus) viewerNamesLocked() string {
	if len(s.viewers) == 0 {
		return ""
	}

	names := make([]string, 0, len(s.viewers))
	for _, name := range s.viewers {
		names = append(names, name)
		if len(names) == 3 {
			break
		}
	}

	return " (" + strings.Join(names, ", ") + ")"
}

func (s *hostSessionStatus) renderTitleLocked() {
	_, _ = fmt.Fprintf(s.out, "\x1b]0;getsloth • %s\x07", s.summaryLocked())
}

func modeLabel(mode string) string {
	if mode == protocol.SessionModeGroup {
		return "Group"
	}
	return "Remote"
}

func safeTerminalLabel(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, value)
}
