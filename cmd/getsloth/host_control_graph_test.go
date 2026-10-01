package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/arinprajapati/getsloth/internal/protocol"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func rtt(v int64) *int64 { return &v }

func graphSnapshot(viewers ...hostControlViewer) hostControlSnapshot {
	return hostControlSnapshot{
		Live:           true,
		Mode:           protocol.SessionModeRemote,
		InviteURL:      "https://example.test/s/abc",
		Password:       "sloth-demo",
		ControllerRole: "host",
		Viewers:        viewers,
		Events:         []string{"Phone joined"},
	}
}

func TestHostControlHistory_RecordsRealSamplesAndForgetsLeftViewers(t *testing.T) {
	history := hostControlHistory{}
	history.record(graphSnapshot(hostControlViewer{ID: "a", RTTMs: rtt(40)}, hostControlViewer{ID: "b"}))
	history.record(graphSnapshot(hostControlViewer{ID: "a", RTTMs: rtt(55)}))

	if got := history["a"]; len(got) != 2 || got[0] != 40 || got[1] != 55 {
		t.Errorf("history[a] = %v, want [40 55]", got)
	}
	if _, ok := history["b"]; ok {
		t.Error("history kept a viewer that left")
	}
}

func TestHostControlHistory_CapsSamples(t *testing.T) {
	history := hostControlHistory{}
	for i := 0; i < maxRTTHistory+20; i++ {
		history.record(graphSnapshot(hostControlViewer{ID: "a", RTTMs: rtt(int64(i))}))
	}
	if got := len(history["a"]); got != maxRTTHistory {
		t.Errorf("len(history[a]) = %d, want %d", got, maxRTTHistory)
	}
	if history["a"][maxRTTHistory-1] != int64(maxRTTHistory+19) {
		t.Error("newest sample was not kept")
	}
}

func TestHostControlHistory_UnknownRTTRecordedAsGap(t *testing.T) {
	history := hostControlHistory{}
	history.record(graphSnapshot(hostControlViewer{ID: "a"}))
	if got := history["a"]; len(got) != 1 || got[0] != rttUnknown {
		t.Errorf("history[a] = %v, want [%d]", got, rttUnknown)
	}
}

func TestRenderRTTChart_ShapeAndScale(t *testing.T) {
	lines := renderRTTChart([]int64{10, 50, 100, rttUnknown}, 8, 3, 100)
	if len(lines) != 3 {
		t.Fatalf("got %d rows, want 3", len(lines))
	}
	for i, line := range lines {
		if got := lipgloss.Width(line); got != 8 {
			t.Errorf("row %d width = %d, want 8", i, got)
		}
	}
	if !strings.Contains(lines[0], "█") {
		t.Errorf("a full-scale sample should reach the top row: %q", lines[0])
	}
	if !strings.Contains(lines[2], "×") {
		t.Errorf("unknown sample should leave a baseline cross: %q", lines[2])
	}
}

func TestRTTStats(t *testing.T) {
	lo, avg, hi, ok := rttStats([]int64{10, rttUnknown, 30})
	if !ok || lo != 10 || avg != 20 || hi != 30 {
		t.Errorf("rttStats = %d %d %d %v, want 10 20 30 true", lo, avg, hi, ok)
	}
	if _, _, _, ok := rttStats([]int64{rttUnknown}); ok {
		t.Error("rttStats with only unknown samples should report !ok")
	}
}

func TestRenderHostControlConsole_FillsHeightAndWidth(t *testing.T) {
	cases := []struct{ width, height, viewers int }{
		{66, 24, 0}, {80, 24, 0}, {80, 24, 1}, {80, 30, 3}, {120, 40, 2}, {72, 30, 5}, {140, 50, 8}, {100, 12, 2},
	}
	for _, view := range []hostControlView{hostControlViewGraph, hostControlViewUsers, hostControlViewJoin} {
		for _, showInvite := range []bool{false, true} {
			for _, tc := range cases {
				viewers := make([]hostControlViewer, tc.viewers)
				history := hostControlHistory{}
				for i := range viewers {
					viewers[i] = hostControlViewer{ID: string(rune('a' + i)), Name: "Viewer", Quality: protocol.ViewerQualityGood, RTTMs: rtt(int64(10 + i*30))}
				}
				snapshot := graphSnapshot(viewers...)
				for i := 0; i < 5; i++ {
					history.record(snapshot)
				}

				out := renderHostControlConsole(snapshot, history, tc.width, tc.height, showInvite, view)
				rows := strings.Split(out, "\n")
				if want := max(tc.height, consoleMinHeight(showInvite)); len(rows) != want {
					t.Errorf("view %d invite=%v %+v: got %d rows, want %d", view, showInvite, tc, len(rows), want)
				}
				for _, row := range rows {
					if got := lipgloss.Width(row); got > max(tc.width, consoleMinWidth) {
						t.Errorf("view %d invite=%v %+v: row width %d too wide: %q", view, showInvite, tc, got, row)
					}
				}
			}
		}
	}
}

