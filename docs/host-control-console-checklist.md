# Host Control Console Checklist

## Goal

When a getsloth session starts, the host should get a separate, host-only control console in a new terminal window. The main terminal remains the shared PTY for shell/Claude/Codex/Vim/tmux/etc. The control console is the live status and action surface.

This replaces stale one-time startup status like `LIVE · Remote · 0 viewers · host controls` with a real, updating UI.

## Product decision

- Open the Host Control Console by default for every getsloth session.
- Do not implement a tmux-style status bar inside the shared session.
- Do not draw persistent status inside the wrapped PTY.
- Keep main-session shortcuts for urgent actions.
- If the console cannot open, fail gracefully and keep the session usable.

## Target UI direction

Terminal TUI with visible action buttons, keyboard shortcuts, mouse/click support where terminals support it, and a compact dashboard layout.

**Visual language decision:** use a dense, terminal-native monitor inspired by
`btop`/`lazygit`, not a web-dashboard imitation. A compact header/keybar leads;
session and live-viewer state occupy the primary row; activity is a full-width
stream below. Long invite credentials must never dominate or overflow the UI.

Example shape:

```text
┌────────────────────────────────────────────────────────────────────────┐
│ [r] reclaim  [k] kill viewers  [i] invite  [c] copy  [q] quit console │
└────────────────────────────────────────────────────────────────────────┘

GETSLOTH CONTROL

Session     LIVE
Mode        Remote
Viewers     2 connected
Controller  alice
URL         https://sloth.arin.work/s/...
Password    9UGRUL7Xnb

┌─ Viewers ─────────────────────────────────────────────────────────────┐
│ alice  controller  42ms   ▁▂▃▄▅▆▅  good                              │
│ bob    watching    180ms  ▁▁▂▁▃▂▁  laggy                             │
└───────────────────────────────────────────────────────────────────────┘

┌─ Events ──────────────────────────────────────────────────────────────┐
│ 12:41 bob joined                                                      │
│ 12:42 alice took control                                              │
│ 12:43 host reclaimed control                                          │
└───────────────────────────────────────────────────────────────────────┘
```

## Required controls

- [x] `[r] reclaim` / take host control back from the active viewer.
- [x] `[k] kill viewers` / disconnect all viewers while keeping the host session alive.
- [x] `[i] invite` / show invite URL and password clearly (toggles reveal; hidden by default).
- [x] `[c] copy` / copy invite details when clipboard support is available (OSC52; independent of the `[i]` reveal toggle).
- [x] `[q] quit console` / close only the control console, not the shared session (`tea.Quit` stops only this program).
- [x] Mouse click support for visible buttons when the terminal supports mouse events.
- [x] Keyboard fallback for every clickable control (mouse clicks route through the same key-action handler).

## Main session behavior

- [x] Keep the shared PTY clean: no persistent status bar inside it.
- [x] Remove or replace stale one-time live status snapshots.
- [x] Startup output should say the control console opened (`main.go`: "host control console opened in a separate Terminal window").
- [x] Keep lightweight escape shortcuts in the main session:
  - [x] `Ctrl-] r` reclaim control.
  - [x] `Ctrl-] i` print current live status.
- [x] If the control console fails to open, print a clear fallback message naming both `Ctrl-] i` and `Ctrl-] r`.

## Live status data

- [x] Session state: live/offline/ending.
- [x] Session mode: Remote or Group.
- [x] Public URL (hidden by default; reveal with `[i]`).
- [x] Password (hidden by default; reveal with `[i]`).
- [x] Viewer count (implicit in the viewer list length; no separate counter).
- [x] Viewer list.
- [x] Active controller: host or viewer name/id.
- [x] Recent event log.
- [x] Kill-switch state/result (now logged to the activity feed via `noteKillSwitch`, not just stderr).
- [ ] Relay connection health — not implemented. There is no reconnect/RTT state on the host side to surface yet; needs its own design (see below), not a bolt-on.

## Viewer connection graph

Purpose: make the console feel alive and useful without pretending to measure true ISP bandwidth.

Use connection health metrics such as:

