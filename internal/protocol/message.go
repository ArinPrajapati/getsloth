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

// Custom WebSocket close codes, application range per RFC 6455. See
// docs/protocol.md's "Socket lifecycle" section.
const (
	CloseSessionEnded       = 4000
	CloseKicked             = 4001
	CloseBadRequest         = 4002
	CloseSessionNotFound    = 4003
	CloseUnsupportedVersion = 4004
)
