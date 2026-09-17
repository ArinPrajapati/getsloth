# getsloth v0 — WebSocket Protocol

This is the contract between three parties: the **host CLI** (Go), the
**relay server** (Go), and the **browser viewer** (TypeScript). It is the
seam between the two parallel workstreams in `tasks/plan.md` — Backend
(Phase 1A, Claude) implements the host + relay side, Frontend (Phase 1B,
Pi) implements the viewer side. Neither should need to read the other's
source to build against this document.

Both sides implement exactly what's written here. If an implementation
needs to deviate, this file changes first.

**Revision history:**
- v1 draft: initial message set.
- v2: fixed the relay receiving the plaintext password in transit —
  switched to public-key encryption of the auth attempt.
- v3 (this version): fixed a second, more serious gap in that same fix —
  the relay was *delivering* the public key the viewer encrypted to,
  which let a malicious relay substitute its own key and MITM the whole
  scheme. The public key now travels in the URL fragment instead, which
  never reaches any server. Also pins exact crypto wire parameters
  (curve, encoding, KDF, AEAD, nonce/tag placement, AAD) so two
  independent implementations don't diverge on details.

## Design rules

1. **Every message is a JSON object with a `type` field** (discriminated
   union) and a `v` field (protocol version, `1` for all of v0).
