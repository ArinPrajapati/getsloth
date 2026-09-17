// Package protocol defines the wire message shapes shared between the
// host CLI and the relay server, per docs/protocol.md. It contains no
// logic beyond simple constructors - no password comparison, no
// session state - so both internal/relay and cmd/getsloth can safely
// depend on it without tripping the depguard rule in .golangci.yml.
//
// Types are added incrementally, task by task, matching what's actually
// implemented - not the full protocol.md set declared up front.
package protocol

// ProtocolVersion is the only value the `v` envelope field accepts in
// v0. See docs/protocol.md's "Protocol version mismatch" section.
const ProtocolVersion = 1

// Envelope is the header every protocol message shares.
type Envelope struct {
	V    int    `json:"v"`
	Type string `json:"type"`
}

func NewEnvelope(msgType string) Envelope {
	return Envelope{V: ProtocolVersion, Type: msgType}
}

// SessionCreatedMsg is sent relay -> host once, right after a host
// connection is accepted.
type SessionCreatedMsg struct {
	Envelope
	SessionID    string `json:"session_id"`
	ConnectionID string `json:"connection_id"`
}

// SessionEndedMsg is broadcast relay -> viewers when the session tears
// down, before the connection is closed with CloseSessionEnded.
type SessionEndedMsg struct {
	Envelope
	Reason string `json:"reason"`
}

// Reasons a session can end, per docs/protocol.md.
const (
	ReasonHostDisconnected = "host_disconnected"
	ReasonHostEnded        = "host_ended"
	ReasonProcessExited    = "process_exited"
)

// ErrorMsg is the single error shape for anything that isn't a defined
// success path. See docs/protocol.md's "Errors" section.
type ErrorMsg struct {
	Envelope
	Code    string `json:"code"`
	Message string `json:"message"`
}

const (
	ErrSessionNotFound    = "SESSION_NOT_FOUND"
	ErrUnauthorized       = "UNAUTHORIZED"
	ErrNotActiveWriter    = "NOT_ACTIVE_WRITER"
	ErrUnsupportedVersion = "UNSUPPORTED_VERSION"
	ErrBadRequest         = "BAD_REQUEST"
)

// OutputMsg carries a chunk of terminal output. Used both host -> relay
// (the host's own PTY output) and relay -> viewers (the broadcast) - the
// shape is identical in both directions per docs/protocol.md, so one Go
// type serves both rather than two structurally-identical ones that
// could drift apart.
//
// DataBase64 is capped at MaxOutputChunkBytes decoded bytes per message
// (docs/protocol.md's Limits table) - callers writing output must chunk
// larger writes themselves rather than relying on the receiver to split
// them.
type OutputMsg struct {
	Envelope
	DataBase64 string `json:"data_base64"`
}

// MaxOutputChunkBytes is the decoded-size limit for a single
// OutputMsg.DataBase64, per docs/protocol.md's Limits table.
const MaxOutputChunkBytes = 65536

// AuthMsg is a viewer's password attempt, encrypted per
// docs/protocol.md's Crypto wire format - the plaintext password never
// crosses the wire, and neither does anything the relay could decrypt.
type AuthMsg struct {
	Envelope
	ViewerPubkeyBase64 string `json:"viewer_pubkey_base64"`
	CiphertextBase64   string `json:"ciphertext_base64"`
	DisplayName        string `json:"display_name,omitempty"`
}

// ResumeMsg lets a viewer reconnect with a previously issued token
// instead of re-authenticating, per docs/protocol.md's Reconnect
// section.
type ResumeMsg struct {
	Envelope
	Token string `json:"token"`
}

// AuthRequestMsg is the relay forwarding a viewer's auth attempt to the
// host, opaquely - the relay never inspects CiphertextBase64's content,
// only routes it.
type AuthRequestMsg struct {
	Envelope
	RequestID          string `json:"request_id"`
	ViewerPubkeyBase64 string `json:"viewer_pubkey_base64"`
	CiphertextBase64   string `json:"ciphertext_base64"`
}

// AuthResponseMsg is the host's verdict on one AuthRequestMsg. There is
// deliberately no token field here - the relay issues the token itself,
// per docs/protocol.md's Auth flow step 6 (the host generating it would
// leave the relay with no way to validate it later on resume).
type AuthResponseMsg struct {
	Envelope
	RequestID string `json:"request_id"`
	OK        bool   `json:"ok"`
}

// AuthResultMsg is the relay's reply to a viewer's auth or resume
// attempt.
type AuthResultMsg struct {
	Envelope
	OK           bool   `json:"ok"`
	Token        string `json:"token,omitempty"`
	ConnectionID string `json:"connection_id,omitempty"`
	Code         string `json:"code,omitempty"`
	RetryAfterMs int64  `json:"retry_after_ms,omitempty"`
}

const (
	AuthCodeFailed      = "AUTH_FAILED"
	AuthCodeRateLimited = "RATE_LIMITED"
)

// InputMsg is a viewer's keystrokes (or a translated mobile quick
// action), sent viewer -> relay. Only applied to the PTY if the sender
// is the current active writer - otherwise silently dropped by the
// relay, never forwarded to the host.
type InputMsg struct {
	Envelope
	DataBase64 string `json:"data_base64"`
}

// InputForwardMsg is the relay forwarding an approved viewer input to
// the host, once the relay has confirmed the sender is the active
// writer - the host trusts this without re-checking, since the relay is
// the sole authority on active-writer state. Never sent for the host's
// own keystrokes, which reach the PTY directly without touching the
// network - see docs/protocol.md's "Host's own input never touches the
// network".
type InputForwardMsg struct {
	Envelope
	DataBase64 string `json:"data_base64"`
	SenderID   string `json:"sender_id"`
}

// TakeControlMsg requests the sender become the active writer - sent by
// either the host or a viewer, host or viewer alike. Relay reassigns
// immediately; a host-originated one is additionally authoritative for
// HostLockWindow, per docs/protocol.md's Control model.
type TakeControlMsg struct {
	Envelope
}

// ControlChangedMsg is broadcast to every connection (host and every
// authenticated viewer) whenever the active writer changes.
type ControlChangedMsg struct {
	Envelope
	ActiveWriterID   string `json:"active_writer_id"`
	ActiveWriterRole string `json:"active_writer_role"`
}

// KillSwitchMsg is host -> relay: disconnect every current viewer
// without ending the session itself. The wrapped process and the
// session keep running; the host can set a new password and reshare
// afterward.
type KillSwitchMsg struct {
	Envelope
}

// KickedMsg is relay -> viewer, sent (with CloseKicked following) when
// the host triggers the kill switch.
type KickedMsg struct {
	Envelope
	Reason string `json:"reason"`
}

const ReasonKillSwitch = "kill_switch"

// EndSessionMsg is host -> relay: an explicit, clean end to the
// session - sent right before the host process exits, as opposed to the
// relay inferring host_disconnected from an abrupt connection drop.
type EndSessionMsg struct {
	Envelope
}

// Custom WebSocket close codes, application range per RFC 6455. See
// docs/protocol.md's "Socket lifecycle" section.
const (
	CloseSessionEnded       = 4000
	CloseKicked             = 4001
	CloseBadRequest         = 4002
	CloseSessionNotFound    = 4003
	CloseUnsupportedVersion = 4004
)
