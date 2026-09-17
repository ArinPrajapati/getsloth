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

- [ ] B5: Password auth — local verification + rate limiting
- [ ] B6: Control handoff — single active writer + host override
- [ ] B7: Kill switch
- [ ] B8: Session teardown + reconnect resilience

**Checkpoint:** Backend feature-complete.

## Phase 1B: Frontend (TypeScript + xterm.js) — Browser Viewer — assigned to Pi
- [ ] F1: Project scaffold + tooling wired to CONSTRAINTS.md
- [ ] F2: WebSocket client + xterm.js live render
- [ ] F3: Password gate UI
- [ ] F4: Take-control button + presence/control indicator
- [ ] F5: Chat panel
- [ ] F6: Mobile quick-action overlay (yes/no/continue + text)
- [ ] F7: Kicked / session-ended states
- [ ] F8: Responsive + accessibility + performance pass

**Checkpoint:** Frontend feature-complete.

## Phase 2: Integration
- [ ] I1: Wire Frontend against real Backend end-to-end
- [ ] I2: Manual multi-device verification (phone + laptop, real network) — human-only task

**Checkpoint:** v0 works end-to-end on real devices, founder sign-off.

## Phase 3: Launch Prep
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
