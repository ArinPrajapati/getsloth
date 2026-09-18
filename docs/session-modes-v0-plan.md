# getsloth v0 session modes and terminal geometry plan

Status: approved and implemented in the UX worktree; real-device/TUI validation remains.

## Why this exists

A running terminal process has one PTY and therefore one rows/columns geometry.
The current viewer lets each browser fit xterm independently, but the process
still emits one cursor-addressed byte stream for the active PTY geometry. A
desktop-sized Claude, Codex, Vim, htop, or tmux screen cannot be reflowed into a
phone-sized grid without corruption.

A full multi-layout terminal engine is not justified before product usage is
validated. v0 will instead offer two explicit session modes with predictable
geometry and permissions.

## Product decision

Do not infer behavior from device type or user agent. A phone in landscape can
be wider than a split laptop window, and browser/device detection is unreliable.
The host chooses a **session mode**; the mode determines viewer count, control
permissions, and terminal-size ownership.

### Remote mode (default)

Purpose: one person controlling their own terminal from one other device.

- Participants: host plus one authenticated remote viewer identity.
- The remote viewer may be a phone, tablet, laptop, console browser, or TV.
- Host owns the PTY geometry initially.
- While the host owns control, the viewer displays the host's exact grid using
  fit or pan; it does not independently reflow the terminal.
- `Type` requests control with the viewer's fitted rows/columns. Control and
  geometry transfer together; after confirmation the browser opens its input.
- While the viewer controls, its visual viewport owns PTY dimensions. Opening
  the mobile keyboard may reduce rows; orientation or viewport-width changes
  may change columns.
- Host reclaim or viewer disconnect immediately restores the host's latest
  cached terminal dimensions.
- A second authenticated identity is rejected with a clear occupied state.
- A reconnect token reserves the remote slot for the existing identity during
  the existing 30-second reconnect window. A resumed viewer rejoins as a
  spectator and must request control again.

### Group mode

Purpose: share a terminal with several people for watching and discussion.

- Participants: host plus multiple authenticated viewers.
- Host is always the active writer and always owns PTY dimensions.
- Viewers can watch, open chat, receive notifications, change local theme/font
  presentation, and choose fit or pan/zoom.
- Viewer `take_control`, `input`, and `resize` are rejected by the relay. This is
  enforced by the server, not merely hidden in the UI.
- Phone viewers receive the same host-sized grid as laptop viewers. Their
  default is fit-to-width; they can switch to readable 1:1 text with panning.
- Remote control in group sessions is deferred until usage proves it is needed.

## Behavior matrix

| Capability | Remote mode | Group mode |
|---|---|---|
| Authenticated remote identities | One | Multiple |
| Initial active writer | Host | Host |
| Viewer can take control | Yes | No |
| Viewer can send PTY input | Only while active | No |
| Viewer can resize PTY | Only while active | No |
| PTY geometry owner | Active writer | Host |
| Viewer chat | Yes | Yes |
| Phone presentation while host drives | Fit or pan host grid | Fit or pan host grid |
| Host reclaim | Immediate | Already host-controlled |

## User-facing entry points

Keep the current command shape compatible:

```text
getsloth                     # remote mode, default shell
getsloth claude              # remote mode, command
getsloth --remote claude     # explicit remote mode
getsloth --group claude      # group mode, command
```

The development launcher (`./scripts/dev-session.sh`) asks the host to choose
Remote or Group when neither flag is supplied. Automation can pass
`--remote` or `--group` to skip the prompt.

Only known getsloth flags before the command are parsed. Arguments after the
command continue to belong to the wrapped command. Help text must explain the
permission difference, not just call the modes "solo" and "group."

The share page identifies the mode after authentication:

- Remote mode: `Remote session · one controller`
- Group mode: `Shared viewing · host controls`

## Terminal geometry model

The session has one canonical terminal size:

```text
host local size ─┐
                 ├─ active geometry owner ─> PTY cols/rows ─> all viewers
viewer fit size ─┘
```

The relay tracks:

- `mode`
- canonical `cols` and `rows`
- latest host `cols` and `rows` as the reclaim fallback
- active writer
- remote-mode viewer slot and reconnect reservation
- a geometry epoch so replay never mixes output drawn for different sizes

Every viewer xterm is resized to the canonical grid before terminal output is
written. A spectator's CSS viewport may scale or pan that grid, but xterm's
logical columns/rows remain canonical.

