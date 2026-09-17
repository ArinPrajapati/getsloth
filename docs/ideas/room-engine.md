# lazysesh

## Problem Statement

How might we let you watch, redirect, and control your own live AI coding agent session from any device, anywhere — starting a task on your laptop and continuing it from your phone in bed — regardless of which agent tool (Claude Code, Aider, Cursor, Codex CLI, etc.) is running, starting from a shippable v0 this week?

**Headline positioning, decided:** solo/mobile control of your own AI agent session, not team collaboration. Team collaboration (watch + take-control + chat between multiple people) is real, already built by the same architecture, and stays in as the secondary "also works with your team" line — but it is not the launch headline. This is an explicit reversal of an earlier decision (see "Sharpening" section below, which argued for leading with collaboration) — reversed because of a strong, specific, first-hand founder pain point: wanting to walk away from the desk while an agent runs and keep steering it from a phone, without a heavyweight tool like TeamViewer or the setup cost of SSH-key-based remote terminal tools.

**Why this is different from existing remote-terminal tools (Termius, Blink, a-Shell, Tailscale+Mosh), stated precisely so the pitch doesn't invite an obvious "why not just use X" reaction:** zero setup — no SSH keys, no app install on the viewing device. One command on the host machine, open a URL on the phone.

**The real product risk this creates, and the mitigation decided:** typing into a raw terminal on a phone is bad UX, and "lying in bed, one thumb" is the exact moment being pitched. A generic shrunk-down xterm.js view would feel like a tech demo, not a daily tool. Since most mid-session interaction with a coding agent is approving/denying a permission prompt, hitting continue, or a short redirect — not full command-line typing — **v0's mobile view gets big tap targets for common agent actions (yes / no / continue) plus a simple short-text input, layered over the raw terminal, not just a shrunk terminal emulator.** This is now in scope for this week's build, not a later polish item.

**Calibration note on expectations:** a "1k–3k GitHub stars easily" prediction was raised for this launch. Flagged once, deliberately: genuinely viral dev-tool launches do land there, but plenty of well-executed, resonant ones land in the low hundreds with real engaged users and that's still a good outcome for a traction test — worth not treating a lower number as failure.

## Origin: the YC problem statement

The original prompt (from a YC-style problem statement) asked for shared live agent sessions:

```
Multiple humans
     ↓
Same live agent session
     ↓
Watch it
Redirect it
Add context
Take over
Hand it off
Continue from the same state
```

The strongest line: anyone on a team should be able to "drop into the same live agent session to watch it work, redirect it, and hand it off."

This does **not** inherently require sharing an entire machine (terminal, filesystem, DBs, ports, worktrees). That's a separate, larger idea layered on top.

## The bigger idea we explored: Room Engine

We explored extending the YC ask into something much larger: a **multiplayer execution environment** where AI is one resource among many, not the core primitive — closer to "Discord/Slack + SSH + tmux + AI agents + remote development, sharing one execution context."

**Explicit design decision: AI is not a participant.** It's a resource, the same way Postgres is an important part of a system but not a participant in it. Participants are humans; AI (and terminals, DBs, logs, etc.) are resources rooms grant access to.

### Room Engine model

```
ROOM
├── Participants (humans only)
│   ├── Arun
│   ├── Abhay
│   └── Ayush
│
├── Machines
│   ├── Arun's laptop
│   ├── production-server-1
│   └── cloud-runner-4
│
├── Workspaces
│   ├── git worktree A
│   └── git worktree B
│
├── Sessions
│   ├── terminal-1
│   ├── terminal-2
│   ├── logs
│   ├── debugger
│   └── browser/preview
│
├── Services
│   ├── localhost:3000
│   ├── postgres:5432
│   └── redis:6379
│
└── Timeline (event log of everything that happened)
```

### Key architectural principles from that exploration

