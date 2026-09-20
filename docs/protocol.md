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
- v3: fixed a second, more serious gap in that same fix — the relay was
  *delivering* the public key the viewer encrypted to, which let a
  malicious relay substitute its own key and MITM the whole scheme. The
  public key now travels in the URL fragment instead, which never
  reaches any server. Also pins exact crypto wire parameters (curve,
  encoding, KDF, AEAD, nonce/tag placement, AAD) so two independent
  implementations don't diverge on details.
- v4: added the missing relay→host input-forwarding message (the
  protocol previously never specified how a viewer's approved keystrokes
  actually reached the PTY), clarified that the host's own input bypasses
  the network entirely, and fixed a misattached comment and a
  limits-table field-name mismatch.
- v5: approved for implementation. Added reconnect +
  active-writer interaction semantics and token entropy/encoding —
  the two remaining non-blocking clarifications from the final review
  pass.
- v6: added active-writer-gated terminal `resize` messages so browser
  xterm rows/columns are applied to the host PTY. This is required for
  full-screen TUIs (`nvim`, `htop`, `tmux`, agent TUIs) to render across
  the full browser terminal instead of a stale default PTY grid.
- v7 (this version): added explicit `remote` and `group` session modes,
  canonical terminal geometry, atomic viewer takeover with dimensions,
  host-size restoration, and occupied/read-only errors. This prevents
  multiple differently sized browsers from fighting over one PTY grid.

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

The host configures one mode immediately after `session_created`:

- **Remote** (default): one authenticated remote viewer identity; that viewer
  may take control. Control and PTY geometry ownership transfer together.
- **Group**: multiple authenticated viewers; the host remains the only writer
  and geometry owner. Viewers can watch and chat but cannot send PTY input,
  resize, or take control.

Device type is never inferred. A phone, laptop, console, or TV browser follows
the permissions of the host-selected mode.

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
terminal separately, per FR2 — it does not go in this URL at all, in
either the path or the fragment. The key and the password protect two
different things: the key authenticates the channel so the relay can't
read or tamper with the auth exchange; the password is what actually
gates access, so a leaked (forwarded, screenshotted-without-the-terminal)
copy of this link alone isn't sufficient to join.

### QR-only link variant

The terminal QR code (see `cmd/getsloth/qrcode.go`) encodes a *different*
URL, built by `qrShareURL`, that adds the password to the fragment too:

```
https://getsloth.dev/s/<session_id>#k=<pubkey>&p=<password>
```