### Host lifecycle

1. Read the host terminal size synchronously before starting the wrapped
   process.
2. Start the PTY at that exact size; do not start at a default and resize later.
3. Cache every host `SIGWINCH` size.
4. Apply and broadcast host size changes only while host is the geometry owner.
5. While a remote viewer controls, host `SIGWINCH` updates only the fallback
   cache and must not resize the PTY.
6. On reclaim/disconnect, apply the cached host size immediately and notify all
   viewers.
7. Keep the local terminal/tab title updated with live/offline state, mode,
   viewer count/names, and the current controller. This exposes status without
   reserving a PTY row or corrupting a full-screen TUI.
8. Reserve `Ctrl-]` as the local getsloth command prefix. `Ctrl-] r` reclaims
   control even while host input is gated; `Ctrl-] i` prints detailed status on
   demand. The existing `SIGUSR2` reclaim remains available for scripts.

### Viewer lifecycle

1. After auth, receive session mode and canonical geometry before output replay.
2. Create/resize xterm to the canonical rows/columns.
3. In spectator state, render using fit-to-width or 1:1 pan without sending PTY
   resize messages.
4. In remote mode, `Type` calculates the current usable grid and requests
   control with those dimensions.
5. Focus the terminal only after `control_changed` confirms this viewer owns
   control and the corresponding geometry is active.
6. While active, debounce viewport changes and send updated dimensions.

## Protocol revision

The implemented protocol uses the following shapes. `docs/protocol.md` is the
schema of record.

### Session creation

Host creation declares:

```typescript
interface SessionConfigMsg {
  v: 1;
  type: "session_config";
  mode: "remote" | "group";
  host_cols: number;
  host_rows: number;
}
```

### Authenticated session state

Returned in the successful `auth_result` before any replayed output:

```typescript
interface AuthResultMsg {
  v: 1;
  type: "auth_result";
  ok: true;
  token: string;
  connection_id: string;
  mode: "remote" | "group";
  cols: number;
  rows: number;
  active_writer_id: string;
  active_writer_role: "host" | "viewer";
}
```

### Atomic viewer takeover

Remote-mode viewer takeover includes the desired grid:

```typescript
interface TakeControlMsg {
  v: 1;
  type: "take_control";
  cols: number;
  rows: number;
}
```

The relay validates mode, viewer identity, and bounds, then transfers writer and
geometry as one state transition. It must not first announce control and wait
indefinitely for a later resize.

### Canonical-size broadcast

```typescript
interface TerminalSizeMsg {
  v: 1;
  type: "terminal_size";
  cols: number;
  rows: number;
}
```

The relay sends this whenever canonical geometry changes. It is state, not a
request for each spectator to resize the PTY.

### Errors

Add programmatic errors:

- `SESSION_OCCUPIED`: another remote identity owns the remote-mode slot.
- `READ_ONLY_SESSION`: a group viewer attempted control, input, or resize.

Do not depend on disabled buttons for enforcement.

## Output replay boundary for v0

Perfect late-join screen restoration needs a real VT state snapshot and is
explicitly deferred. v0 still makes replay materially safer:

- Send canonical geometry before replay.
- Associate buffered output with a geometry epoch.
- Clear the raw replay buffer whenever canonical geometry changes so bytes
  drawn for two grids are never replayed into one screen.
- Treat replay as best-effort recent context, not a guaranteed screen snapshot.
- A resize sends `SIGWINCH` to the running application so full-screen TUIs can
  repaint for the new owner.

Do not claim perfect late-join restoration for every TUI until a VT snapshot
model exists.

## Viewer UX

### Remote mode

- Host driving: status bar says `Watching · host controls`.
- `Type` is an action, not a selected cosmetic state. It requests control.
- Pending: `Taking control…`; keyboard remains closed.
- Granted: status changes to `You control`; terminal focuses and keyboard may
  open.
- Rejected/host lock: show the reason and keep keyboard closed.
- Disconnect: host control and host geometry restore immediately.

### Group mode

- Status bar says `Shared viewing · host controls`.
- Remove `Type` and `Control` actions entirely.
- Keep connection state, chat notification, view control, and settings.
- View control exposes `Fit` and `Actual size`.
- On touch devices, actual-size mode supports panning without focusing the
  terminal or opening the keyboard.

