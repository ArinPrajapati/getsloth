# getsloth v0 — WebSocket Protocol

This is the contract between three parties: the **host CLI** (Go), the
**relay server** (Go), and the **browser viewer** (TypeScript). It is the
seam between the two parallel workstreams in `tasks/plan.md` — Backend
(Phase 1A) implements the host + relay side, Frontend (Phase 1B) implements
the viewer side. Neither should need to read the other's source to build
against this document.

Both sides implement exactly what's written here. If an implementation
needs to deviate, this file changes first.

**Revision note:** this version incorporates a review pass that caught a
real gap between this doc and the product decision in
`docs/ideas/room-engine.md` ("password verified locally on the host,
**never sent to or checked by** the relay") — the previous draft had the
relay receiving the plaintext password in transit. See
[Auth flow](#auth-flow) for the fix.

## Design rules

1. **Every message is a JSON object with a `type` field** (discriminated
   union) and a `v` field (protocol version, `1` for all of v0). Unknown
   `type` values must be ignored, not crash the reader.
2. **Wire field names are `snake_case`.**
3. **One error shape for everything that isn't a defined success path** —
   see [Errors](#errors).
4. **Validate at the boundary.** The relay validates every inbound message
   (`type` known, role allowed to send it, session exists, size limits —
   see [Limits](#limits)) before acting on it or forwarding it.
5. **TypeScript interfaces below are the schema of record for this v0.**
   Required vs. optional (`?`) fields and literal string unions carry the
   same constraints a JSON Schema document would. No separate
   `.schema.json` files are maintained in parallel — one source of truth,
   not two that can drift. (This is a deliberate deviation from
   `tasks/plan.md`'s literal "JSON schema" wording — flagged for the
   founder to override if real schema files are wanted.)

## Connection roles

```
wss://relay.getsloth.dev/ws/host                 → host CLI connects here to create a session
wss://relay.getsloth.dev/ws/viewer/<session_id>  → browser viewer connects here to join one
```

A **host** connection creates and owns exactly one session for its
lifetime. A **viewer** connection joins an existing session and must
authenticate (see [Auth flow](#auth-flow)) before receiving anything beyond
`auth_pubkey` and `error` messages.

## The relay-blind boundary — how it's actually guaranteed

Per `CONSTRAINTS.md`'s architecture rule ("the relay must never contain
password-comparison or auth-decision logic"), and per the explicit product
decision that the password is **never sent to** the relay, not just never
checked by it:

- **The host generates a fresh ephemeral key pair when it creates the
  session** (recommend P-256 ECDH — natively supported by both Go's
  standard library `crypto/ecdh`, added in Go 1.20, and every modern
  browser's native `SubtleCrypto`, so neither side needs a third-party
  crypto dependency; confirm during implementation and fall back to a
  well-vetted library only if a real gap turns up). The private key never
  leaves the host process and is discarded when it exits — no persistence,
  consistent with the "no state beyond session lifetime" requirement.
- **The relay receives the public key once and hands it to every viewer
  that connects**, before auth (`auth_pubkey`, below).
- **The viewer encrypts its password attempt to that public key** using
  its own fresh one-time key pair (standard ECDH + AEAD: derive a shared
  secret, encrypt with AES-GCM or equivalent). What crosses the wire to
  the relay is ciphertext plus the viewer's one-time public key — the
  relay structurally cannot decrypt it, not merely "chooses not to."
- **The relay only ever sees a verdict** (`auth_response.ok`) from the
  host. It counts verdicts for rate limiting, never password content.
- **There is no password state on the relay**, encrypted or otherwise, and
  no "rotate password" message exists: the kill switch (below) invalidates
  relay-issued *tokens*; whatever the host compares future attempts
  against afterward is entirely its own local state.
- `output`, `chat_message`, and `input` are relayed without inspection of
  content, only inspection of routing metadata (`type`, sender role,
  current active-writer status, size limits).

## Auth flow

```
Viewer                          Relay                          Host CLI
  │                                │                                │
  │◄─── auth_pubkey ───────────────│ (cached from session_created)  │
  │                                │                                │
  │── auth (ciphertext + pubkey) ─►│                                │
  │                                │── auth_request (req_id, ──────►│  (host derives shared
  │                                │    ciphertext, client_pubkey)  │   secret, decrypts,
  │                                │◄── auth_response (req_id, ok) ─│   compares locally)
  │◄─── auth_result (token) ───────│ (relay generates token itself) │
```

1. Viewer connects to `/ws/viewer/<session_id>`. Relay immediately sends
   `auth_pubkey` (or `error{SESSION_NOT_FOUND}` and closes, if the session
   doesn't exist).
2. Viewer encrypts its password attempt to the host's public key and sends
   `auth`.
3. Relay checks the viewer isn't currently rate-limited for this
   `(session_id, remote_address)` pair (see [Rate limiting](#rate-limiting)
   below — keyed on more than the ephemeral connection, so reconnecting
   doesn't reset the counter). If limited, relay responds directly with
   `auth_result{ok:false, code:"RATE_LIMITED", retry_after_ms}` and does
   **not** contact the host.
4. Otherwise relay forwards `auth_request` to the host (ciphertext and the
   viewer's one-time public key, opaque to the relay), tagged with a
   `request_id` to correlate concurrent viewers.
5. Host decrypts locally and replies `auth_response{request_id, ok}`.
6. **On `ok:true`, the relay itself generates a random session token**,
   records it against that connection, and sends it in `auth_result`. The
   host never sees or generates this token — this was an inconsistency in
   the previous draft (host-generated token, but relay validating it on
   resume with no way to know it) and is fixed by making the relay the
   sole issuer.
7. On `ok:false`, relay increments the `(session_id, remote_address)`
   failure counter. At the configured threshold
   (`RATE_LIMIT_MAX_ATTEMPTS` = 5) further attempts get `RATE_LIMITED`
   without contacting the host until the cooldown
   (`RATE_LIMIT_COOLDOWN_MS` = 60000) expires. The connection is **not**
   closed on rate limit — the viewer stays connected and can see the
   cooldown countdown.

## Rate limiting

Keyed on `(session_id, remote_address)`, not on the WebSocket connection
object — reconnecting must not reset the counter, otherwise the limit is
trivially bypassed. This is a v0-appropriate mitigation, not a complete
one: remote address can be spoofed or shared behind NAT. Acceptable given
the "trusted internal teams" v0 threat model; revisit if abuse in the wild
shows this is insufficient.

## Control model

Exactly one connection is the "active writer" at any time. Initially, the
host. Any authenticated connection may send `take_control`; the relay
reassigns the active writer and broadcasts `control_changed`.

**Host `take_control` is authoritative and starts a short lock window.**
When the host sends `take_control`, the relay reassigns immediately *and*
rejects any non-host `take_control` received within
`HOST_LOCK_WINDOW_MS` (2000ms) afterward, responding
`error{code:"NOT_ACTIVE_WRITER", message:"host recently reclaimed
control"}` to the rejected sender instead of reassigning. This is what
makes "the host always has an instant override" actually true — without
it, a viewer's `take_control` sent immediately after a host reclaim would
silently undo it. A viewer's own `take_control` is not similarly
authoritative and can be immediately taken back by anyone, including
another viewer — only the host gets the lock window.

`input` messages are only applied to the PTY if the sender is the current
active writer; from anyone else they're silently dropped by the relay (not
forwarded to the host). The Frontend does not need to duplicate this logic
beyond disabling its own UI as a courtesy.

## Quick actions — a Frontend concept, not a wire message

The mobile yes/no/continue/text UI (Task F6) does **not** have its own
wire message type. The Frontend translates a tap directly into an `input`
message before sending, using this fixed mapping:

| Action | `input.data` (before base64, see Limits) |
|---|---|
| yes | `y\r` |
| no | `n\r` |
| continue | `\r` (plain Enter — most CLI "press enter to continue" prompts expect nothing else; there's no universal "continue" keystroke) |
| text | `{typed text}\r` |

This was previously a separate `quick_action` message type; collapsing it
into `input` means there is exactly one PTY-input path on the wire, gated
by active-writer status in exactly one place, instead of two message types
that both need the same gate applied identically. This mapping is a
best-effort default, not guaranteed to match every agent's actual prompt
convention — acceptable for v0, revisit if it's a real source of friction
after launch.

## Reconnect

A viewer that briefly loses network sends `resume{token}` instead of
`auth{...}`. If the relay-issued token is still valid (session alive, not
invalidated by a kill switch, within `RECONNECT_WINDOW_MS` = 30000) the
relay responds `auth_result{ok:true}` immediately — no host round-trip,
since the relay itself is the token's issuer and authority.

## Limits

Hard caps, enforced by the relay; violation closes the connection with
`error{code:"BAD_REQUEST"}` and close code `4002` (see
[Socket lifecycle](#socket-lifecycle)):

| Field | Limit |
|---|---|
| Any single WebSocket frame | 256 KiB |
| `chat_message.text` | 2000 bytes (UTF-8) |
| `input.data` (decoded) | 4096 bytes |
| `output.data` (decoded, per message) | 65536 bytes — host/relay must chunk larger output across multiple messages, not send one giant frame |
| `auth.ciphertext_base64` (decoded) | 1024 bytes |
| `display_name` | 64 bytes |

## Socket lifecycle

Custom WebSocket close codes (application range, per RFC 6455):

| Code | Meaning | Also preceded by |
|---|---|---|
| 4000 | `session_ended` | `SessionEndedMsg` broadcast to all viewers first |
| 4001 | `kicked` | `KickedMsg` sent to the affected viewer first |
| 4002 | `bad_request` | `ErrorMsg{code:"BAD_REQUEST"}` sent first when possible |
| 4003 | `session_not_found` | `ErrorMsg{code:"SESSION_NOT_FOUND"}` sent first |
| 4004 | `unsupported_version` | `ErrorMsg{code:"UNSUPPORTED_VERSION"}` sent first |

Rules:
- **`UNAUTHORIZED`** (a message requiring auth arrives before `auth`/`resume`
  succeeds) does **not** close the connection — it's a normal race (e.g.
  the client fires a message before an earlier auth response lands), and
  the client can simply authenticate and retry.
- **`RATE_LIMITED`** does not close the connection — see [Auth
  flow](#auth-flow).
- **`NOT_ACTIVE_WRITER`** (dropped `input`, or a `take_control` rejected by
  the host lock window) does not close the connection — it's routine.
- Every other relay-initiated close sends the relevant typed message
  first, then closes with the matching code above, so a client that misses
  the JSON frame (e.g. an abrupt network drop) can still distinguish the
  reason from the close code alone.

## Protocol version mismatch

If a received message's `v` is not `1`, the receiver responds
`error{code:"UNSUPPORTED_VERSION", message:"..."}` and closes with code
`4004`. There is no negotiation in v0 — a version mismatch is a hard stop,
not a fallback.

## Message reference

All messages share this envelope:

```typescript
interface Envelope {
  v: 1;
  type: string; // discriminator, see below
}
```

### Relay → Viewer (before auth)

```typescript
// Sent immediately on connect, before any auth exchange
interface AuthPubkeyMsg extends Envelope {
  type: "auth_pubkey";
  public_key_base64: string; // host's ephemeral ECDH public key for this session
}
```

### Viewer → Relay

```typescript
// Auth attempt — password never crosses in plaintext, see "relay-blind boundary"
interface AuthMsg extends Envelope {
  type: "auth";
  client_pubkey_base64: string;   // viewer's fresh one-time ECDH public key
  ciphertext_base64: string;      // password attempt, encrypted to the host's public key
  display_name?: string;          // optional, shown in presence (e.g. "Alex"); relay assigns "Viewer N" if omitted
}

// Resume a previously authenticated connection after a brief disconnect
interface ResumeMsg extends Envelope {
  type: "resume";
  token: string; // relay-issued, from a prior auth_result
}

// Raw keystrokes (and translated quick-actions) — only applied if sender
// is the current active writer
interface InputMsg extends Envelope {
  type: "input";
  data_base64: string; // PTY input is not guaranteed valid UTF-8; base64 avoids corrupting or rejecting binary-ish terminal data
}

// Request to become the active writer
interface TakeControlMsg extends Envelope {
  type: "take_control";
}

// Chat message — never applied to the PTY under any circumstance.
// Frontend renders this via textContent (or equivalent escaping), never
// innerHTML — chat text is untrusted input from other session members.
interface ChatMsg extends Envelope {
  type: "chat_message";
  text: string;
}
```

### Host → Relay

```typescript
// Sent once, immediately after the WebSocket opens, to create a session
interface SessionCreateMsg extends Envelope {
  type: "session_create";
  public_key_base64: string; // host's ephemeral ECDH public key for this session
}

// Terminal output chunk, forwarded to every authenticated viewer verbatim
interface OutputMsg extends Envelope {
  type: "output";
  data_base64: string; // see InputMsg — PTY output is not guaranteed valid UTF-8 either
}

// Reply to a relay-forwarded auth_request
interface AuthResponseMsg extends Envelope {
  type: "auth_response";
  request_id: string;
  ok: boolean;
  // no token field — the relay issues the token itself, see Auth flow step 6
}

// Host-triggered kill switch — disconnects all viewers, session stays alive
interface KillSwitchMsg extends Envelope {
  type: "kill_switch";
}

// Host explicitly ending the session (in addition to the implicit case of
// the host connection just closing, e.g. the wrapped process exiting)
interface EndSessionMsg extends Envelope {
  type: "end_session";
}
```

Host also sends `take_control` and `chat_message` using the exact shapes
above — the host is just another connection with the `host` role attached
server-side (which is what grants its `take_control` the lock-window
authority described in [Control model](#control-model)).

### Relay → Host

```typescript
interface SessionCreatedMsg extends Envelope {
  type: "session_created";
  session_id: string;
}

// Forwarded auth attempt for the host to decrypt and verify locally
interface AuthRequestMsg extends Envelope {
  type: "auth_request";
  request_id: string;
  client_pubkey_base64: string;
  ciphertext_base64: string;
}
```

### Relay → Viewer (after connect)

```typescript
interface AuthResultMsg extends Envelope {
  type: "auth_result";
  ok: boolean;
  token?: string;                          // present when ok === true, relay-issued
  code?: "AUTH_FAILED" | "RATE_LIMITED";   // present when ok === false
  retry_after_ms?: number;                 // present when code === "RATE_LIMITED"
}

interface KickedMsg extends Envelope {
  type: "kicked";
  reason: "kill_switch";
}
```

### Relay → All authenticated connections (host + every authenticated viewer)

```typescript
interface OutputBroadcastMsg extends Envelope {
  type: "output";
  data_base64: string;
}

interface ControlChangedMsg extends Envelope {
  type: "control_changed";
  active_writer_id: string;
  active_writer_role: "host" | "viewer";
}

interface ChatBroadcastMsg extends Envelope {
  type: "chat_message";
  sender_id: string;
  sender_role: "host" | "viewer";
  sender_display_name?: string;
  text: string;
}

interface PresenceMsg extends Envelope {
  type: "presence";
  connections: Array<{
    id: string;
    role: "host" | "viewer";
    display_name?: string; // Frontend labels the local user's own entry "You" client-side by comparing connection id — not a protocol concern
    is_active_writer: boolean;
  }>;
}

interface SessionEndedMsg extends Envelope {
  type: "session_ended";
  reason: "process_exited" | "host_ended" | "host_disconnected";
}
```

## Errors

```typescript
interface ErrorMsg extends Envelope {
  type: "error";
  code:
    | "SESSION_NOT_FOUND"
    | "UNAUTHORIZED"
    | "NOT_ACTIVE_WRITER"
    | "UNSUPPORTED_VERSION"
    | "BAD_REQUEST";
  message: string; // human-readable, not for programmatic branching. Frontend
                    // renders this via textContent, never innerHTML — same
                    // rule as chat, since some error text may echo
                    // client-influenced context.
}
```

## Functional requirement → message map

| FR | Requirement | Message(s) |
|---|---|---|
| 1 | Host wraps any command in a PTY | N/A — CLI-local |
| 2 | Prints shareable URL + password | `session_create` → `session_created` (URL from `session_id`; password is host-local, never on the wire — see Auth flow) |
| 3 | Browser view gated by password | `auth_pubkey`, `auth`, `auth_result` |
| 4 | Password verified on host, not relay | `auth_request`, `auth_response` — password itself never crosses in a form the relay can read |
| 5 | Rate-limited attempts | `auth_result{code:"RATE_LIMITED"}` |
| 6 | Live output streaming | `output` (host→relay→viewers) |
| 7 | Single active writer, instant take-control, host override | `input`, `take_control`, `control_changed`, host lock window |
| 8 | Chat panel, never touches PTY | `chat_message` |
| 9 | Mobile quick-actions as PTY input | Frontend-side translation to `input`, see [Quick actions](#quick-actions--a-frontend-concept-not-a-wire-message) |
| 10 | Kill switch disconnects viewers, session survives | `kill_switch`, `kicked` |
| 11 | Session teardown on process/CLI exit | `session_ended`, `end_session` |
| 12 | Knowing who's in the session | `presence` |

Reconnect resilience (non-functional requirement) is covered by `resume`
and the relay-issued token.

## Constants (confirmed)

| Constant | Value |
|---|---|
| `RATE_LIMIT_MAX_ATTEMPTS` | 5 |
| `RATE_LIMIT_COOLDOWN_MS` | 60000 |
| `RECONNECT_WINDOW_MS` | 30000 |
| `HOST_LOCK_WINDOW_MS` | 2000 |

## Remaining open item

- Exact ECDH/AEAD primitives (recommend Go `crypto/ecdh` P-256 +
  `crypto/cipher` AES-GCM, and browser `SubtleCrypto` with matching
  parameters) — this is a verification step for Task B5/F3, not a
  decision: confirm both sides interoperate with a small round-trip test
  before building the full auth flow on top of it.