func TestRenderOverlayChart_DrawsEachSeriesInItsOwnCell(t *testing.T) {
	lines := renderOverlayChart([][]int64{{25, 25, 25, 25}, {0, 0, 0, 0}}, 4, 3, 25)
	if len(lines) != 3 {
		t.Fatalf("got %d rows, want 3", len(lines))
	}
	if !strings.ContainsAny(lines[0], "\u2801\u2808\u2809") {
		t.Errorf("a full-scale series should draw on the top row: %q", lines[0])
	}
	if !strings.ContainsAny(lines[2], "\u2840\u2880\u28c0") {
		t.Errorf("a zero series should draw on the bottom row: %q", lines[2])
	}
}

func TestAggregateRTT_AveragesAcrossViewersAndMarksGaps(t *testing.T) {
	history := hostControlHistory{"a": {10, 20}, "b": {30, rttUnknown}}
	viewers := []hostControlViewer{{ID: "a"}, {ID: "b"}}
	got := aggregateRTT(history, viewers, 3)
	want := []int64{rttUnknown, 20, 20}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("aggregateRTT[%d] = %d, want %d (all: %v)", i, got[i], want[i], got)
		}
	}
}

func TestTitledPanel_KeepsExactWidthWithStyledTitle(t *testing.T) {
	panel := titledPanel(controlAccentStyle.Render("A VERY LONG TITLE THAT MUST BE CUT"), []string{"line", strings.Repeat("x", 80)}, 30, 3)
	rows := strings.Split(panel, "\n")
	if len(rows) != 5 {
		t.Fatalf("got %d rows, want 5", len(rows))
	}
	for _, row := range rows {
		if got := lipgloss.Width(row); got != 30 {
			t.Errorf("row width = %d, want 30: %q", got, row)
		}
	}
}

func TestRenderHostControlConsole_UsersViewListsViewersWithStats(t *testing.T) {
	history := hostControlHistory{}
	snapshot := graphSnapshot(hostControlViewer{ID: "a", Name: "Phone", Quality: protocol.ViewerQualityGood, RTTMs: rtt(42), IsController: true})
	history.record(snapshot)
	out := renderHostControlConsole(snapshot, history, 110, 30, false, hostControlViewUsers)
	for _, want := range []string{"LIVE VIEWERS", "PHONE", "CTRL", "42ms", "GOOD", "42/42/42"} {
		if !strings.Contains(out, want) {
			t.Errorf("users view missing %q", want)
		}
	}
}

func TestRenderHostControlConsole_RevealsInviteOnlyWhenRequested(t *testing.T) {
	snapshot := graphSnapshot()
	if out := renderHostControlConsole(snapshot, hostControlHistory{}, 100, 30, false, hostControlViewGraph); strings.Contains(out, "sloth-demo") {
		t.Error("password revealed without request")
	}
	if out := renderHostControlConsole(snapshot, hostControlHistory{}, 100, 30, true, hostControlViewGraph); !strings.Contains(out, "sloth-demo") {
		t.Error("password not shown when requested")
	}
}

