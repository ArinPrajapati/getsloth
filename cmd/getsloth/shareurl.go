package main

import "fmt"

// shareURL builds the link the host shares with viewers, per
// docs/protocol.md's Share link format: the auth public key travels in
// the URL fragment, which browsers never send to any server - not the
// relay, not the page host serving the viewer itself.
func shareURL(webBaseURL, sessionID, hostPubkeyBase64URL string) string {
	return fmt.Sprintf("%s/s/%s#k=%s", webBaseURL, sessionID, hostPubkeyBase64URL)
}
