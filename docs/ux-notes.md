# UX notes — viewer

Working notes for the UX/UI pass, to be done in `worktree/ux-design` once
Backend integration and end-to-end testing are further along. Not a spec,
not binding — just captured thinking so it isn't lost between sessions.

## Why does the host open the link on a second device?

Question raised 2026-09-17: why would the host copy the share link off
their PC and open it on a phone (or another laptop)? Their reasons should
shape the viewer's default UI state, not just its feature set.

Candidate reasons, roughly in order of how central each is to the actual
pitch (mobile-first, quick-actions as "the core differentiator," phone-in-
bed demo — see `docs/ideas/getsloth.md`):

1. **The agent is running something long and unattended, and the host has
   physically left the PC.** Couch, bed, out for coffee — the phone is
   whatever's in hand. This is passive, at-a-distance monitoring, not
   active dual-device work. The most common case by the product's own
   framing.
2. **The agent hit a decision point and needs a fast yes/no/continue, and
   returning to the original machine isn't convenient right now.** This is
   the specific reason quick-actions (F6) exist — a *moment* trigger, not
   ambient watching. Usually paired with #1: mostly idle, then a short
   burst of interaction.
3. **Showing or handing off visibility to someone else on another device**
   without giving them the laptop itself. Secondary — maps more to chat
   than to quick-actions.
4. **Their second device is just more convenient in-hand** than walking to
   or reaching the desktop. Same root cause as #1, framed as convenience
   rather than necessity.

**Working conclusion:** the dominant pattern is #1 + #2 — long idle
stretches of passive glancing, punctuated by short, low-friction
interactions. The current viewer doesn't reflect that: it renders the full
terminal, chat panel, and control panel all open at once, as if the phone
were a second desktop the host is actively working from the whole time.

**Implication for the UX pass:** consider a more collapsed/ambient default
state for the mobile viewer — e.g., a compact "still running, nothing
needed" indicator as the resting state, with the full terminal/chat
expanding on demand rather than everything rendered open simultaneously.
Not scoped or designed yet, just the direction worth exploring first.
