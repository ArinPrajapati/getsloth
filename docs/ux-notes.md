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

## UX issues / decisions to resolve before implementation

### 1. Terminal emulation quality is a product requirement

The viewer is not a styled log window. It must behave like a real terminal in
the browser, because the user may run TUIs and agent CLIs such as `htop`,
`vim`, `less`, `tmux`, `claude`, or `codex`.

This means the implementation must preserve the real terminal model:

- Backend runs the wrapped process inside a real PTY, not plain stdout/stderr
  pipes.
- Relay streams raw bytes without rewriting or interpreting them.
- Browser writes those bytes directly into the terminal emulator.
- Browser input is forwarded back as raw terminal input bytes.
- Resize events are synced back to the PTY as real rows/columns.
- Keyboard handling supports terminal keys: arrows, Tab, Esc, Ctrl combos,
  Enter, Backspace, bracketed paste, etc.
- Mouse support should be considered for TUI apps that support mouse input.
- Font metrics, line height, and theme must be tuned so box drawing, cursor
  position, and full-screen TUIs do not look broken.

`xterm.js` remains the right default terminal emulator. It is the boring,
proven choice used by serious tools such as VS Code, Hyper, Azure Cloud Shell,
Proxmox-style web consoles, Render-style web consoles, and many web SSH tools.
It supports curses/TUI apps, colors, cursor movement, mouse events, Unicode,
IME, themes, accessibility options, WebGL rendering, and addons such as fit,
links, search, serialize, and Unicode handling.

Alternatives considered:

- `hterm`: mature and real, but more Chrome/Google-terminal flavored and does
  not buy enough to justify switching from xterm.js.
- WeTTY / ttyd / GoTTY: full web-terminal products/servers, not replacements
  for the frontend emulator; many use xterm.js underneath.
- `term.js`: older lineage, not the choice for a new product.
- jQuery Terminal / fake terminal widgets: not suitable for real PTY/TUI
  emulation.
- DomTerm: interesting, more ambitious, heavier, and less standard for this
  v0 use case.

Decision: stay with xterm.js, but test it like a real terminal. Before calling
the UX done, manually verify with `htop`, `vim`, `less`, `tmux`, `claude`, and
`codex`, including resize and keyboard behavior.

Implementation note from the first UX pass: fitting the browser element is not
enough. xterm's fitted columns/rows must be sent back through the relay and
applied to the host PTY. Otherwise full-screen TUIs like `nvim` appear trapped
in a small top-left grid even though the CSS terminal area is fullscreen.

tmux-specific UX note from browser testing: split panes render, but pane focus
is easy to misunderstand. Keyboard input goes to tmux's active pane, not
necessarily the pane the user visually clicked/tapped. Clicking/tapping a pane
only changes focus when tmux mouse mode is enabled (`set -g mouse on`), and on
mobile the usual tmux prefix/arrow workflow is awkward without helper keys.
Acceptance testing must cover:

- vertical and horizontal splits;
- moving pane focus with keyboard shortcuts;
- moving pane focus by mouse/touch when tmux mouse mode is enabled;
- whether the active pane is visually clear enough in the browser;
- mobile helper keys for tmux workflows (`Ctrl-b`, arrows, Esc, Tab).

Shell default note from the same test: getsloth should not create or style a
custom shell/prompt. If the host runs `getsloth` without an explicit command,
it should start the user's configured default shell (`$SHELL`), matching tmux's
principle of using the user's shell environment. If the host explicitly runs
`getsloth sh`, then seeing a plain `sh-3.2$` prompt is correct because that is
the command they asked for.

Termux reference: mobile terminal UI can be dense and still usable when it is
honest about being a terminal. The lesson is not to copy Termux branding or
colors; it is that TUI output should occupy the screen as terminal output, with
the app's own function bars (`htop`'s F1/F2/etc., `vim` status lines, agent
prompts) rendered inside the terminal grid. getsloth controls should stay out
of that grid except for a thin viewer status layer and a mobile terminal-helper
row when typing.

### 2. Typing is first-class on mobile

Do not design the mobile viewer around permanent "Yes / No / Continue"
buttons. Those came from an earlier quick-action idea, but they make the
interface look like an approval app instead of a terminal viewer.

The user must be able to type properly on a phone. When the OS keyboard opens,
it may take roughly 40-50% of the screen. The viewer must still keep enough of
the terminal visible for context and must not hide the current prompt or the
latest output behind controls.

Most important: the user must see what they are typing. The current cursor row
and input area must remain visible above the OS keyboard/composer. If the cursor
is near the bottom of the terminal when typing starts, the terminal viewport
needs to resize, pan, or otherwise keep the cursor/input line in view. A mobile
typing mode that hides the prompt under the keyboard is a failed design even if
the terminal technically receives input.

xterm.js can help here: the public buffer API exposes the active buffer and
cursor cell position (`term.buffer.active.cursorX` and
`term.buffer.active.cursorY`; the active buffer matters because apps like `vim`
and `htop` use the alternate screen). Use public API only, never private `_core`
fields. That gives terminal-cell coordinates, which the viewer can translate
using the fitted rows/cols and cell dimensions into a "is the cursor covered by
the keyboard/composer?" layout decision.

Implications:

- Default mobile status bar should be compact: session status, chat,
  type/input, control, settings.
- "Type" opens a focused input/composer that is explicitly designed around the
  phone keyboard being visible.
- Terminal should resize or reserve space above the keyboard/composer, not be
  covered blindly.
- Typing mode must keep the active cursor/input line visible. This should be a
  visual acceptance test on an actual phone, not only desktop responsive mode.
- Quick actions, if they return, are contextual helpers only: shown when a real
  prompt/decision state is detected or when the user opens an actions helper.
  They are not permanent primary UI.

