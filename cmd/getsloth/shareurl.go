package main

import (
	"fmt"
	"net/url"
)

// shareURL builds the link the host shares with viewers, per
// docs/protocol.md's Share link format: the auth public key travels in
// the URL fragment, which browsers never send to any server - not the
// relay, not the page host serving the viewer itself.
func shareURL(webBaseURL, sessionID, hostPubkeyBase64URL string) string {
	return fmt.Sprintf("%s/s/%s#k=%s", webBaseURL, sessionID, hostPubkeyBase64URL)
}

// qrShareURL builds the link encoded into the terminal QR code
// specifically - unlike shareURL, it also carries the session password in
// the fragment, so scanning it on a phone can skip the manual password
// prompt entirely. This is deliberately not what gets printed as the
// plain copyable link: someone scanning the QR off the host's own
// terminal can already read the password printed right next to it, so
// embedding it here reveals nothing they didn't already have - but a
// pasted/forwarded link is a different trust boundary (it can reach
// someone who never saw the terminal), which is why the two link forms
// stay separate. See docs/ideas/getsloth.md's "QR bypasses the password
// prompt" decision.
func qrShareURL(webBaseURL, sessionID, hostPubkeyBase64URL, password string) string {
	return fmt.Sprintf("%s/s/%s#k=%s&p=%s", webBaseURL, sessionID, hostPubkeyBase64URL, url.QueryEscape(password))
}
