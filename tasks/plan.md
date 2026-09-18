# Implementation Plan: getsloth v0

## Overview

A CLI (`getsloth <command>`) wraps any command in a PTY and relays it through
a hosted WebSocket relay to a password-gated browser viewer, so the host can
watch and control their own AI agent session from any device — the headline
pitch is controlling it from your phone. Stack: Go for the CLI + relay,
TypeScript + xterm.js for the browser client. Full context in
`docs/ideas/getsloth.md`; quality bar in `CONSTRAINTS.md`. Ships Friday.

## Architecture Decisions

- **Protocol contract defined first, before either workstream starts
  networking code.** This is the seam between the two agents working in
  parallel — Backend implements the relay/host side of it, Frontend
  implements the browser side, and neither has to wait on the other's
  internals, only on the contract being stable.
- **Backend workstream (Go): PTY + CLI + relay server.** Owns everything
  that isn't rendered in a browser.
- **Frontend workstream (TypeScript + xterm.js): browser viewer.** Owns
  everything the viewer sees and interacts with, built against the protocol
  contract — can be developed against a mock/stub relay if the real one
  isn't ready yet.
- **Password verification lives only in the host CLI process**, never in
  the relay — this is the CONSTRAINTS.md architecture rule, enforced by a
  `depguard` lint rule in the Go module structure (Task B1 sets up the
  package boundary this depends on).
- **Integration and manual multi-device verification (phone + laptop) is a
  human step**, not something either agent can fully self-verify — flagged
  explicitly in Phase 2.

## Task List

### Phase 0: Shared Contract

