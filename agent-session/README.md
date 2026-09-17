# Agent session logs

Every agent working in this repo (Claude, Pi, or any other harness) keeps a
running log of its own work session in this directory. These files are
**gitignored** — they're a private working record between the founder and
whichever agent is active, not part of the shipped project.

## Why

With multiple agents working separate, clearly-scoped tasks in parallel, a
record of what each one was doing, why, and what it ran into is the only way
to reconstruct the reasoning later — decisions made mid-session, gotchas
hit, new ideas raised, and instructions given in chat don't survive if
they're not written down somewhere durable.

## File naming

```
agent-session/<agent-name>_<task-slug>_<YYYY-MM-DD>.md
```

Examples:
```
agent-session/claude_relay-server_2026-09-18.md
agent-session/pi_frontend-viewer_2026-09-18.md
```

One file per agent per task per day. If a task spans multiple days, keep
appending to the same file rather than starting a new one, unless the date
change is worth marking as a new entry within it.

## What goes in it

At minimum, each session log should capture:

- **What** the agent is doing (the task, in a sentence or two)
- **Why** — the reasoning or instruction that led to this task
- **Decisions made** during the session, and why (especially anything not
  already in `CONSTRAINTS.md` or `docs/ideas/room-engine.md`)
- **Gotchas / surprises** hit along the way — things that would trip up the
  next agent or the founder if undocumented
- **New ideas or instructions given during the session** by the founder that
  aren't yet reflected in the spec or constraints files
- **Open questions** left unresolved at the end of the session

Plain markdown, append-as-you-go. This is a working notebook, not a
polished document — terse notes beat nothing.
