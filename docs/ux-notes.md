# UX notes — viewer

Working notes for the UX/UI pass, to be done in `worktree/ux-design` once
Backend integration and end-to-end testing are further along. Not a spec,
not binding — just captured thinking so it isn't lost between sessions.

## Why does the host open the link on a second device?

Question raised 2026-09-17. The answer is simple, don't overthink it: he
wants to see his terminal in the browser. That's the whole ask. The reason
he's on a different device doesn't matter — the second he opens the link,
the expectation is his live terminal, in the browser, right there.

So the design bar is just that: does opening the link show the terminal,
clearly and immediately, with everything else (chat, control, quick
actions) in support of that one job rather than competing with it for
attention.

## Reference: YouTube's control bar

Full-screen YouTube shows every control (play, timeline, volume, captions,
settings, fullscreen) at all times — it doesn't hide them behind a gesture
or a menu. But they're a thin overlay on top of the video, not separate
boxes stacked below it competing for space. The video still reads as the
whole screen even though the controls are always present and reachable.

That's the model, not a collapsed/hidden-by-default sheet (an earlier,
wrong instinct in this note's first draft):

- **Terminal = the video.** Full-bleed, the whole screen, the default
  undistracted state.
- **Chat / control / quick-actions = the YouTube control bar.** An overlay
  on top of the terminal, not separate cards stacked in document flow.
  Present and reachable at all times, visually subordinate (thin,
  low-contrast until touched) so they don't fight the terminal for
  attention.
- **The overlay is toggleable.** If the host wants a fully undistracted
  view — just the raw terminal, nothing else on screen — they can hide it,
  same as hiding YouTube's controls.

## Branding stays out of the productivity surface

Branding ("getsloth" wordmark, logo, marketing flourish) belongs on the
landing page and other marketing-facing surfaces — not in the actual
viewer/session screen. The screen the host is using to watch/control a
live session stays free of brand dressing; that's for where we're selling
the product, not where someone's trying to see their terminal.

## Decisions (founder, 2026-09-17)

Three decisions handed down, recorded here as-is — locked, not up for
reinterpretation during the UX pass.

### 1. No kill switch in the participant-facing frontend

The kill switch is never exposed in the web/terminal viewer, for this
version. Reason: if a participant could remotely trigger it, the server
could shut down while that same server is their only means of access —
once disconnected, they may have no way to reconnect or recover the
session.

- No kill-switch control anywhere in the web viewer, now or by accident
  later.
- Kill switch stays available only from the machine/environment where the
  session was originally started (the host CLI) — this already matches
  `docs/protocol.md`: `KillSwitchMsg` is host→relay only, never a message a
  viewer can send.
- Remote kill-switch functionality can be reconsidered in a later version
  if a concrete use case demands it, and only with real permissions and
  safeguards against accidental or unauthorized shutdowns.

This is a constraint on future work as much as a note on current work —
nobody should add a "kill session" button to the viewer without revisiting
this decision explicitly.

### 2. Status bar — tmux-style, the main control surface

The frontend should follow a tmux status-bar approach: one persistent bar
that's the main control area for the session, not scattered buttons/cards
across the interface. This is the concrete shape for the "YouTube control
bar" idea above — a single bar, not a floating button cluster.

The status bar holds:
- Session status
- Connection/server status
- Chat and messaging
- Notifications
- Other frequently used session actions

### 3. Chat lives in the status bar, with a notification state

Chat is reached through the status bar, not a separate always-open panel.

- New message arrives → the chat/message icon shows a notification or
  highlighted state, so the participant knows immediately without having
  the panel open.
- Clicking the icon/notification opens the messaging interface.
- The participant reads and replies without leaving the main working
  view — chat is an overlay/sheet over the terminal, not a navigation away
  from it.

Net effect: interface stays compact, controls and communication stay
reachable, without ever interrupting the primary "watch the terminal" job.

Revisit this whole note when starting the actual UX pass.