2. **Wire field names are `snake_case`.**
3. **One error shape for everything that isn't a defined success path** —
   see [Errors](#errors).
4. **Unknown `type` handling differs by direction, deliberately:**
   - **Relay → validating client input:** the relay strictly validates
     every inbound (client→relay) message. An unknown `type`, or a known
     `type` with the wrong shape, is `BAD_REQUEST` (see
     [Socket lifecycle](#socket-lifecycle)) — this is the trust boundary,
     and it doesn't get to be lenient.
   - **Client → handling relay output:** the host and viewer SHOULD
     ignore an unknown `type` received *from* the relay rather than
     crashing — this is what allows the protocol to add new
     relay-originated broadcast types later without breaking v0 clients.
   These are not the same rule applied twice; strict at the boundary,
   lenient for forward compatibility, is the intended asymmetry.
5. **TypeScript interfaces below are the schema of record for v0.**
   Required vs. optional (`?`) fields and literal string unions carry the
   same constraints a JSON Schema document would — no separate
   `.schema.json` files maintained in parallel. (Deliberate deviation
   from `tasks/plan.md`'s literal "JSON schema" wording, flagged for the
   founder to override.)

## Connection roles

```
wss://relay.getsloth.dev/ws/host                 → host CLI connects here to create a session
wss://relay.getsloth.dev/ws/viewer/<session_id>  → browser viewer connects here to join one
```

A **host** connection creates and owns exactly one session for its
lifetime. A **viewer** connection joins an existing session and must
authenticate (see [Auth flow](#auth-flow)) before receiving anything
beyond `error` messages.

## Share link format

The host CLI generates the share URL, and this is where the auth public
key lives — **never delivered by the relay, so the relay is never in a
position to substitute its own key**:

```
https://getsloth.dev/s/<session_id>#k=<base64url(raw uncompressed P-256 public key)>
```

The `#k=...` fragment is never sent by the browser to any server — not
the relay, not even the static page host (Vercel) serving `index.html`.
It's read by client-side JavaScript only. This is the same property magic
links and password-reset links rely on to keep a value out of server
logs; here it's what makes "the relay cannot MITM the auth key" actually
true instead of aspirational.

The password (separate from the key) is still printed to the host's
terminal separately, per FR2 — it does not go in the URL at all, in
either the path or the fragment. The key and the password protect two
different things: the key authenticates the channel so the relay can't
read or tamper with the auth exchange; the password is what actually
gates access, so a leaked link alone isn't sufficient to join.

## The relay-blind boundary — how it's actually guaranteed

Per `CONSTRAINTS.md`'s architecture rule and the product decision that the
password is never sent to the relay:

- **The host generates a fresh P-256 key pair when it creates the
  session.** The private key never leaves the host process and is
  discarded when it exits.
- **The public key is never sent to the relay at all** — it goes straight
  from the CLI's stdout into the share URL's fragment, which the founder
  or host shares out-of-band (Slack, SMS, in person). The relay has no
  opportunity to see it, let alone substitute it.
- **The viewer encrypts its password attempt using the key from the
  fragment**, per the exact construction in
  [Crypto wire format](#crypto-wire-format). What crosses the wire to the
  relay is ciphertext the relay structurally cannot decrypt — it never
  held, and cannot obtain, the private key.
- **The relay only ever sees a verdict** (`auth_response.ok`) from the
  host, never password content, and counts only verdicts for rate
  limiting.
- **There is no password or key state on the relay**, and no "rotate
  password" message: the kill switch invalidates relay-issued *tokens*;
  whatever the host compares future attempts against is its own local
  state, unrelated to anything the relay tracks.
- `output`, `chat_message`, and `input` are relayed without inspection of
  content, only routing metadata and size limits.

## Crypto wire format

Pinned precisely because Backend (Go) and Frontend (TS) implement
opposite ends independently — "roughly ECDH + AES-GCM" is not sufficient
for two implementations to interoperate.

| Parameter | Value |
|---|---|
| Curve | P-256 (secp256r1) |
| Public key encoding | Raw uncompressed SEC1 point, 65 bytes (`0x04 \|\| X \|\| Y`). This is exactly what Go's `crypto/ecdh` `PublicKey.Bytes()` returns for NIST curves, and what WebCrypto's `"raw"` import/export format expects — no SPKI/JWK translation needed on either side. |
| JSON field encoding | Standard base64, RFC 4648 §4, padded (`+`, `/`, `=`) |
| URL fragment encoding | base64url, RFC 4648 §5, **unpadded** — required to be URL-safe without percent-encoding. This is the one place encoding differs from the JSON fields, deliberately. |
| Key derivation | HKDF-SHA256 over the raw ECDH shared secret |
| HKDF salt | Empty (zero-length) — the shared secret already has full entropy from fresh ephemeral keys; a salt isn't adding anything here |
| HKDF info | ASCII bytes of the literal string `getsloth-v1-auth` — domain separation, cheap insurance even with only one use today |
| HKDF output length | 32 bytes (the AES-256 key) |
| AEAD | AES-256-GCM |
| Nonce | 12 bytes, randomly generated by the encrypting side (the viewer) per attempt, **prepended** to the ciphertext before base64 encoding — one field, not a separate `nonce_base64` |
| Tag | 16 bytes, **included at the end of the ciphertext** — this is the default behavior of both Go's `cipher.AEAD.Seal` and WebCrypto's `SubtleCrypto.encrypt` for AES-GCM; use `tagLength: 128` explicitly in WebCrypto calls rather than relying on the implicit default |
| AAD | UTF-8 bytes of the string `"getsloth-auth-v1:" + session_id` — binds a ciphertext to the specific session it was encrypted for. Deliberately **not** `request_id`: that's assigned by the relay *after* the viewer has already encrypted, so it cannot be part of the AAD without breaking the flow |
| Plaintext | UTF-8 bytes of the password string, no length prefix (AEAD authenticates the whole blob) |

So: `ciphertext_base64 = base64(nonce(12 bytes) || AES-256-GCM(key, nonce, aad, plaintext=password_utf8))`.

This is standard, well-documented composition on both platforms (Go
`crypto/ecdh` + `golang.org/x/crypto/hkdf` + `crypto/cipher` GCM; browser
`SubtleCrypto` ECDH `deriveBits` → HKDF `deriveKey` → AES-GCM `encrypt`),
not a custom scheme — but pinned parameters don't guarantee two
independent implementations actually interoperate on the first try. **Do
a small round-trip test (Go encrypts, TS decrypts, and vice versa) before
building the rest of the auth flow on top of it** — this is the first
thing Tasks B5 and F3 should verify, not the last.

## Auth flow

```
Viewer (already has host's                Relay                          Host CLI
 public key from URL fragment)
  │                                          │                                │
  │── auth (ciphertext, viewer pubkey) ─────►│                                │
  │                                          │── auth_request (req_id, ─────►│  (host derives shared
  │                                          │    ciphertext, viewer pubkey) │   secret, decrypts,
  │                                          │◄── auth_response (req_id, ok)─│   compares locally)
  │◄─── auth_result (token, connection_id) ──│ (relay generates token itself)│
```

1. Viewer already has the host's public key from the share URL's
   fragment — no exchange with the relay is needed to obtain it.
2. Viewer encrypts its password attempt per
   [Crypto wire format](#crypto-wire-format) and connects to
   `/ws/viewer/<session_id>`, sending `auth`. (`error{SESSION_NOT_FOUND}`
   and close, if the session doesn't exist.)
3. Relay checks the viewer isn't rate-limited for this
   `(session_id, remote_address)` pair (see
   [Rate limiting](#rate-limiting)). If limited, relay responds directly
   with `auth_result{ok:false, code:"RATE_LIMITED", retry_after_ms}` and
   does **not** contact the host.
4. Otherwise relay forwards `auth_request` to the host — ciphertext and
   the viewer's one-time public key, opaque to the relay — tagged with a
   `request_id` to correlate concurrent viewers.
5. Host derives the shared secret, decrypts, compares locally, replies
   `auth_response{request_id, ok}`.
6. **On `ok:true`, the relay generates a random session token and a
   `connection_id` for this viewer**, records both against the
   connection, and sends them in `auth_result`. The host never generates
   or sees this token.
7. On `ok:false`, relay increments the `(session_id, remote_address)`
   failure counter. At `RATE_LIMIT_MAX_ATTEMPTS` (5) further attempts get
   `RATE_LIMITED` without contacting the host until
   `RATE_LIMIT_COOLDOWN_MS` (60000) expires. The connection is **not**
   closed on rate limit.

## Rate limiting

Keyed on `(session_id, remote_address)`, not the WebSocket connection
object, so reconnecting doesn't reset the counter. Not a complete
mitigation (address can be spoofed or shared behind NAT) but appropriate
for the v0 "trusted internal teams" threat model.

## Control model

Exactly one connection is the "active writer" at any time — initially the
host, including the host's own local keystrokes, which are gated by this
exact same rule (see the note on `output` scoping below for why the host
still needs `control_changed`).

Any authenticated connection may send `take_control`; the relay
reassigns the active writer and broadcasts `control_changed`.

**Host `take_control` is authoritative and starts a short lock window.**
When the host sends `take_control`, the relay reassigns immediately *and*
rejects any non-host `take_control` received within `HOST_LOCK_WINDOW_MS`
(2000ms) afterward, responding `error{code:"NOT_ACTIVE_WRITER"}` to the
rejected sender instead of reassigning. A viewer's own `take_control` is
not similarly authoritative — only the host gets the lock window.

`input` messages are only applied to the PTY if the sender is the current
active writer; from anyone else they're silently dropped by the relay
(not forwarded to the host).

## Quick actions — a Frontend concept, not a wire message

The mobile yes/no/continue/text UI (Task F6) translates directly to an
`input` message client-side, using this fixed mapping:

| Action | `input.data` (before base64) |
|---|---|
| yes | `y\r` |
| no | `n\r` |
| continue | `\r` (plain Enter) |
| text | `{typed text}\r` |

One PTY-input path on the wire, gated by active-writer status in exactly
one place.

## Reconnect

A viewer that briefly loses network sends `resume{token}` instead of
`auth{...}`. If the relay-issued token is still valid (session alive, not
invalidated by a kill switch, within `RECONNECT_WINDOW_MS` = 30000) the
relay responds `auth_result{ok:true, connection_id}` immediately — no
host round-trip, since the relay itself is the token's issuer and
authority.

## Limits

Hard caps, enforced by the relay; violation closes the connection with
`error{code:"BAD_REQUEST"}` and close code `4002`:

| Field | Limit |
|---|---|
| Any single WebSocket frame | 256 KiB |
| `chat_message.text` | 2000 bytes (UTF-8) |
| `input.data` (decoded) | 4096 bytes |
| `output.data` (decoded, per message) | 65536 bytes — host/relay chunk larger output across multiple messages |
| `auth.ciphertext_base64` (decoded) | 1024 bytes |
| `display_name` | 64 bytes |

## Socket lifecycle

Custom WebSocket close codes (application range, RFC 6455):

| Code | Meaning | Preceded by |
|---|---|---|
| 4000 | `session_ended` | `SessionEndedMsg` broadcast first |
| 4001 | `kicked` | `KickedMsg` sent to the affected viewer first |
| 4002 | `bad_request` | `ErrorMsg{code:"BAD_REQUEST"}` first when possible |
| 4003 | `session_not_found` | `ErrorMsg{code:"SESSION_NOT_FOUND"}` first |
| 4004 | `unsupported_version` | `ErrorMsg{code:"UNSUPPORTED_VERSION"}` first |

`UNAUTHORIZED`, `RATE_LIMITED`, and `NOT_ACTIVE_WRITER` never close the
connection — all three are routine, expected outcomes, not protocol
violations.

## Protocol version mismatch

If a received message's `v` is not `1`, the receiver responds
`error{code:"UNSUPPORTED_VERSION"}` and closes with code `4004`. No
negotiation in v0.

## Message reference

```typescript
interface Envelope {
  v: 1;
  type: string;
}
```

### Viewer → Relay

```typescript
interface AuthMsg extends Envelope {
  type: "auth";
  viewer_pubkey_base64: string;   // viewer's fresh one-time ECDH public key, raw uncompressed point
  ciphertext_base64: string;      // see Crypto wire format
  display_name?: string;          // shown in presence; relay assigns "Viewer N" if omitted
}

interface ResumeMsg extends Envelope {
  type: "resume";
  token: string; // relay-issued, from a prior auth_result
}

interface InputMsg extends Envelope {
  type: "input";
  data_base64: string; // PTY data isn't guaranteed valid UTF-8
}

interface TakeControlMsg extends Envelope {
  type: "take_control";
}

// Rendered client-side via textContent, never innerHTML — untrusted input
interface ChatMsg extends Envelope {
  type: "chat_message";
  text: string;
}
```

### Host → Relay

```typescript
// No public key here — it never touches the relay, see Share link format
interface SessionCreateMsg extends Envelope {
  type: "session_create";
}

interface OutputMsg extends Envelope {
  type: "output";
  data_base64: string;
}

interface AuthResponseMsg extends Envelope {
  type: "auth_response";
  request_id: string;
  ok: boolean;
  // no token — the relay issues it, see Auth flow step 6
}

interface KillSwitchMsg extends Envelope {
  type: "kill_switch";
}

interface EndSessionMsg extends Envelope {
  type: "end_session";
}
```

Host also sends `take_control` and `chat_message` using the shapes above
— the host is just another connection with the `host` role attached
server-side, which is what grants its `take_control` lock-window
authority. Host has no `display_name` — it always displays as `"Host"`,
a fixed label, not a field.

### Relay → Host

```typescript
interface SessionCreatedMsg extends Envelope {
  type: "session_created";
  session_id: string;
  connection_id: string; // so the host can recognize itself in `presence`
}

interface AuthRequestMsg extends Envelope {
  type: "auth_request";
  request_id: string;
  viewer_pubkey_base64: string;
  ciphertext_base64: string;
}
```

### Relay → Viewer (before/during auth)

```typescript
interface AuthResultMsg extends Envelope {
  type: "auth_result";
  ok: boolean;
  token?: string;                          // present when ok === true
  connection_id?: string;                  // present when ok === true — so the viewer can recognize itself in `presence`
  code?: "AUTH_FAILED" | "RATE_LIMITED";
  retry_after_ms?: number;
}

interface KickedMsg extends Envelope {
  type: "kicked";
  reason: "kill_switch";
}
```

### Relay → viewers only

```typescript
// NOT sent back to the host — the host already has this output locally
// from its own PTY; echoing it back would double-render or loop.
interface OutputBroadcastMsg extends Envelope {
  type: "output";
  data_base64: string;
}
```

### Relay → all authenticated connections (host + every authenticated viewer)

```typescript
interface ControlChangedMsg extends Envelope {
  type: "control_changed";
  active_writer_id: string;
  active_writer_role: "host" | "viewer";
}

// The host receives this too — it needs to know when to stop forwarding
// its own local keystrokes, exactly like a viewer would.
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
    display_name?: string; // absent for host; Frontend/host label their own entry "You" locally by comparing connection_id
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
  message: string; // human-readable, not for programmatic branching.
                    // Rendered via textContent, never innerHTML.
}
```

## Functional requirement → message map

| FR | Requirement | Message(s) |
|---|---|---|
| 1 | Host wraps any command in a PTY | N/A — CLI-local |
| 2 | Prints shareable URL + password | `session_create` → `session_created` (URL built from `session_id` + the host's public key in the fragment, per [Share link format](#share-link-format); password is host-local, never on the wire) |
| 3 | Browser view gated by password | `auth`, `auth_result` |
| 4 | Password verified on host, not relay | `auth_request`, `auth_response` — encrypted per [Crypto wire format](#crypto-wire-format), key never touches the relay |
| 5 | Rate-limited attempts | `auth_result{code:"RATE_LIMITED"}` |
| 6 | Live output streaming | `output` (host→relay→viewers only) |
| 7 | Single active writer, instant take-control, host override | `input`, `take_control`, `control_changed`, host lock window |
| 8 | Chat panel, never touches PTY | `chat_message` |
| 9 | Mobile quick-actions as PTY input | Frontend-side translation to `input`, see [Quick actions](#quick-actions--a-frontend-concept-not-a-wire-message) |
| 10 | Kill switch disconnects viewers, session survives | `kill_switch`, `kicked` |
| 11 | Session teardown on process/CLI exit | `session_ended`, `end_session` |
| 12 | Knowing who's in the session | `presence` |

Reconnect resilience is covered by `resume` and the relay-issued token.

## Constants (confirmed)

| Constant | Value |
|---|---|
| `RATE_LIMIT_MAX_ATTEMPTS` | 5 |
| `RATE_LIMIT_COOLDOWN_MS` | 60000 |
| `RECONNECT_WINDOW_MS` | 30000 |
| `HOST_LOCK_WINDOW_MS` | 2000 |

See [Crypto wire format](#crypto-wire-format) for the pinned cryptographic
parameters — not left as an open item, but still worth an early
round-trip interop test before Tasks B5/F3 build the rest of the auth
flow on top of it.
