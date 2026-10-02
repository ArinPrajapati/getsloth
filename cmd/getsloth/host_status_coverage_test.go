package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/arinprajapati/getsloth/internal/protocol"
)

func TestHostStatusCoverage_BoundsActivityHistoryToNewestEvents(t *testing.T) {
	status := newHostSessionStatus(protocol.SessionModeRemote, &bytes.Buffer{})
	for index := 0; index < maxHostControlEvents+3; index++ {
		status.noteChat("viewer", fmt.Sprintf("event-%d", index))
	}

	events := status.snapshot().Events
	if len(events) != maxHostControlEvents {
		t.Fatalf("event history length = %d, want %d: %#v", len(events), maxHostControlEvents, events)
	}
	if !strings.Contains(events[0], "event-10") || !strings.Contains(events[len(events)-1], "event-3") {
		t.Fatalf("event history = %#v, want newest-first bounded history", events)
	}
	if strings.Contains(strings.Join(events, "\n"), "event-2") {
		t.Fatalf("event history retained an evicted event: %#v", events)
	}
}

func TestHostStatusCoverage_PresenceDefaultsAndOrdersJoinEvents(t *testing.T) {
	var out bytes.Buffer
	status := newHostSessionStatus(protocol.SessionModeRemote, &out)
	rtt := int64(17)
	status.updatePresence(protocol.PresenceMsg{Connections: []protocol.PresenceConnectionInfo{
		{ID: "host", Role: "host", IsActiveWriter: true},
		{ID: "zeta", Role: "viewer", DisplayName: "zeta", RTTMs: &rtt},
		{ID: "unnamed", Role: "viewer", RTTMs: &rtt},
		{ID: "alpha", Role: "viewer", DisplayName: "alpha"},
	}})

	snapshot := status.snapshot()
	if len(snapshot.Viewers) != 3 {
		t.Fatalf("viewers = %+v, want three viewers", snapshot.Viewers)
	}
	if snapshot.Viewers[0].Name != "alpha" || snapshot.Viewers[1].Name != "viewer" || snapshot.Viewers[2].Name != "zeta" {
		t.Fatalf("viewers = %+v, want name-sorted fallback viewer", snapshot.Viewers)
	}
	if snapshot.Viewers[1].Quality != protocol.ViewerQualityUnknown || snapshot.Viewers[1].RTTMs == nil || *snapshot.Viewers[1].RTTMs != 17 {
		t.Fatalf("fallback viewer health = %+v, want unknown/17ms", snapshot.Viewers[1])
	}
	if got := snapshot.Events; len(got) < 3 || got[0] != "zeta joined" || got[1] != "viewer joined" || got[2] != "alpha joined" {
		t.Fatalf("join events = %#v, want deterministic newest-first ordering", got)
	}
}

func TestHostStatusCoverage_ControlFallbackAndNoDuplicateEvent(t *testing.T) {
	status := newHostSessionStatus(protocol.SessionModeRemote, &bytes.Buffer{})
	status.updateControl(protocol.ControlChangedMsg{
		ActiveWriterID:   "unknown-viewer",
		ActiveWriterRole: "viewer",
	})
	first := status.snapshot().Events
	if len(first) != 1 || first[0] != "Viewer took control" {
		t.Fatalf("unknown viewer control event = %#v", first)
	}

	status.updateControl(protocol.ControlChangedMsg{
		ActiveWriterID:   "unknown-viewer",
		ActiveWriterRole: "viewer",
	})
	if got := status.snapshot().Events; len(got) != 1 {
		t.Fatalf("unchanged control state added an event: %#v", got)
	}
}

func TestHostStatusCoverage_SummaryLimitsViewerNamesAndSnapshotShowsDisconnect(t *testing.T) {
	status := newHostSessionStatus(protocol.SessionModeGroup, &bytes.Buffer{})
	status.updatePresence(protocol.PresenceMsg{Connections: []protocol.PresenceConnectionInfo{
		{ID: "a", Role: "viewer", DisplayName: "alpha"},
		{ID: "b", Role: "viewer", DisplayName: "bravo"},
		{ID: "c", Role: "viewer", DisplayName: "charlie"},
		{ID: "d", Role: "viewer", DisplayName: "delta"},
	}})

	summary := status.summary()
	namesInSummary := 0
	for _, name := range []string{"alpha", "bravo", "charlie", "delta"} {
		if strings.Contains(summary, name) {
			namesInSummary++
		}
	}
	if namesInSummary != 3 {
		t.Fatalf("summary = %q, want exactly three viewer names", summary)
	}

	status.setDisconnected()
	if snapshot := status.snapshot(); snapshot.Live {
		t.Fatalf("disconnected snapshot = %+v, want Live=false", snapshot)
	}
}