- [ ] Latency/ping per viewer.
- [ ] Recent latency sparkline per viewer.
- [ ] Stream/output throughput if available.
- [ ] Backpressure or slow-write signal if available.
- [ ] Quality label: `good`, `laggy`, `stalled`, or similar.

Example row:

```text
alice  controller  42ms   ▁▂▃▄▅▆▅  good
bob    watching    180ms  ▁▁▂▁▃▂▁  laggy
```

## Architecture checklist

- [ ] Main getsloth process owns the real session and relay connection.
- [ ] Main process exposes a local-only host control channel.
- [ ] Control console connects locally to the main process, not through the public relay.
- [ ] Local control channel uses an unguessable per-session token or private socket path.
- [ ] Control console can subscribe to live state/events.
- [ ] Control console can send host actions: reclaim, kill viewers, request invite info.
- [ ] Closing the console does not end the getsloth session.
- [ ] Ending the getsloth session closes or invalidates the console channel.
- [ ] Multiple console windows should either be supported safely or rejected clearly.

## Terminal-launch checklist

- [ ] Auto-open a second terminal by default for every getsloth session.
- [ ] macOS support first.
- [ ] Prefer the user’s default terminal where practical.
- [ ] Fall back gracefully if terminal launch fails.
- [ ] Do not require tmux.
- [ ] Do not break users already running inside tmux.
- [ ] Later: Linux terminal launcher support.
- [ ] Later: Windows Terminal support.

## Security checklist

- [ ] Host control channel is local-only.
- [ ] Control token/socket path is not sent to viewers.
- [ ] Relay never receives the password for verification.
- [ ] Kill/reclaim actions are host-only.
- [ ] Do not expose control actions through the public viewer channel unless explicitly designed and authorized.
- [ ] No secrets written into repo, logs, or persistent files.

## Testing checklist

- [ ] Unit tests for local control state updates.
- [ ] Unit tests for reclaim action from control console.
- [ ] Unit tests for kill-viewers action from control console.
- [ ] Unit tests for invite info action.
- [ ] Unit tests for viewer health/sparkline calculation.
- [ ] Integration test for main process starting local control endpoint.
- [ ] Integration test for console connecting to endpoint.
- [ ] Integration test that console exit does not end the host session.
- [ ] Integration test that host session end invalidates console control.
- [ ] Manual test: `./scripts/dev-session.sh` opens main session and control console.
- [ ] Manual test: getsloth invoked directly opens main session and control console.
- [ ] Manual test: user already inside tmux does not get nested tmux/status-bar problems.

## Implementation phases

### Phase 1 — Document and clean current UX

- [x] Document this design and checklist.
- [x] Remove misleading stale startup status line.
- [x] Keep `Ctrl-] i` and `Ctrl-] r` fallback behavior.

### Phase 2 — Local host control channel

- [x] Add local-only control endpoint/socket in the host process.
- [x] Publish live session state to local subscribers.
- [x] Implement host actions over the local channel.

### Phase 3 — Basic control console

- [x] Add `getsloth control ...` command or equivalent internal console entrypoint.
- [x] Render basic terminal UI.
- [x] Show live session status, URL, password, viewers, and controller.
- [x] Show recent events.
- [x] Add keyboard commands.

### Phase 4 — Auto-open separate terminal

- [x] Launch control console automatically on session start.
- [x] Implement macOS Terminal launch first.
- [x] Add fallback messaging.

### Phase 5 — Buttons, mouse, and graph polish

- [x] Add visible clickable buttons.
- [x] Add mouse event support.
- [ ] Add per-viewer latency/health graph — blocked on relay/protocol RTT plumbing that doesn't exist yet (no ping/pong, no per-connection timing anywhere in `internal/relay` or `internal/protocol`). Faking numbers here would misrepresent real network state; scope as its own task.
- [ ] Add quality labels — same blocker as above.

## Open questions

- [ ] Which terminal UI library should be used for Go TUI rendering?
- [ ] Should closing the control console show a warning, or just exit silently?
- [ ] Should the kill switch require confirmation, or should `[k]` immediately disconnect viewers?
- [ ] Should invite copy include both URL and password, or URL only?
- [ ] How should viewer display names be assigned before real identity exists?
