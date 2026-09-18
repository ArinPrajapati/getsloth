# getsloth v0 — Task List

Full detail (acceptance criteria, verification, files) in `tasks/plan.md`.
This file is the checkbox tracker.

## Phase 0: Shared Contract
- [x] T0: Define and document the WebSocket protocol (`docs/protocol.md`)

**Checkpoint:** contract approved by founder before Phase 1 starts. ✅
Approved after 3 review rounds (Pi) — each round caught a real gap:
plaintext password in transit, then a relay-MITM hole in the fix for
that, then a missing relay→host input-forwarding path. All fixed, plus
final non-blocking clarifications (reconnect/active-writer interaction,
token format). Phase 1 can start.

## Phase 1A: Backend (Go) — CLI + Relay — assigned to Claude
- [x] B1: Go module scaffold + tooling wired to CONSTRAINTS.md
- [x] B2: PTY wrapping — `getsloth <command>` works standalone
- [x] B3: Relay server skeleton — sessions, host + viewer connect
- [x] B4: Live output streaming (host → relay → viewer)

**Checkpoint:** backend can stream, no auth/control yet. ✅ Verified
end-to-end with real binaries (not just unit tests) — measured ~1-2ms
output latency, well under the 300ms target.

- [x] B5: Password auth — local verification + rate limiting
- [x] B6: Control handoff — single active writer + host override
- [x] B7: Kill switch
- [x] B8: Session teardown + reconnect resilience

**Checkpoint:** Backend feature-complete. ✅ B1-B8 all done, 91.7%
coverage on internal/relay, real end-to-end tests (not just unit tests)
for the crypto auth flow, control handoff, and kill switch. Open items:
confirm module path before going public, manual interactive-terminal
check still needed, signal-based host triggers (kill switch/reclaim)
aren't discoverable in-session yet.

## Phase 1B: Frontend (TypeScript + xterm.js) — Browser Viewer — assigned to Pi
- [x] F1: Project scaffold + tooling wired to CONSTRAINTS.md
- [x] F2: WebSocket client + xterm.js live render
- [x] F3: Password gate UI
- [x] F4: Take-control button + presence/control indicator
- [x] F5: Chat panel
- [x] F6: Mobile quick-action overlay (yes/no/continue + text) — logic/tests
      by Pi, styling/layout completed by Claude. Real-phone manual check
      (plan.md's explicit acceptance criterion) still outstanding — human
      task, not agent-verifiable.
- [x] F7: Kicked / session-ended states — by Claude
- [ ] F8: Responsive + accessibility + performance pass

**Checkpoint:** Frontend feature-complete.

## Phase 2: Integration
- [ ] I1: Wire Frontend against real Backend end-to-end
- [ ] I2: Manual multi-device verification (phone + laptop, real network) — human-only task

**Checkpoint:** v0 works end-to-end on real devices, founder sign-off.

## Phase 2.5: Session modes + terminal geometry

Detailed product and implementation contract:
`docs/session-modes-v0-plan.md`.

- [ ] M0: Approve Remote mode / Group mode boundary and update protocol
- [ ] M1: CLI mode selection + correct host geometry lifecycle
- [ ] M2: Relay viewer-limit, read-only group policy, and canonical geometry
- [ ] M3: Browser session-state/control protocol updates
- [ ] M4: Canonical-grid terminal rendering with fit/pan mobile viewing
- [ ] M5: Mode-aware status bar and intentional mobile typing flow
- [ ] M6: Multi-device shell/TUI verification

**Checkpoint:** phone remote control is correct in Remote mode; Group mode is
stable host-sized viewing/chat without remote terminal control.

## Phase 3: UI/Design Pass
Added after Phase 2 was scoped — explicitly sequenced to start only
once Phase 2's checkpoint passes (founder's call: correctness first,
design second). Not a redesign of behavior, a visual/brand design pass
on top of the already-working, already-integrated Frontend.
- [ ] D1: Visual design pass on the web viewer

**Checkpoint:** Design pass complete — founder sign-off, F1-F8 tests
still green.

## Phase 4: Launch Prep
- [ ] L1: LICENSE file (AGPL-3.0)
- [ ] L2: README
- [ ] L3: Deploy — Frontend to Vercel, relay to founder-provided server (bare-bones)
- [ ] L4: Demo capture (phone, in bed, matching the actual pitch)
- [ ] L5: Go public — push repo public, publish launch post

**Checkpoint:** Shipped. Traction-test milestone — nothing beyond v0 until
this signal is in.

## Resolved
- Agent split: Claude = Backend, Pi = Frontend
- Reconnect window: 30s, rate-limit threshold: 5 attempts (see docs/protocol.md)
- Hosting: Frontend → Vercel, relay → founder-provided server (bare-bones)

## Open Questions (need founder input)
- Relay server credentials — needed before Task L3's relay deploy, doesn't
  block Backend build/test work