func TestHostControlTUI_ViewKeysSwitchBetweenGraphAndUsers(t *testing.T) {
	model := newHostControlTUI("/tmp/none.sock", &bytes.Buffer{}, graphSnapshot())
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model = updated.(hostControlTUI)
	if model.view != hostControlViewGraph {
		t.Fatal("graph should be the default view")
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = updated.(hostControlTUI)
	if model.view != hostControlViewUsers || !strings.Contains(model.View(), "LIVE VIEWERS") {
		t.Error("tab should switch to the users view")
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	if updated.(hostControlTUI).view != hostControlViewGraph {
		t.Error("g should select the graph view")
	}
}

func TestHostControlTUI_SnapshotMessagesFeedHistory(t *testing.T) {
	model := newHostControlTUI("/tmp/none.sock", &bytes.Buffer{}, hostControlSnapshot{})
	updated, _ := model.Update(hostControlSnapshotMsg{snapshot: graphSnapshot(hostControlViewer{ID: "a", RTTMs: rtt(33)})})
	if got := updated.(hostControlTUI).history["a"]; len(got) != 1 || got[0] != 33 {
		t.Errorf("history[a] = %v, want [33]", got)
	}
}

func joinSnapshot(mode string, viewers ...hostControlViewer) hostControlSnapshot {
	snapshot := graphSnapshot(viewers...)
	snapshot.Mode = mode
	snapshot.QRInviteURL = "https://example.test/s/abc#k=AAAA&p=sloth-demo"
	return snapshot
}

func TestRenderHostControlConsole_JoinViewShowsQRAndInviteDetails(t *testing.T) {
	out := renderHostControlConsole(joinSnapshot(protocol.SessionModeGroup), hostControlHistory{}, 110, 40, false, hostControlViewJoin)
	for _, want := range []string{"SCAN TO JOIN", "JOIN", "any number of viewers", "https://example.test/s/abc", "[i] to reveal", "▀"} {
		if !strings.Contains(out, want) {
			t.Errorf("join view missing %q", want)
		}
	}
	if strings.Contains(out, "sloth-demo") {
		t.Error("join view printed the password without a reveal")
	}
	if got := len(strings.Split(out, "\n")); got != 40 {
		t.Errorf("join view has %d rows, want 40", got)
	}
}

func TestRenderHostControlConsole_JoinViewRevealsPasswordOnRequest(t *testing.T) {
	out := renderHostControlConsole(joinSnapshot(protocol.SessionModeGroup), hostControlHistory{}, 110, 40, true, hostControlViewJoin)
	if !strings.Contains(out, "password  sloth-demo") {
		t.Error("join view should show the password once revealed")
	}
}

func TestRenderHostControlConsole_JoinViewWarnsWhenRemoteSeatIsTaken(t *testing.T) {
	snapshot := joinSnapshot(protocol.SessionModeRemote, hostControlViewer{ID: "a", Name: "Phone"})
	out := renderHostControlConsole(snapshot, hostControlHistory{}, 130, 40, false, hostControlViewJoin)
	if !strings.Contains(out, "REMOTE mode") || !strings.Contains(out, "seat is taken") {
		t.Errorf("remote join view should warn the seat is taken: %q", out)
	}
}

func TestRenderHostControlConsole_JoinViewExplainsWhenTerminalTooSmallForQR(t *testing.T) {
	snapshot := joinSnapshot(protocol.SessionModeGroup)
	snapshot.QRInviteURL = qrShareURL("https://sloth.arin.work", "-Oa9a0bNFdDV", strings.Repeat("A", 44), "sloth-demo-password")
	out := renderHostControlConsole(snapshot, hostControlHistory{}, 80, 24, false, hostControlViewJoin)
	if !strings.Contains(out, "enlarge the terminal") {
		t.Error("a QR that cannot fit must say so rather than render unscannable")
	}
	if !strings.Contains(out, "https://example.test/s/abc") {
		t.Error("the plain link must still be shown when the QR does not fit")
	}
}

func TestHostControlTUI_TabCyclesThroughJoinAndJKeySelectsIt(t *testing.T) {
	model := newHostControlTUI("/tmp/none.sock", &bytes.Buffer{}, graphSnapshot())
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	model = updated.(hostControlTUI)
	if model.view != hostControlViewJoin {
		t.Fatal("j should select the join view")
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})
	if updated.(hostControlTUI).view != hostControlViewGraph {
		t.Error("tab from join should wrap to the graph view")
	}
}

func TestHostSessionStatus_SnapshotCarriesQRInvite(t *testing.T) {
	status := newHostSessionStatus(protocol.SessionModeGroup, &bytes.Buffer{})
	status.setQRInvite("https://example.test/s/x#k=k&p=p")
	if got := status.snapshot().QRInviteURL; got != "https://example.test/s/x#k=k&p=p" {
		t.Errorf("snapshot QRInviteURL = %q", got)
	}
}

func TestRenderHostControlConsole_JoinViewShowsUplinkChartWhenSpaceAllows(t *testing.T) {
	history := hostControlHistory{}
	snapshot := joinSnapshot(protocol.SessionModeGroup, hostControlViewer{ID: "a", Name: "Phone", RTTMs: rtt(30)})
	history.record(snapshot)
	out := renderHostControlConsole(snapshot, history, 130, 56, false, hostControlViewJoin)
	if !strings.Contains(out, "UPLINK RTT") {
		t.Error("a tall join view should spend spare rows on the uplink chart")
	}
	if got := len(strings.Split(out, "\n")); got != 56 {
		t.Errorf("join view has %d rows, want 56", got)
	}
}