- **Machine attachment via a daemon (`roomd`):** every machine that joins a room runs a small daemon that connects it to the Room Engine and exposes only the resources authorized for that room. Users don't SSH independently — they join the room, and the room gives shared access to attached machines.
- **A room ≠ one machine.** Rooms are a persistent identity; machines attach and detach from them.
- **Sessions are multiplexed, not singular.** A room can contain many terminals, log streams, a debugger, a browser preview — not just one shared screen.
- **Workspaces (git worktrees) belong to a machine; the room grants access to them.** Editor-agnostic (NVim, VS Code, AI coding tools, plain shell).
- **Room networking:** each room gets a private network namespace. A service running on one member's machine (`localhost:3000`, Postgres, Redis, debugger ports, SSH-like TCP) can be registered and accessed by other room members through the room network, not a public URL — access follows room membership.
- **Event-driven state model:** `ROOM_CREATED`, `MEMBER_JOINED`, `MACHINE_ATTACHED`, `WORKSPACE_CREATED`, `TERMINAL_CREATED`, `COMMAND_STARTED/FINISHED`, `PROCESS_STARTED`, `PORT_EXPOSED`, `FILE_CHANGED`, `SERVICE_REGISTERED`, `MACHINE_DISCONNECTED`. Current state = snapshot + event stream (enables reconnect, audit, eventual replay). Large terminal output / files should be referenced as blobs/streams, not inlined into the event log.
- **Three state categories**, kept separate for scalability:
  - *Durable*: membership, machines, workspaces, sessions, permissions, important events, artifacts.
  - *Live*: terminal streams, logs, process output, file-watch events.
  - *Ephemeral*: cursor position, who's viewing what, typing indicators, presence heartbeats.
- **Authority split:** the Room Engine decides *who may* do something (permissions); the attached machine's `roomd` decides *how* it actually happens (local execution) — clients never get direct trusted access to arbitrary machines.
- **Permissions should be resource-level eventually**, not just room-level roles (Admin/Member/Viewer) — e.g. a member might get terminal + workspace access but not Postgres or production.
- **One-line definition:** *A Room Engine creates a persistent, permissioned, realtime collaborative layer over computers, workspaces, processes, services, and sessions.* AI plugs into this later as just another resource type, alongside terminal/worktree/logs/browser.

This full architecture is treated as the **long-term substrate**, not the v0 build.

## Sharpening: who is this for, and what's the real wedge

We stress-tested "build the full room engine now" vs. a thin vertical slice, and converged on a further-sharpened position:

- **Target users (beachhead):** small dev teams doing pairing/collaborative work, and — more importantly — **teams already using agentic coding tools** who want to co-drive the same agent session. This is the closest match to the literal YC ask and the easiest place to find first users.
- **Differentiator that matters most:** no existing tool (tmate/upterm, Teleport, VS Code Live Share, Warp, screen.so) treats an AI agent session as a first-class shareable resource. Everything else in the room-engine vision (permissions, multi-machine, DB tunneling) is differentiation *later*, not now.
- **Security posture for v0/v1:** trusted internal teams only. Coarse access (link = access), no per-resource ACLs, no SOC2/audit story yet. Per-resource permissions are deferred until the production/incident-response use case is actually being pursued (a later beachhead, not the first).
- **Explicit rejection of "build the full room engine as the MVP":** it was raised as an option but rejected — the network/tunneling layer alone (private room network for Postgres/Redis/ports) is comparable in scope to what Tailscale/ngrok/Teleport are whole products for. Building that before anyone has validated the room concept risks burning the runway on undifferentiated infrastructure.
- **The actual leverage / market-entry wedge, per explicit decision:** not the general room engine, not even generic terminal-sharing bundled with an agent. It's specifically **the shared live agent session** — because that is the literal YC-named gap, it's sellable and demoable on its own, and it doesn't require the room substrate to exist first.

## The key technical insight: agent-agnostic via PTY sharing

