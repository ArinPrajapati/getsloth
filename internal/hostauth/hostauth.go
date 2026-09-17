// Package hostauth implements password verification for the host CLI.
//
// This is the one place password-comparison logic is allowed to live —
// see CONSTRAINTS.md. internal/relay must never import this package.
package hostauth

// ComparePassword reports whether attempt matches the password the host
// was started with. Placeholder until Task B5 implements real
// decryption and comparison per docs/protocol.md's crypto wire format.
func ComparePassword(attempt, actual string) bool {
	return attempt == actual
}