### Shared rules

- No branding inside the live terminal surface.
- Status bar remains one row where possible and respects safe-area insets.
- Terminal taps do not open the keyboard unless control has been granted.
- Theme and font choices are local presentation settings. In spectator mode
  they must not change canonical PTY dimensions.

## Implementation sequence

### M0 — contract approval — complete

- Review and approve this mode boundary.
- Update `docs/protocol.md` first.
- Mark the old unrestricted multi-view control behavior as superseded for v0.

### M1 — CLI mode and host geometry — complete

- Parse `--group`; default to remote.
- Start PTY with the host's real size synchronously.
- Cache host size independently of whether it may currently resize the PTY.
- Restore cached host size on reclaim/disconnect.
- Add Go tests around command parsing and geometry ownership.

### M2 — relay policy and state — complete

- Store mode and terminal geometry in each session.
- Enforce one remote identity plus reconnect reservation in remote mode.
- Enforce host-only input/control/resize in group mode.
- Make remote takeover update control and geometry atomically.
- Reassign host and restore host geometry on active-viewer disconnect.
- Clear buffered raw output on every canonical geometry change so replay never
  mixes bytes drawn for different grids (the v0 equivalent of an epoch).
- Add adversarial tests proving forbidden messages never reach the host PTY.

### M3 — browser protocol client — complete

- Parse session-state and terminal-size messages.
- Send desired dimensions with take-control.
- Surface occupied/read-only errors as explicit states.
- Keep resize sends disabled until control is confirmed.

### M4 — canonical-grid terminal renderer — implemented; device validation pending

- Separate logical terminal rows/columns from the browser container size.
- Active remote controller fits xterm and proposes PTY dimensions.
- Spectators keep canonical xterm dimensions and use presentation scaling/pan.
- Refit on `visualViewport` changes while typing and keep the cursor visible.
- Ensure pan gestures never focus xterm or summon the mobile keyboard.

### M5 — mode-aware status bar — implemented; console focus polish pending

- Remote: Watch/Type/control states described above.
- Group: connection, chat, Fit/Actual size, settings; no control affordance.
- Add keyboard/controller focus styling for TV and console browsers.

### M6 — end-to-end verification — pending

- Run automated Go and TypeScript suites plus repository checks.
- Run real sessions on desktop and iPhone with shell, Claude/Codex, Vim, htop,
  less, and tmux.
- Test phone portrait/landscape, keyboard open/closed, browser chrome expanded,
  disconnect/resume, host reclaim from `Ctrl-] r`, local title/status updates,
  and a second remote join attempt.

## Acceptance criteria

### Remote mode

- One remote viewer can watch and take control from phone or laptop.
- A second identity cannot join the control session.
- Before takeover, remote display is coherent even when narrower than host.
- Takeover changes PTY to the viewer grid and a TUI redraws without stale
  desktop fragments.
- The keyboard does not cover the active cursor/input row.
- Host reclaim and viewer disconnect restore host geometry immediately.
- Reconnect does not silently restore control.

### Group mode

- At least three simultaneous viewers receive ordered live output and chat.
- Host terminal dimensions remain unchanged as viewers join, resize, rotate, or
  open keyboards.
- Viewer control/input/resize attempts are rejected server-side.
- Desktop viewers show the canonical grid clearly.
- Phone viewers default to a coherent fit view and can switch to readable pan.
- No touch gesture opens a typing keyboard in the read-only terminal.

### Quality gates

- Existing auth, kill-switch, teardown, and reconnect tests remain green.
- Touched Go and TypeScript code meets `CONSTRAINTS.md` coverage requirements.
- Manual TUI checks show no mixed-width corruption.
- Accessibility and performance gates run as part of the existing F8 work.

## Explicitly deferred

- Simultaneous independent responsive TUI layouts.
- Remote control by group participants.
- Perfect terminal snapshots and guaranteed late-join restoration.
- Application-specific Claude/Codex semantic rendering.
- Automatic mobile/desktop classification.
- Multiple terminals/resources in one room.

## Rollout and learning

Instrument only privacy-safe counters if analytics are added later:

- sessions created by mode;
- remote takeover used or not;
- group viewer count;
- fit versus actual-size view usage;
- reconnect and occupied-session outcomes.

Reconsider group remote control or a terminal-state engine only after real usage
shows which limitation users actually encounter.
