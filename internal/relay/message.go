package relay

// Wire message shapes per docs/protocol.md. Only the types Task B3
// (connection lifecycle) needs are defined here - later tasks add the
// rest (output, auth, control, chat, ...) as they're implemented, rather
// than pre-declaring an unused message set.

// ProtocolVersion is the only value the `v` envelope field accepts in
// v0. See docs/protocol.md's "Protocol version mismatch" section.
const ProtocolVersion = 1

// Envelope is the header every protocol message shares.
type Envelope struct {
	V    int    `json:"v"`
	Type string `json:"type"`
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

// Reasons a session can end, per docs/protocol.md. Task B3 only ever
// produces ReasonHostDisconnected (an abrupt host socket close); the
// other two are for B7/B8 (explicit end_session, wrapped process exit).
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

// Custom WebSocket close codes, application range per RFC 6455. See
// docs/protocol.md's "Socket lifecycle" section.
const (
	CloseSessionEnded       = 4000
	CloseKicked             = 4001
	CloseBadRequest         = 4002
	CloseSessionNotFound    = 4003
	CloseUnsupportedVersion = 4004
)

func newEnvelope(msgType string) Envelope {
	return Envelope{V: ProtocolVersion, Type: msgType}
}