### 3. Status bar should be tmux-like, not a row of product cards

The viewer should feel like a terminal session with a status bar, not a web app
with cards stacked around a terminal.

Desktop/laptop:

- Single-line status bar is preferred.
- It can show more labels directly because there is space.
- Suggested shape: live/session state, active driver, viewer count, current
  focus/waiting state, chat notification, type, control, settings.

Phone:

- Bar can compress labels into shorter text/icons.
- It may become two rows only when typing or when an active notification/action
  needs more room.
- It must respect coarse touch targets without making the status bar dominate
  the terminal.

Large browser surfaces, TVs, PlayStation/browser devices:

- Same mental model: terminal owns the screen, status bar supports it.
- Need readable font-size options and strong focus states for keyboard/controller
  navigation.
- Avoid hover-only interactions.

### 4. Terminal theme and settings are viewer tools, not branding

Terminal theme and settings exist to make the session readable and comfortable.
They are not a place for getsloth branding.

Settings to consider for v0/v1:

- Terminal theme: dark neutral default, possibly warm/dim variants.
- Font size: especially important for mobile, TVs, and high-DPI laptop screens.
- Line height/font metrics: must preserve terminal grid correctness.
- Status bar show/hide toggle for an undistracted terminal.
- Maybe keyboard helpers for mobile: Esc, Ctrl, Tab, arrows, paste.
- For TUI/terminal use on mobile, consider a Termux-like helper row above the
  OS keyboard for Esc, Ctrl, Tab, arrows, paste, and maybe function keys. This
  is terminal assistance, not product branding or quick-action UI.

Branding belongs on the landing page and marketing surfaces. The live session
viewer should feel like a productivity environment, not a branded product page.

## Pre-implementation UX gap checklist

Current state after merging `main` into `claude/ux-design`:

- Integration has moved forward and the frontend is wired end-to-end, but the
  viewer UI is still the old card-based layout: branded hero, status card,
  terminal card, separate chat card, separate control card, and sticky
  quick-actions card on mobile.
- That layout was correct enough for proving behavior, but it contradicts the
  new UX direction. The design pass is not just "make colors nicer"; it needs
  to restructure the viewer around terminal-first layout.

Before finalizing the UX spec and starting implementation, resolve these gaps:

1. **Session shell**
   - Replace the live-session hero/card shell with a terminal-first shell.
   - No `getsloth` wordmark/marketing copy inside the authenticated live
     session view.
   - Password gate can still be a focused gate, but after auth the terminal
     should become the page.

2. **Status bar**
   - Build one tmux-like status bar as the main control surface.
   - It should contain session/connection state, active driver/control state,
     chat notification, type/input entry point, settings, and maybe viewer
     count.
   - Avoid separate persistent cards for chat/control/quick-actions in the
     live-session view.

3. **Quick actions**
   - Remove permanent `Yes / No / Continue` from the default UI.
   - Keep any quick actions contextual only: prompt-aware helper or optional
     action menu, never the main visible product surface.
   - Existing `quick-actions.ts` behavior can remain as tested protocol glue
     until the design pass rewires it, but the visible UX should not center on
     those buttons.

4. **Typing model**
   - Typing is first-class on mobile.
   - Add a proper `Type`/input mode instead of assuming users only tap canned
     actions.
   - When the OS keyboard opens, keep the active cursor/input row visible.
   - This likely requires the terminal view layer to expose/track cursor
     position from xterm's public active buffer API and combine it with the
     visible keyboard/composer space.

5. **Mobile terminal helper row**
   - Explore a Termux-like helper row for terminal keys: Esc, Ctrl, Tab,
     arrows, paste, maybe function keys.
   - This row is terminal assistance, not product chrome.
   - It should appear when typing or when a TUI needs it, not permanently steal
     screen space.

6. **Terminal sizing and resize**
   - xterm currently uses a fixed font/theme and no FitAddon-style fitting in
     `web/src/xterm-terminal.ts`.
   - The design pass needs a reliable fit/resize story: terminal fills the
     available viewport, status bar/composer/keyboard reserve space, and the
     terminal grid updates cleanly.
   - Be careful with PTY resize semantics: the host machine owns the real PTY.
     A viewer's browser size may not always be allowed to resize the shared PTY
     without disrupting the host. Decide whether viewer resize only affects the
     browser render, or whether active-control viewers can request PTY resize.

7. **TUI compatibility checks**
   - Manual verification must include TUI/full-screen terminal programs, not
     only agent text output.
   - Test: `htop`, `vim`, `less`, `tmux`, `claude`, `codex`, plus resize,
     arrow keys, Ctrl+C, Esc, Tab, paste, and mouse where applicable.
   - The goal is "looks like a real terminal on phone/laptop/browser", not
     "logs stream into a dark box."

8. **Terminal theme/settings**
   - Need viewer settings for at least font size and status-bar visibility.
   - Theme should be neutral terminal-first: dark readable default, maybe dim
     and high-contrast variants.
   - Settings are viewer-local comfort controls, not session state and not
     branding.

9. **Chat**
   - Chat should move behind the status bar with notification state.
   - New messages should be visible as a small highlight/badge while the
     terminal remains primary.
   - Chat opens as an overlay/sheet and must not write into the PTY path.

10. **Large-browser / TV / console-browser support**
    - Same UX model as desktop/mobile: terminal owns the screen.
    - Need readable font-size settings and visible focus states for keyboard or
      controller navigation.
    - Avoid hover-only controls.

11. **Accessibility/performance gates**
    - F8 is still pending in `tasks/todo.md`.
    - The design pass should expect to run `axe` and `lighthouse`, but the
      human phone/keyboard/TUI checks are equally important for this product.

Revisit this whole note when starting the actual UX pass.