- [ ] **Task T0: Define and document the WebSocket protocol**

  **Description:** Write `docs/protocol.md` specifying every message type
  exchanged between host CLI, relay, and browser viewer: `join`, `auth`
  (password attempt), `auth_result` (pass/fail + session token), `output`
  (terminal data chunk), `input` (keystrokes, only valid from the current
  active writer), `take_control`, `control_changed` (broadcast of who holds
  control), `chat_message`, `quick_action` (translates to `input` on the
  wire — mobile yes/no/continue/text), `kill_switch`, `kicked`,
  `session_ended`, `presence` (who's connected). For each: sender, payload
  shape (JSON schema, not just prose), and who validates it (relay vs host).

  **Acceptance criteria:**
  - [ ] Every functional requirement (host CLI FRs 1–12 from the spec) maps
        to at least one message type in the doc
  - [ ] The doc states explicitly which messages the relay is allowed to
        inspect/act on vs. blindly forward (this is where the "relay never
        sees the password in cleartext comparison logic" boundary is made
        concrete)
  - [ ] Both workstreams (Backend, Frontend) sign off before Phase 1 starts

  **Verification:**
  - [ ] Manual check: founder reviews and approves before either agent
        starts networking code

  **Dependencies:** None

  **Files likely touched:** `docs/protocol.md`

  **Estimated scope:** Small: 1 file

### Checkpoint: Contract approved
- [ ] `docs/protocol.md` exists and is approved by the founder
- [ ] Backend and Frontend workstreams can start in parallel

---

### Phase 1A: Backend workstream (Go) — CLI + Relay

- [ ] **Task B1: Go module scaffold + tooling wired to CONSTRAINTS.md**

  **Description:** `go mod init`, package layout separating `cmd/getsloth`
  (host CLI) from `internal/relay` (server) with a `depguard` rule (per
  CONSTRAINTS.md) preventing `internal/relay` from importing any
  password-comparison code. Add `scripts/check.sh` running
  `gofmt -l . && go vet ./... && staticcheck ./... && gitleaks detect
  --redact --no-banner`.

  **Acceptance criteria:**
  - [ ] `go build ./...` succeeds with an empty/stub `main.go`
  - [ ] `scripts/check.sh` runs clean
  - [ ] `internal/relay` importing a package containing password comparison
        logic fails lint (verify by temporarily adding one and confirming
        the failure, then removing it)

  **Verification:**
  - [ ] `bash scripts/check.sh`
  - [ ] Manual check: the depguard rule actually fires (test as above)

  **Dependencies:** None (can start immediately, doesn't need T0)

  **Files likely touched:** `go.mod`, `.golangci.yml`, `scripts/check.sh`,
  `cmd/getsloth/main.go`, `internal/relay/relay.go`

  **Estimated scope:** Small: 3-4 files

- [ ] **Task B2: PTY wrapping — `getsloth <command>` works standalone**

  **Description:** Spawn the given command in a PTY (`creack/pty`), pipe
  the host's real stdin/stdout/stderr through it so running
  `getsloth claude` behaves identically to running `claude` directly. No
  networking yet — this proves the core mechanic in isolation.

  **Acceptance criteria:**
  - [ ] `getsloth echo hello` prints `hello` and exits 0
  - [ ] `getsloth bash` gives an interactive shell indistinguishable from a
        normal terminal (arrow keys, Ctrl+C, resize all work)
  - [ ] Exit code of the wrapped process is propagated

  **Verification:**
  - [ ] `go test ./cmd/getsloth/... -cover` (target ≥80% of this package)
  - [ ] Manual check: run an interactive program (`vim`, or the actual
        agent CLI) through it and confirm normal usage feels unchanged

  **Dependencies:** B1

  **Files likely touched:** `cmd/getsloth/pty.go`, `cmd/getsloth/main.go`,
  `cmd/getsloth/pty_test.go`

  **Estimated scope:** Small: 2-3 files

- [ ] **Task B3: Relay server skeleton — sessions, host + viewer connect**

  **Description:** WebSocket server implementing `join` from T0's protocol:
  a host connects and creates a session (gets a session ID), a viewer
  connects with a session ID and is held in a waiting state (no auth yet —
  that's B5). No terminal data relayed yet, just connection lifecycle.

  **Acceptance criteria:**
  - [ ] Host connection creates a session; a second host connection with
        the same process is not possible (one session per running
        `getsloth`)
  - [ ] Viewer connecting to an unknown session ID gets a clean rejection,
        not a hang or crash
  - [ ] Host disconnect tears down the session and disconnects any viewers

  **Verification:**
  - [ ] `go test ./internal/relay/... -cover` (≥80%)
  - [ ] `bash scripts/check.sh`

  **Dependencies:** T0, B1

  **Files likely touched:** `internal/relay/session.go`,
  `internal/relay/server.go`, `internal/relay/session_test.go`

  **Estimated scope:** Medium: 3-5 files

- [ ] **Task B4: Live output streaming (host PTY → relay → viewer)**

  **Description:** Wire B2's PTY output through B3's relay to any connected
  viewer as `output` messages, in real time. This is the first true
  end-to-end vertical slice: a viewer can watch a live session (auth still
  bypassed/stubbed for this task).

  **Acceptance criteria:**
  - [ ] A connected viewer sees terminal output within the <300ms latency
        target under a normal local-network test
  - [ ] Output ordering is preserved (no interleaving/reordering under
        rapid output)

  **Verification:**
  - [ ] `go test ./... -cover`
  - [ ] Manual check: run `getsloth` locally, connect a second
        terminal/browser stub as viewer, confirm live output

  **Dependencies:** B2, B3

  **Files likely touched:** `internal/relay/stream.go`, `cmd/getsloth/main.go`

  **Estimated scope:** Medium: 3-4 files

### Checkpoint: Backend can stream (no auth/control yet)
- [ ] Tests pass, `scripts/check.sh` clean
- [ ] A raw viewer (even a test script) can watch live output end-to-end
- [ ] Review with founder before proceeding

- [ ] **Task B5: Password auth — local verification + rate limiting**

  **Description:** Implement `auth`/`auth_result` per `docs/protocol.md`:
  viewer encrypts a password attempt to the host's session public key
  (never plaintext on the wire), relay forwards the ciphertext opaquely,
  host decrypts and compares locally against the password it was started
  with (generated or `--password` flag) and returns pass/fail. The relay
  itself issues the session token on pass — not the host. Rate-limit
  failed attempts keyed on `(session_id, remote_address)`, per
  `docs/protocol.md`.

  **Acceptance criteria:**
  - [ ] Correct password grants a session token; viewer can now receive
        `output` messages (previously gated)
  - [ ] Wrong password is rejected, and after N attempts (define N, suggest
        5) that client is dropped/blocked for a cooldown period
  - [ ] `internal/relay` package contains no password-comparison code
        (verified by the B1 depguard rule)

  **Verification:**
  - [ ] `go test ./... -cover`, specifically covering the rate-limit and
        pass/fail branches
  - [ ] `bash scripts/check.sh` including `gosec ./...`

  **Dependencies:** B4

  **Files likely touched:** `cmd/getsloth/auth.go`, `internal/relay/auth.go`,
  `internal/relay/ratelimit.go`, associated `_test.go` files

  **Estimated scope:** Medium: 4-5 files

- [ ] **Task B6: Control handoff — single active writer**

  **Description:** Implement `input`, `take_control`, `control_changed`.
  Only the current active writer's `input` messages reach the PTY; a
  `take_control` message instantly reassigns the active writer and
  broadcasts `control_changed`. Host has a local (non-networked) override
  to reclaim control instantly regardless of current holder.

  **Acceptance criteria:**
  - [ ] Non-active-writer `input` messages are silently dropped (not
        applied to the PTY)
  - [ ] `take_control` reassigns instantly; all connected clients receive
        `control_changed`
  - [ ] Host's local override always wins, even mid-handoff

  **Verification:**
  - [ ] `go test ./... -cover`
  - [ ] Manual check: two viewer stubs, confirm handoff and host override

  **Dependencies:** B5

  **Files likely touched:** `internal/relay/control.go`,
  `internal/relay/control_test.go`, `cmd/getsloth/override.go`

  **Estimated scope:** Medium: 3-4 files

- [ ] **Task B7: Kill switch**

  **Description:** Host-triggered command that disconnects all current
  viewers (invalidates their session tokens) without ending the wrapped
  process or the host's own session — host can issue a new password
  afterward and reshare.

  **Acceptance criteria:**
  - [ ] All connected viewers are disconnected within one round-trip of the
        host triggering it
  - [ ] The wrapped process keeps running, unaffected
  - [ ] A new password can be set and a fresh viewer can join afterward

  **Verification:**
  - [ ] `go test ./... -cover`
  - [ ] Manual check: trigger with 2+ connected viewer stubs

  **Dependencies:** B6

  **Files likely touched:** `cmd/getsloth/main.go`, `internal/relay/kill.go`

  **Estimated scope:** Small: 2 files

- [ ] **Task B8: Session teardown + reconnect resilience**

  **Description:** Clean relay-side teardown when the wrapped process or
  the `getsloth` process exits (`session_ended` broadcast). Viewer
  reconnect: a browser tab that briefly loses network can reconnect to the
  same session with the same token without the host doing anything.

  **Acceptance criteria:**
  - [ ] Wrapped process exit sends `session_ended` to all viewers and
        cleans up relay state (no leaked goroutines/connections)
  - [ ] A viewer reconnecting within a short window (define: 30s) with a
        valid token resumes without re-entering the password
  - [ ] A viewer reconnecting after the session ended gets a clear
        "session ended" state, not a hang

  **Verification:**
  - [ ] `go test ./... -cover`
  - [ ] Manual check: kill the wrapped process, confirm viewer sees ended
        state; toggle network off/on briefly, confirm reconnect

  **Dependencies:** B7

  **Files likely touched:** `internal/relay/session.go`,
  `internal/relay/reconnect.go`

  **Estimated scope:** Medium: 3 files

### Checkpoint: Backend feature-complete
- [ ] All Backend tests pass, `scripts/check.sh` clean, `govulncheck ./...`
      clean (or findings triaged)
- [ ] Full protocol from T0 is implemented relay-side
- [ ] Review with founder before Phase 2 integration

---

### Phase 1B: Frontend workstream (TypeScript + xterm.js) — Browser Viewer

Can start as soon as T0 is approved — does not need to wait on Backend
tasks; build against a mock relay (a small local WebSocket stub emitting
T0's message shapes) until B4/B5/B6 are ready for real integration.

- [x] **Task F1: Project scaffold + tooling wired to CONSTRAINTS.md**

  **Description:** Vite + TypeScript scaffold, `eslint` + `tsc --noEmit`
  wired into `scripts/check.sh` (frontend variant), no framework beyond
  xterm.js and vanilla DOM — per the earlier decision that a heavy
  framework isn't needed for a single page.

  **Acceptance criteria:**
  - [ ] `npm run build` produces a working static site
  - [ ] `tsc --noEmit` and `eslint .` run clean on the scaffold

  **Verification:**
  - [ ] `npm run check` (wraps `tsc --noEmit && eslint .`)

  **Dependencies:** None

  **Files likely touched:** `web/package.json`, `web/tsconfig.json`,
  `web/eslint.config.js`, `web/index.html`, `web/src/main.ts`

  **Estimated scope:** Small: 4-5 files

- [x] **Task F2: WebSocket client + xterm.js live render**

  **Description:** Connect to the relay per T0's protocol, render incoming
  `output` messages into an xterm.js instance. Build against a mock relay
  stub emitting fake `output` messages until B4 is available.

  **Acceptance criteria:**
  - [ ] Terminal renders streamed output correctly (ANSI colors, cursor
        movement, resizing all handled by xterm.js as expected)
  - [ ] Connection failure/drop shows a visible state, not a silent blank
        screen

  **Verification:**
  - [ ] `vitest run --coverage` on the connection/parsing logic (≥80% of
        changed lines)
  - [ ] Manual check: against the mock stub, then against real B4 once
        available

  **Dependencies:** T0 (F1 for scaffold)

  **Files likely touched:** `web/src/ws-client.ts`, `web/src/terminal.ts`,
  `web/src/ws-client.test.ts`

  **Estimated scope:** Medium: 3-4 files

- [x] **Task F3: Password gate UI**

  **Description:** Prompt for password before the terminal view is shown,
  submit as `auth`, handle `auth_result` (success reveals terminal, failure
  shows an error and remaining-attempts/cooldown state per B5's rate
  limiting).

  **Acceptance criteria:**
  - [ ] Wrong password shows a clear error; terminal view stays hidden
  - [ ] Rate-limit cooldown is visibly communicated, not a silent failure
  - [ ] Correct password reveals the terminal view

  **Verification:**
  - [ ] `vitest run --coverage`
  - [ ] Manual check against mock stub and real B5

  **Dependencies:** F2

  **Files likely touched:** `web/src/auth-gate.ts`, `web/src/auth-gate.test.ts`

  **Estimated scope:** Small: 2 files

- [x] **Task F4: Take-control button + presence/control indicator**

  **Description:** UI for `take_control`, rendering `control_changed` and
  `presence` state — who's watching, who currently holds control, a
  one-click take-control action.

  **Acceptance criteria:**
  - [ ] Clicking "take control" sends the message and UI updates on
        `control_changed` broadcast, not just optimistically
  - [ ] Non-active-writer state visibly disables local keystrokes from
        being sent (matches B6's server-side drop, so UI doesn't lie about
        what will happen)

  **Verification:**
  - [ ] `vitest run --coverage`
  - [ ] Manual check with two browser tabs against real B6

  **Dependencies:** F3

  **Files likely touched:** `web/src/control.ts`, `web/src/control.test.ts`

  **Estimated scope:** Small: 2 files

- [x] **Task F5: Chat panel**

  **Description:** Send/receive `chat_message`, rendered in a panel beside
  the terminal. Explicitly never writes into the PTY input path (separate
  message type per T0, not reusable `input`).

  **Acceptance criteria:**
  - [ ] Messages from any connected client appear for all connected clients
  - [ ] Sending a chat message has no effect on the terminal/PTY state
  - [ ] Chat text is rendered via `textContent` (or equivalent escaping),
        never `innerHTML` — chat text is untrusted input from other
        session members (per `docs/protocol.md`)

  **Verification:**
  - [ ] `vitest run --coverage`
  - [ ] Manual check: confirm chat traffic never appears as terminal input
  - [ ] Manual check: a chat message containing `<img src=x onerror=alert(1)>`
        renders as literal text, does not execute

  **Dependencies:** F3

  **Files likely touched:** `web/src/chat.ts`, `web/src/chat.test.ts`

  **Estimated scope:** Small: 1-2 files

- [x] **Task F6: Mobile quick-action overlay**

  **Description:** The core differentiator for the launch pitch — big tap
  targets for yes/no/continue plus a short-text input, layered over the
  terminal view, sending as `quick_action` (→ `input` on the wire per T0).
  Must be genuinely usable one-thumb on a phone, not a shrunk desktop UI.

  **Acceptance criteria:**
  - [ ] Tap targets meet a real mobile usability bar (min 44x44px per iOS
        HIG / WCAG 2.5.5)
  - [ ] Quick actions actually reach the PTY as if typed (verified against
        real B6, not just the mock)
  - [ ] Layout doesn't break the raw terminal view underneath — both are
        usable, not one replacing the other

  **Verification:**
  - [ ] `vitest run --coverage`
  - [ ] Manual check: **on an actual phone**, not just a resized desktop
        browser — this task's whole point is the phone-in-bed use case

  **Dependencies:** F4

  **Files likely touched:** `web/src/mobile-overlay.ts`,
  `web/src/mobile-overlay.test.ts`, `web/src/styles/mobile.css`

  **Estimated scope:** Medium: 3 files

- [x] **Task F7: Kicked / session-ended states**

  **Description:** Handle `kicked` (kill switch fired) and `session_ended`
  — clear, distinct UI states, not the same generic "disconnected" message,
  since they mean different things to the viewer.

  **Acceptance criteria:**
  - [ ] `kicked` shows a state distinct from `session_ended` (e.g. "host
        ended your access" vs "session is over")
  - [ ] Neither state allows stale UI (old terminal content lingering as if
        still live)
  - [ ] `error.message` text (and any other server-provided text rendered
        here) uses `textContent`, never `innerHTML` — same rule as chat
        (per `docs/protocol.md`)

  **Verification:**
  - [ ] `vitest run --coverage`
  - [ ] Manual check against real B7/B8

  **Dependencies:** F4

  **Files likely touched:** `web/src/session-state.ts`

  **Estimated scope:** Small: 1 file

- [ ] **Task F8: Responsive layout + accessibility + performance pass**

  **Description:** Full mobile/desktop responsive pass across everything
  built so far; run the CONSTRAINTS.md accessibility and performance
  checks against a local build.

  **Acceptance criteria:**
  - [ ] `axe` reports zero critical or serious violations
  - [ ] `lighthouse` reports LCP ≤2500ms, CLS ≤0.1
  - [ ] Manual pass on at least one real phone confirms usability of every
        prior Frontend task's UI

  **Verification:**
  - [ ] `axe http://localhost:PORT --tags wcag2a,wcag2aa,wcag21aa`
  - [ ] `lighthouse http://localhost:PORT --output=json`

  **Dependencies:** F5, F6, F7

  **Files likely touched:** `web/src/styles/*.css`

  **Estimated scope:** Medium: several style files, no new logic

### Checkpoint: Frontend feature-complete
- [ ] All Frontend tests pass, `npm run check` clean
- [ ] axe/lighthouse thresholds met
- [ ] Review with founder before Phase 2 integration

---

### Phase 2: Integration (joint — after both checkpoints pass)

- [x] **Task I1: Wire Frontend against real Backend end-to-end**

  **Description:** Replace the Frontend's mock relay stub with the real
  relay, fix any protocol mismatches discovered (T0 is a contract, not a
  guarantee both sides interpreted it identically).

  **Acceptance criteria:**
  - [x] Every Functional Requirement (spec, FR 1–12) works end-to-end
        against the real Backend + Frontend together

  **Verification:**
  - [x] Manual check: full walkthrough of all 12 functional requirements
        (see agent-session log for the specific gaps found and fixed:
        presence/chat/env-var wiring, terminal keystroke passthrough +
        byte-limit truncation, frontend reconnect via resume token)

  **Dependencies:** Backend checkpoint, Frontend checkpoint

  **Files likely touched:** wherever mismatches are found (expect small
  fixes on both sides, not new features)

  **Estimated scope:** Medium (unpredictable — protocol mismatch fixes)

- [ ] **Task I2: Manual multi-device verification (human-only task)**

  **Description:** The actual "phone in bed" pitch, verified by a human,
  not an agent — start a session on a laptop, control it from a phone on a
  different network, confirm the full loop (watch, take control, chat,
  quick-actions, kill switch, reconnect after a brief network drop).

  **Acceptance criteria:**
  - [ ] Every item in the manual checklist below passes on a real phone,
        real laptop, real network (not localhost)

  **Verification (manual checklist):**
  - [ ] Start `getsloth claude` (or any agent) on laptop, open link on phone
  - [ ] Password gate works, rate limit triggers on wrong attempts
  - [ ] Live output visible on phone within perceived "instant" latency
  - [ ] Take control from phone, type/tap a quick action, laptop reflects it
  - [ ] Host reclaims control instantly via local override
  - [ ] Chat message from phone appears without affecting the terminal
  - [ ] Kill switch from host disconnects the phone viewer, session stays
        alive, new password re-invites it
  - [ ] Brief phone network drop (e.g. switch wifi to cellular) reconnects
        without re-entering the password

  **Dependencies:** I1

  **Files likely touched:** none (verification task)

  **Estimated scope:** N/A — manual

### Checkpoint: v0 works end-to-end on real devices
- [ ] I2's full manual checklist passes
- [ ] Founder sign-off before launch prep

---

### Phase 3: UI/Design Pass

Added after Phase 2 was scoped, explicitly sequenced to start only once
Phase 2's checkpoint passes — the founder's call: functional correctness
first, design pass second, not interleaved. Trigger: the functional
Frontend (F1-F8) works but its current visual design is rough — this is
a dedicated pass to actually design it, not a bug-fix task.

- [ ] **Task D1: Visual design pass on the web viewer**

  **Description:** A dedicated design session on top of the now
  functionally-complete and integrated viewer UI — terminal view,
  password gate, chat panel, take-control/presence indicator, mobile
  quick-action overlay. Not a redesign of interaction/behavior (that's
  already built and tested in F1-F8 and verified end-to-end in Phase 2)
  — visual/brand design on top of working functionality: layout,
  typography, color, spacing, the "lazy/mobile" brand feeling from
  `docs/ideas/getsloth.md`, consistent across light/dark and
  mobile/desktop.

  **Acceptance criteria:**
  - [ ] A coherent visual design system applied across every screen/state
        the viewer can be in (password gate, live session, kicked,
        session-ended)
  - [ ] Mobile layout specifically reflects the actual headline pitch
        (controlling from a phone), not just a shrunk desktop layout
  - [ ] No regression to F1-F8's tested behavior - this is styling, not
        a rebuild

  **Verification:**
  - [ ] Manual visual review (founder) - this is a design task, not one
        with a meaningful automated test
  - [ ] Re-run F1-F8's existing test suite to confirm no behavioral
        regressions from the styling pass

  **Dependencies:** Frontend checkpoint (F1-F8) AND Phase 2's checkpoint
  (I1, I2) - explicitly sequenced after integration is verified, not
  before or during

  **Files likely touched:** `web/src/**/*.css`, component markup/class
  names as needed to support the new design

  **Estimated scope:** Medium-Large, design-effort-bound rather than
  code-complexity-bound

### Checkpoint: Design pass complete
- [ ] Founder sign-off on visual design
- [ ] F1-F8 test suite still green after the styling pass

---

### Phase 4: Launch Prep

- [ ] **Task L1: LICENSE file (AGPL-3.0)**

  **Acceptance criteria:** [ ] Standard AGPL-3.0 text in `LICENSE` at repo root, matching the decision in `docs/ideas/getsloth.md`.

  **Dependencies:** None — can happen any time before going public

  **Files:** `LICENSE`

  **Estimated scope:** Small: 1 file

- [ ] **Task L2: README**

  **Description:** Install instructions (`getsloth` binary), usage
  (`getsloth <command>`), self-hosting the relay, a plain, non-hype
  description matching the tone decided earlier (no emoji-header slop).

  **Acceptance criteria:**
  - [ ] A stranger can go from README to a running session with no other
        context

  **Dependencies:** I2 (so instructions match what actually shipped)

  **Files:** `README.md`

  **Estimated scope:** Small: 1 file

- [ ] **Task L3: Deploy — Frontend to Vercel, relay to the founder's server**

  **Description:** Two separate deploy targets, decided above:
  - Frontend (`web/`) deploys to Vercel — standard static/SPA deploy, no
    special config expected.
  - `internal/relay` deploys to a server the founder provides credentials
    for — bare-bones (plain process/systemd or similar, not a managed
    PaaS). Point `relay.getsloth.dev` (or whatever subdomain) at it, and
    the Frontend's WebSocket client config at that same address. Confirm
    the default CLI onboarding path (no self-hosting required) works
    publicly.

  **Acceptance criteria:**
  - [ ] Frontend is reachable at its Vercel URL (or a custom domain
        pointed at it)
  - [ ] `getsloth <command>` from a machine with no special config reaches
        the public relay and produces a working shareable link that opens
        the Vercel-hosted viewer

  **Dependencies:** I2, and separately, founder providing relay server
  access (currently blocking only the relay half of this task, not the
  Frontend half or any earlier Backend work)

  **Files:** deployment config (e.g. `deploy/`, CI workflow if used)

  **Estimated scope:** Small–Medium, infra-dependent

- [ ] **Task L4: Demo capture**

  **Description:** Record the ~30s demo matching the actual headline pitch
  — shot from a phone, in bed, controlling a live agent session.

  **Acceptance criteria:** [ ] One clean GIF/video ready for the launch post

  **Dependencies:** L3 (needs the real public relay, not localhost)

  **Estimated scope:** N/A — manual/content task

- [ ] **Task L5: Go public**

  **Description:** Push the repo to a public remote, publish the launch
  post per the positioning decided in the ideation doc (solo/mobile/lazy
  headline, collaboration as secondary).

  **Acceptance criteria:**
  - [ ] Repo is public, README/LICENSE visible, demo linked from the post

  **Dependencies:** L1, L2, L3, L4

  **Estimated scope:** N/A — manual/publishing task

### Checkpoint: Shipped
- [ ] Repo public, relay live, launch post published
- [ ] This is the traction-test milestone — nothing beyond v0 gets built
      until this signal is in, per the explicit scope discipline in
      `docs/ideas/getsloth.md`

## Risks and Mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| Protocol contract (T0) drifts between what Backend and Frontend actually implement | High — blocks Phase 2 integration | T0 requires explicit sign-off from both before Phase 1 starts; I1 exists specifically to catch and fix drift |
| PTY behavior differs across terminal programs (raw mode, resize signals) | Medium | B2 tests against an actual interactive program, not just `echo`, before networking is layered on |
| Mobile UX (F6) looks fine on desktop-resized-to-mobile-width but fails on a real device | High — this is the whole launch pitch | F6 and F8 both require manual checks on an actual phone, called out explicitly, not just responsive CSS review |
| Relay hosting/deploy (L3) takes longer than expected, eating into Friday | Medium | L3 doesn't depend on L1/L2/L4, can start as soon as I1 is stable rather than waiting for full Phase 2 sign-off |
| Two agents (Claude, Pi) interpret CONSTRAINTS.md differently | Medium | Tooling (gofmt/tsc/eslint/gitleaks) is the enforcement, not agent judgment — see CONSTRAINTS.md notes; agent-session logs (Task convention already set up) surface drift for founder review |

## Decisions (previously open)

- **Agent split:** Claude takes Backend (Go CLI + relay, Phase 1A). Pi
  takes Frontend (TypeScript + xterm.js viewer, Phase 1B).
- **Hosting, split in two:** the Frontend (Task L3's web-viewer half)
  deploys to **Vercel**. The relay (Task L3's WebSocket-server half)
  deploys to a **server the founder will provide credentials for later —
  bare-bones only**, not a managed platform like Fly.io/Railway. Task L3
  in `tasks/plan.md` should be read as two deploy targets, not one, until
  those credentials arrive.
- **Protocol constants confirmed** (see `docs/protocol.md`):
  `RATE_LIMIT_MAX_ATTEMPTS` = 5, `RATE_LIMIT_COOLDOWN_MS` = 60000,
  `RECONNECT_WINDOW_MS` = 30000, `HOST_LOCK_WINDOW_MS` = 2000.

## Open Questions

- Relay server credentials/access — founder to provide before Task L3's
  relay half can actually be executed (Backend build/test work isn't
  blocked by this, only the final deploy step).
