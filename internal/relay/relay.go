// Package relay implements the getsloth WebSocket relay server.
//
// It must never import internal/hostauth or otherwise contain
// password-comparison logic — see CONSTRAINTS.md's architecture rule and
// docs/protocol.md's "relay-blind boundary" section. This is enforced by
// the depguard rule in .golangci.yml, not just this comment.
package relay