**Constraint from the user: the product must be agent-agnostic** (must work with Claude Code, Aider, Cursor, Codex CLI, Goose, etc. — not tied to one vendor's API).

**Insight:** almost every coding agent runs as a process inside a terminal. Instead of building per-agent integrations (slow, fragile, N different protocols), **share the PTY the agent is running in.** The tool never needs to know or care what's running inside it — this is the same trick tmate/upterm use for terminal sharing, just made multiplayer and browser-based. This gives agent-agnosticism for free.

### What "real-time, one session, work on it together" means in practice

Clarified explicitly by the user: **control means being able to work on one session in real time**, not a slow async handoff. Since a PTY only has one stdin, true concurrent multi-cursor typing (Google-Docs-style) isn't meaningful for a terminal. So the model is:

- **Everyone watches the same live output stream simultaneously** — true real time, no lag or turn-taking for *viewing*.
- **Control of the keyboard is a single active writer**, transferable **instantly** via a "take control" click — no approval flow, no delay (like tmate combined with Zoom/TeamViewer-style remote control handoff).

## "Inject context without taking control" — explored and resolved

We discussed a third interaction mode, beyond pure spectating and full keyboard takeover: what does a watcher do when they notice something but don't want to physically grab control?

Two models were considered:

- **Model A — Sidebar chat note, human-mediated.** A watcher types a note ("check the auth middleware, not the router") into a chat panel next to the terminal. The driver reads it and decides whether/how to act on it. Nothing touches the PTY automatically. Cheap to build, zero risk of garbling terminal input.
- **Model B — Direct injection into the input stream.** The note gets inserted as literal text into the driver's stdin/terminal input automatically, without the driver explicitly approving it in the moment. A genuinely new PTY-sharing mechanic, but riskier from a trust/UX standpoint (feels like someone typing into your terminal uninvited).

**Decision: Model A (simple chat window) is in scope for v0.** Model B is not being pursued for now — Model A plus watch + take-control is considered a complete v0 loop, and Model B was judged to possibly be solving a problem the other two mechanics already cover between them.

## Problems this solves in normal AI-assisted development today

1. **AI coding is currently a solo activity, even on a team.** No real-time visibility into an agent's reasoning/file changes until the PR shows up; the only current option is a laggy Zoom screen-share where the watcher can't do anything.
2. **You can't cheaply hand off a running agent task.** Long-running tasks (big refactor, migration) can't be picked up mid-task by a teammate with full context — they wait or start over reading the diff cold.
3. **"Adding context" mid-task is currently impossible without taking the keyboard.** No channel for a teammate to inject a quick pointer without physically taking over. (This is the pain Model A addresses.)
4. **Debugging escalation today is copy-paste, not shared reality.** People paste terminal output into Slack instead of looking at the live thing together.
5. **No way to learn by watching someone drive an agent well.** Weaker but real: a "Twitch for AI coding" content/virality angle — relevant specifically because the launch plan is social-media-driven traction testing.

Of these, #1 and #3 are the sharpest "hit this literally today" pains for a team already using AI agents. #5 is a distribution mechanism, not core value, but matters for the social launch.

## Decision: MVP for this week

**Goal:** ship a v0 by end of week, launch on social media, and measure organic traction/pickup as the validation signal — before investing in anything beyond this.

**In scope for v0:**
- CLI tool (e.g. `roomshare claude`, `roomshare aider`) that wraps any command and spawns it in a PTY.
- Instantly generates a shareable link.
- Browser view: live terminal (xterm.js + WebSocket), anyone with the link can watch in real time.
- "Take control" — single active writer, instant handoff on click, visible indicator of who currently has control.
- Simple chat window alongside the terminal (Model A) — lets any watcher drop a note visible to the driver, without touching the input stream.
- No accounts — the link *is* the access control (Figma/Google-Docs-link-share model).

**Explicitly out of scope for v0 (deferred):**
- Multi-machine, worktrees, DB/port tunneling, service registry — the full room-engine substrate.
- Granular/resource-level permissions — link access = full access for now.
- Persistent rooms, reconnect-after-days, full audit timeline — session ends when the terminal process ends.
- Any agent-specific integration — if it runs in a PTY, it works; no special-casing per agent.
- Direct context-injection into the input stream (Model B).

**What v0 is actually testing:** whether "watch someone's AI coding agent live and jump in" is a hook people share and use unprompted — not whether the full room-engine vision is correct. Nothing beyond v0 gets built until this signal is in.

## Rough roadmap (not a commitment — sequenced by validation)

- **v0 (this week):** Watch + take-control + chat, agent-agnostic via PTY sharing, link-based access. Ships to social media for a traction read.
- **v1 (next 2–4 weeks, contingent on v0 traction):** Session persistence — reconnect mid-session, replay a session after it ends. Answers the hand-off pain (#2) and supports the learning/content use case (#5).
- **v2:** Light identity + history — who was in a session, saved sessions per team, basic org accounts. Still one terminal per room, still all-or-nothing access.
- **v3 (the original Room Engine vision):** Multiple resources per room (more than one terminal, logs, worktrees), multi-machine attach via `roomd`, real resource-level permissions. This is where incident response / production debugging become legitimate use cases — deliberately deferred until the core watch/inject/handoff loop is proven to matter.

**Explicit scope discipline:** do not build v2/v3 until v0's traction shows people actually want to watch or be watched driving an agent. If the link-share/watch mechanic doesn't get organic pickup, the room-engine investment is not worth making regardless of architectural quality.

## Concrete delivery for Friday

- **`roomshare` CLI** (single binary or npm/npx install) that wraps any command: `roomshare claude`, `roomshare aider`, `roomshare npm run dev`, etc. The agent/program runs exactly as it would have — wrapping is invisible to the host's own experience.
- **A relay server** (WebSocket) the CLI connects to — the only "hosted" piece.
- **A web viewer** at the generated link: live terminal render (xterm.js), a chat panel, a "take control" button, and an indicator of who's watching / who currently has control.
- **A mobile-optimized layer on the same viewer:** big tap targets for common agent actions (yes / no / continue) plus a simple short-text input box, layered over the raw terminal view rather than relying on a shrunk-down terminal emulator alone. Added in scope given the headline is now solo/mobile control.
- No accounts, no persistent database beyond ephemeral session state — the link *is* the access control.
- README, a ~30-second demo GIF (ideally shot from a phone, in bed, matching the actual pitch), and a public GitHub repo — the whole point of Friday is the social/launch post.

## UX walkthrough

1. Host installs once (`npm i -g roomshare` or a curl script), then prefixes their normal command: `roomshare claude`.
2. Terminal prints: `Live at https://roomshare.dev/s/x7k2 — share this link`. Host keeps working normally — no change to their own experience.
3. Anyone with the link opens it in a browser and sees the terminal streaming live, no install needed on their end.
4. A viewer can type a note in the chat panel — surfaces to the host as a small non-blocking toast/inline note, doesn't touch the terminal (Model A, decided above).
5. A viewer can click "Take control" — transfers instantly, an indicator shows who's driving; previous driver becomes a viewer until they reclaim it.
6. **Decision: the host always has an instant local override to reclaim control** (e.g. a local keystroke, not routed through a viewer approval step) — otherwise a host's own session could be hijacked indefinitely by a viewer, which is a bad first impression for a launch post.
7. Session ends when the wrapped process exits — link shows "session ended," no replay in v0.

## Open source decision: yes, and specifically AGPL-3.0

Two separate reasons converged on this, not just "trust":

1. **Trust is existential here, not a nice-to-have.** The product's entire pitch requires someone to point a live terminal at your relay server. If they can't see what the relay does with that stream, nobody serious runs it against anything real.
2. **The PTY-sharing mechanic is not the moat.** tmate and upterm have proven this exact trick (share a terminal over a relay) for years — it's commodity. Keeping it closed protects nothing valuable yet, while costing the distribution that comes from GitHub stars / Show-HN / open-source credibility, which matters directly for this week's traction test.

**License specifically AGPL-3.0, not MIT** — because the stated business model is "sell the hosted/maintained version to people who don't want the hassle of self-hosting." MIT lets a cloud provider take the code, host it themselves, and out-compete on convenience without contributing anything back (the AWS-vs-open-source problem). AGPL requires anyone running a modified version as a network service to release their changes, protecting the future hosted-relay business while keeping the trust benefit intact. Same pattern used by Sentry, n8n, and similar open-core infra products, for the same reason.

**Scope decision:** open source the CLI + relay protocol + self-hosted relay in full, this week. Don't try to carve out a proprietary piece for v0 — there isn't one worth protecting yet, and doing so adds launch-week complexity for no benefit. The monetized layer (hosted convenience, later multiplayer-room features) comes after usage exists, not before.

## New idea: AI as a participant, not just a resource in the terminal

Distinct from the earlier "AI is not a participant, it's a resource" rule (which was about the agent *running inside* the shared terminal — the thing being watched/driven). This is a different claim: **a separate AI agent can be a client of the room itself** — able to watch, post notes in chat, or even take control, the same way a human viewer can. Both can be true simultaneously: the in-terminal agent stays a resource; a second, room-level agent becomes a participant.

This is only cheap if the room's client protocol is built client-agnostic from the start — i.e., not hardcoded to "browser + human." If watch / chat / take-control are exposed as a generic protocol, a bot speaking that protocol is structurally already a valid client, no special feature required.

**Why it's a strong marketing claim:** "agent-agnostic terminal sharing" alone isn't sharp once tmate/upterm are on the table — that part is commodity. "Built for humans and AI to collaborate in the same session, not just humans" is a claim nobody else is making, and it's ownable specifically because of the protocol design choice above.

**What it unlocks (future, not v0 build target):**
- A reviewer/monitor bot that watches a session and posts suggestions automatically (e.g. "this looks like it's about to touch prod config").
- Two agents effectively collaborating in one terminal — one driving, another commentating or standing by to take over.
- Headless monitoring — something watches many rooms across a team without a human present, for alerting.
- Strongest possible launch-post visual: a human and an AI both visibly present in the same live session — more novel than "watch my terminal" alone.

**Decision pending:** whether Friday's launch includes only the *positioning* (protocol built agent-agnostically, claimed in README/copy, no bot shipped) or also a *working proof* (a trivial example bot script using the same protocol, posting one canned chat observation, for a real screenshot/GIF). Leaning toward the working proof if it's genuinely a few hours given the protocol is already generic — not worth it if it distracts from the core loop shipping by Friday.

## New use case: solo remote access to your own session

Because the viewer is just a browser hitting a link, the exact same v0 build gives you solo remote access to your own terminal/agent session from any device, anywhere — no second person required. E.g.: kick off a long Claude Code task before lunch, check on it and nudge it from your phone.

**Why this matters for launch:** it's a *stronger* solo demo GIF than team collaboration, because it doesn't need a second person cooperating on camera. Proposed as the **onboarding hook**: "try it alone first — start a session, open the link on your phone" is lower friction than "convince a teammate to join," and shows the same core mechanic.

**Honest caveat, not a footnote:** "access from anywhere, any device" is only true while the host machine is awake, unlocked, and network-reachable — hence "make sure your PC doesn't sleep." A durable "always-on, from anywhere" story requires running the session on something always-on (a cloud runner), not a laptop — which is exactly the "Machines" piece of the original Room Engine vision (v3), not something v0 needs to solve.

**Positioning decision:** don't lead marketing with "remote access from anywhere" — that's the exact pitch of Tailscale SSH, Teleport, Termius, Mosh, and tmate itself, a mature crowded space with zero differentiation for this product on that axis alone. Use it as a secondary use case and the low-friction onboarding path; the headline stays "watch and collaborate on a live AI agent session, together."

## Naming — decided: `lazysesh`

Once the headline flipped to solo/mobile/lazy control of your own agent session (see above), naming was re-run to match that feeling, not just the mechanic.

Candidates considered and rejected:
- `Sesh` (earlier front-runner, before the positioning flip) — bare name crowded: npm taken (dead abandoned package, still occupies the name), `github.com/sesh` taken, `sesh.com`/`sesh.io`/`sesh.live` all taken/live.
- `Cotty`, `Dropin`, `Tandem`, `Mobshell` — considered during the collaboration-headline phase, superseded by the positioning flip.
- `Beam` — rejected, collides with Beam Cloud (a GPU compute platform) in the same developer audience.
- `Hammock` — strong emotional fit (direct nod to "Hammock Driven Development," a well-loved reference in this exact developer crowd) but bare name taken on both npm and GitHub.
- `Loaf` — casual, fits the feeling, but bare name taken on both npm and GitHub.

**Decided: `lazysesh`** — combines the earlier `sesh` front-runner with the laziness/comfort angle the story is actually about, and rides the same warm brand affinity developers already have for "lazy"-prefixed tools in this exact terminal/CLI crowd (LazyVim, lazygit are both beloved precedents, not a negative connotation here).

**Verified clean:** npm (404, free), GitHub (404, free), `lazysesh.dev` and `lazysesh.sh` both returned no response (likely available — confirm at an actual registrar before purchasing, this was only a basic reachability check, not a real WHOIS lookup).

## Abuse / safety guardrails — v0 (decided)

Previously deferred, now decided as the minimum bar for Friday — more guardrails come later, this is not the final security model:

- **Kill switch:** host can instantly disconnect all current viewers/participants. Session stays alive (the wrapped process keeps running) so the host can re-lock and reshare with a new password — it is not the same as ending the session (that already happens when the host exits the wrapped process/terminal).
- **Simple password lock on the session** — required to view/join, not just a bare link.
- **Password is verified locally on the host's machine, never sent to or checked by the relay.** The relay stays a dumb, blind pipe — it forwards the attempt and forwards the pass/fail result but never learns or stores the password itself. This is an architectural choice, not just a feature: it reinforces the open-source/AGPL trust story, since even a self-hoster or relay operator can't intercept session access.
- **Rate limiting on failed password attempts** (e.g. a handful of wrong guesses drops/blocks that client for a period) — added because local verification makes this essentially free, and a plain password without it is brute-forceable in seconds against a short-lived session.

## Key assumptions to validate

- [ ] Teams/individuals will click a shared link to watch a live AI agent session, and some will actually take control or use chat — not just pure lurking.
- [ ] "Watch my AI agent work live" is a hook people share unprompted on social media (the actual traction test).
- [ ] The chat-alongside-terminal (Model A) is sufficient for the "add context" need — validated only after real usage, not just reasoning.
- [ ] PTY-sharing is genuinely sufficient for "agent-agnostic" — no agent so far assumed to need anything beyond stdin/stdout/stderr passthrough (worth a quick check against how Cursor's agent mode and Codex CLI actually run before building).

## Open questions

- Exact mechanics of instant control handoff when the current driver is mid-keystroke (e.g., partially typed command) — does control transfer mid-line cleanly?
- Confirm kill-switch semantics: disconnect-all-but-keep-session-alive (current assumption, see guardrails section above) vs. terminate the whole session.
- Naming for the product/CLI.
- Whether the social-media launch post should emphasize "watch" (spectator/content angle) or "take control together" (collaboration angle) — these could attract different audiences and affects what gets built first within v0.
- Whether Friday's launch includes a working example bot client (proof of "AI as participant") or just the positioning claim — pending a time-cost check against the rest of v0.
- Whether the launch post gives the solo remote-access use case equal billing with team collaboration, or stays secondary/onboarding-only as currently leaning.