This is deliberately not the link above. Someone who can see the QR is,
by construction, looking at the host's own terminal — which already
prints the password in plain text right next to it — so embedding it
here reveals nothing new. It only exists to skip retyping a 10-character
password on a phone right after scanning; it is never what gets pasted
into Slack or handed to a teammate (that's still the password-free link).
The web viewer reads `p` from the fragment and auto-submits it through
the same auth path a manual password entry uses (see
[The relay-blind boundary](#the-relay-blind-boundary--how-its-actually-guaranteed)
below — nothing about that guarantee changes based on where the password
value came from).

`<pubkey>` here is also encoded differently than in the plain link above:
the *compressed* SEC1 point (33 bytes: a `0x02`/`0x03` parity prefix +
the X coordinate) rather than the raw uncompressed point (65 bytes: `0x04
|| X || Y`) the [Public key encoding](#crypto-wire-format) row otherwise
pins. This is purely a QR-size optimization — the terminal QR code is the
one place fewer bytes measurably shrinks the rendered code (adding the
password already pushes the module count up; halving the key's
contribution brings it back down) — and doesn't change what's
cryptographically true: it's still the same P-256 point, on the same
curve, used for the same ECDH per [Crypto wire format](#crypto-wire-format).

Because WebCrypto's `importKey('raw', ...)` only accepts the uncompressed
form, the web viewer decompresses this key back to 65 bytes before
import (`web/src/ec-point.ts`, using `@noble/curves` — not hand-rolled
curve math) whenever the fragment key it received is 33 bytes long; the
plain link's 65-byte key is used as-is. The Go side produces the
compressed form via `crypto/elliptic.MarshalCompressed` (also not
hand-rolled). Both are verified to interoperate on real, independently
generated key material, not just unit-tested in isolation — see
`internal/hostauth/hostauth_test.go`'s
`TestPublicKeyCompressedBase64URL_RoundTripsToSamePoint` and
`web/src/auth.test.ts`'s compressed-key end-to-end case.

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
   `auth_response{request_id, ok}`. **If decryption itself fails**
   (malformed ciphertext, wrong/stale key from a corrupted link, etc.),
   the host replies `ok:false` — identically to a wrong-password result,
   not a distinct error. This is deliberate: a distinguishable "your
   ciphertext was malformed" response would be a small oracle leaking
   information a failed attempt shouldn't; failing closed the same way
   for both cases avoids that.
6. **On `ok:true`, the relay generates a random session token and a
   `connection_id` for this viewer**, records both against the
   connection, and sends them in `auth_result`. The host never generates
   or sees this token. **Token format:** 32 bytes from a CSPRNG,
   standard base64-encoded (same convention as the rest of the wire
   fields — it only ever appears in JSON messages, never a URL, so
   there's no reason to switch encodings the way the share-link key
   does).
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

In remote mode, the authenticated viewer may send `take_control` with its
desired `cols` and `rows`; the relay changes writer and canonical geometry as
one transition, then broadcasts `control_changed`. In group mode, viewer
`take_control`, `input`, and `resize` are rejected with `READ_ONLY_SESSION`.

**Host `take_control` is authoritative and starts a short lock window.**
When the host sends `take_control`, the relay reassigns immediately *and*
rejects any non-host `take_control` received within `HOST_LOCK_WINDOW_MS`
(2000ms) afterward, responding `error{code:"NOT_ACTIVE_WRITER"}` to the
rejected sender instead of reassigning. A viewer's own `take_control` is
not similarly authoritative — only the host gets the lock window.

`input` messages are only applied to the PTY if the sender is the current
active writer; from a viewer that isn't, they're silently dropped by the
relay (not forwarded to the host). When a viewer *is* the active writer,
the relay forwards their input to the host as `InputForwardMsg` (see
Message reference) and the host writes the decoded bytes to the PTY.

`resize` messages follow the same active-writer rule in remote mode. The active viewer's
browser terminal rows/columns are forwarded to the host so the host can
resize the real PTY. A viewer that is not the active writer must not be
able to resize the PTY out from under whoever is currently driving.

There is one canonical PTY grid for the session. Spectators resize their
logical xterm grid to those canonical dimensions and locally fit or pan it;
their browser viewport never becomes a second PTY size. The relay remembers
the host's latest local size and restores it immediately when the host reclaims
control or the active remote viewer disconnects.

**Host's own input never touches the network.** The host CLI owns the
PTY directly, so when the host is the active writer, its local keystrokes
are written straight to the PTY, with no WebSocket round-trip in either
direction — there is no `InputMsg` the host sends to itself and no
`InputForwardMsg` the host ever receives for its own typing. When the
host is *not* the active writer (a viewer holds it), the host CLI must
still read its own stdin but drop those keystrokes locally instead of
writing them to the PTY, using the `control_changed` state it already
has — this check is entirely local, no network call needed to make it.

The host CLI reserves `Ctrl-]` as a local command prefix that is processed
before this input gate. `Ctrl-] r` sends the host's authoritative
`take_control`, so reclaim remains available while ordinary host keystrokes are
being dropped. `Ctrl-] i` prints the current mode, viewers, connection state,
and controller. The terminal/tab title carries the same compact status without
consuming a PTY row; this avoids damaging full-screen TUIs. `SIGUSR2` remains a
scriptable reclaim alternative.

## Browser terminal input

The viewer uses xterm's normal keyboard/input path; there are no dedicated
yes/no/continue controls in the session UI. Every keystroke remains an `input`
message and is accepted only from the confirmed active writer. On touch
devices, the terminal is focused—and the software keyboard opened—only after a
remote-mode `control_changed` confirms the viewer owns control.

## Reconnect

A viewer that briefly loses network sends `resume{token}` instead of
`auth{...}`. If the relay-issued token is still valid (session alive, not
invalidated by a kill switch, within `RECONNECT_WINDOW_MS` = 30000) the
relay responds with the complete successful `auth_result` session state
immediately — no
host round-trip, since the relay itself is the token's issuer and
authority.

**Active-writer status does not survive a disconnect, regardless of the
reconnect window.** If the current active writer's connection drops for
any reason, the relay immediately reassigns the active writer to the
**host** and broadcasts `control_changed` right away — it does not wait
out `RECONNECT_WINDOW_MS` on the chance they come back. This keeps the
system consistent with the host always being the fallback authority
(the same principle behind the host lock window in
[Control model](#control-model)) and avoids a confusing state where
nobody can type while a disconnected client is still nominally "in
control." If the disconnected viewer reconnects via `resume` within the
window, they rejoin as an authenticated participant like anyone else —
they do **not** automatically regain control and must send `take_control`
again if they want it back.

## Limits

Hard caps, enforced by the relay; violation closes the connection with
`error{code:"BAD_REQUEST"}` and close code `4002`:

| Field | Limit |
|---|---|
| Any single WebSocket frame | 256 KiB |
| `chat_message.text` | 2000 bytes (UTF-8) |
| `input.data_base64`, decoded | 4096 bytes |
| `output.data_base64`, decoded, per message | 65536 bytes — host/relay chunk larger output across multiple messages |
| `auth.ciphertext_base64`, decoded | 1024 bytes |
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

`UNAUTHORIZED`, `RATE_LIMITED`, `SESSION_OCCUPIED`, `READ_ONLY_SESSION`,
and `NOT_ACTIVE_WRITER` never close the connection; they are expected outcomes,
not protocol violations.

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

interface ResizeMsg extends Envelope {
  type: "resize";
  cols: number; // terminal columns, fitted from the browser xterm surface
  rows: number; // terminal rows, fitted from the browser xterm surface
}

interface TakeControlMsg extends Envelope {
  type: "take_control";
  cols?: number; // required from a viewer; host reclaim uses cached host size
  rows?: number;
}

// Rendered client-side via textContent, never innerHTML — untrusted input
interface ChatMsg extends Envelope {
  type: "chat_message";
  text: string;
}
```

### Host → Relay

```typescript
// Sent immediately after session_created and before output starts.
interface SessionConfigMsg extends Envelope {
  type: "session_config";
  mode: "remote" | "group";
  host_cols: number;
  host_rows: number;
}

interface HostSizeMsg extends Envelope {
  type: "host_size";
  cols: number;
  rows: number;
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

// The missing piece in earlier drafts: the relay doesn't own the PTY, the
// host does. Once a viewer's InputMsg passes the active-writer check, the
// relay must forward it to the host so the host can write the decoded
// bytes to the PTY. Sent ONLY when a viewer is the active writer — see
// "Host's own input never touches the network" under Control model for
// why the host never receives this for its own keystrokes.
interface InputForwardMsg extends Envelope {
  type: "input";
  data_base64: string;
  sender_id: string; // the viewer connection_id that sent it
}

// Forwarded only for the active viewer. The host applies this to the
// real PTY rows/cols; without it, full-screen TUIs render into a stale
// default-sized grid even if the browser CSS box is fullscreen.
interface ResizeMsg extends Envelope {
  type: "resize";
  cols: number;
  rows: number;
}
```

### Relay → Viewer (before/during auth)

```typescript
interface AuthResultMsg extends Envelope {
  type: "auth_result";
  ok: boolean;
  token?: string;                          // present when ok === true
  connection_id?: string;                  // present when ok === true — so the viewer can recognize itself in `presence`
  code?: "AUTH_FAILED" | "RATE_LIMITED" | "SESSION_OCCUPIED";
  retry_after_ms?: number;
  mode?: "remote" | "group";             // present when ok === true
  cols?: number;                           // canonical grid when ok === true
  rows?: number;
  active_writer_id?: string;
  active_writer_role?: "host" | "viewer";
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

interface TerminalSizeMsg extends Envelope {
  type: "terminal_size";
  cols: number;
  rows: number;
}
```

### Relay → all authenticated connections (host + every authenticated viewer)

```typescript
// The host receives this too — it needs to know when to stop writing its
// own local keystrokes to the PTY, exactly like a viewer's input would be
// gated. See "Host's own input never touches the network" under Control
// model for how the host applies this locally.
interface ControlChangedMsg extends Envelope {
  type: "control_changed";
  active_writer_id: string;
  active_writer_role: "host" | "viewer";
  cols: number;
  rows: number;
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
    | "READ_ONLY_SESSION"
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
| 2 | Prints shareable URL + password | `session_created` → `session_config` (URL built from `session_id` + the host's public key in the fragment, per [Share link format](#share-link-format); password is host-local, never on the wire) |
| 3 | Browser view gated by password | `auth`, `auth_result` |
| 4 | Password verified on host, not relay | `auth_request`, `auth_response` — encrypted per [Crypto wire format](#crypto-wire-format), key never touches the relay |
| 5 | Rate-limited attempts | `auth_result{code:"RATE_LIMITED"}` |
| 6 | Live output streaming | `output` (host→relay→viewers only) |
| 7 | Single active writer, instant take-control, host override | `input` (viewer→relay), `InputForwardMsg` (relay→host), `resize` (viewer→relay→host), `take_control`, `control_changed`, host lock window |
| 8 | Chat panel, never touches PTY | `chat_message` |
| 9 | Mobile terminal typing | `take_control{cols,rows}` confirmation, then normal xterm `input` |
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
