package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/arinprajapati/getsloth/internal/hostauth"
	"github.com/arinprajapati/getsloth/internal/protocol"
	"github.com/arinprajapati/getsloth/internal/relay"
	"github.com/gorilla/websocket"
	"golang.org/x/crypto/hkdf"
)

// encryptAsViewer implements the viewer side of docs/protocol.md's
// Crypto wire format independently of anything in this codebase's own
// packages - deliberately duplicated from
// internal/hostauth/hostauth_test.go rather than shared, because the
// point is to exercise interop through the real production code path
// (connectHost, runHostMessageLoop) end to end, the same way a
// genuinely separate client implementation (e.g. Pi's TypeScript
// frontend) would have to.
func encryptAsViewer(t *testing.T, hostPubkeyBase64URL, sessionID, password string) (viewerPubBase64, ciphertextBase64 string) {
	t.Helper()

	hostPubBytes, err := base64.RawURLEncoding.DecodeString(hostPubkeyBase64URL)
	if err != nil {
		t.Fatalf("decoding host pubkey fragment encoding: %v", err)
	}
	hostPub, err := ecdh.P256().NewPublicKey(hostPubBytes)
	if err != nil {
		t.Fatalf("parsing host pubkey: %v", err)
	}

	viewerPriv, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generating viewer key: %v", err)
	}

	shared, err := viewerPriv.ECDH(hostPub)
	if err != nil {
		t.Fatalf("ECDH: %v", err)
	}

	aesKey := make([]byte, 32)
	kdf := hkdf.New(sha256.New, shared, nil, []byte("getsloth-v1-auth"))
	if _, err := io.ReadFull(kdf, aesKey); err != nil {
		t.Fatalf("HKDF: %v", err)
	}

	block, err := aes.NewCipher(aesKey)
	if err != nil {
		t.Fatalf("aes.NewCipher: %v", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatalf("cipher.NewGCM: %v", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		t.Fatalf("generating nonce: %v", err)
	}

	aad := []byte("getsloth-auth-v1:" + sessionID)
	sealed := gcm.Seal(nil, nonce, []byte(password), aad)
	blob := append(nonce, sealed...)

	return base64.StdEncoding.EncodeToString(viewerPriv.PublicKey().Bytes()),
		base64.StdEncoding.EncodeToString(blob)
}

// dummyControlState provides a runHostMessageLoop with the plumbing
// B6's control tests need but auth-focused tests don't exercise: an
// always-active writer flag and a ptmx channel pre-filled with nil,
// safe as long as the test never sends an "input" message (which would
// try to write through it).
func dummyControlState() (*atomic.Bool, chan *os.File) {
	active := &atomic.Bool{}
	active.Store(true)
	ptmxCh := make(chan *os.File, 1)
	ptmxCh <- nil
	return active, ptmxCh
}

func TestFullAuthFlow_RealRelay_RealHost_RealCrypto(t *testing.T) {
	srv := relay.NewServer()
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	base := "ws" + strings.TrimPrefix(ts.URL, "http")

	ws, created, err := connectHost(base)
	if err != nil {
		t.Fatalf("connectHost: %v", err)
	}
	defer func() { _ = ws.Close() }()

	keys, err := hostauth.NewKeyPair()
	if err != nil {
		t.Fatalf("NewKeyPair: %v", err)
	}
	const password = "correct-horse-battery-staple"
	active, ptmxCh := dummyControlState()
	go runHostMessageLoop(ws, created.SessionID, password, keys, active, ptmxCh, io.Discard)

	viewer, _, err := websocket.DefaultDialer.Dial(base+"/ws/viewer/"+created.SessionID, nil)
	if err != nil {
		t.Fatalf("dialing viewer: %v", err)
	}
	defer func() { _ = viewer.Close() }()

	viewerPub, ciphertext := encryptAsViewer(t, keys.PublicKeyBase64URL(), created.SessionID, password)
	if err := viewer.WriteJSON(protocol.AuthMsg{
		Envelope:           protocol.NewEnvelope("auth"),
		ViewerPubkeyBase64: viewerPub,
		CiphertextBase64:   ciphertext,
	}); err != nil {
		t.Fatalf("sending auth: %v", err)
	}

	_ = viewer.SetReadDeadline(time.Now().Add(2 * time.Second))
	var result protocol.AuthResultMsg
	if err := viewer.ReadJSON(&result); err != nil {
		t.Fatalf("ReadJSON auth_result: %v", err)
	}
	if !result.OK {
		t.Fatalf("auth_result.ok = false with the correct password, want true (real crypto, real relay, real host)")
	}
}

// TestHostSeesChatFromViewer_RealRelay proves the fix for a real gap
// found in review: chat_message previously had no case in
// runHostMessageLoop's dispatch at all, so a viewer's chat - the whole
// point of the "add context without taking control" feature per
// docs/ideas/getsloth.md - silently vanished before ever reaching
// anything the host could see.
func TestHostSeesChatFromViewer_RealRelay(t *testing.T) {
	srv := relay.NewServer()
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	base := "ws" + strings.TrimPrefix(ts.URL, "http")

	ws, created, err := connectHost(base)
	if err != nil {
		t.Fatalf("connectHost: %v", err)
	}
	defer func() { _ = ws.Close() }()

	keys, err := hostauth.NewKeyPair()
	if err != nil {
		t.Fatalf("NewKeyPair: %v", err)
	}
	const password = "chat-test-password"
	active, ptmxCh := dummyControlState()
	var chatOut stringBuffer
	go runHostMessageLoop(ws, created.SessionID, password, keys, active, ptmxCh, &chatOut)

	viewer, _, err := websocket.DefaultDialer.Dial(base+"/ws/viewer/"+created.SessionID, nil)
	if err != nil {
		t.Fatalf("dialing viewer: %v", err)
	}
	defer func() { _ = viewer.Close() }()

	viewerPub, ciphertext := encryptAsViewer(t, keys.PublicKeyBase64URL(), created.SessionID, password)
	if err := viewer.WriteJSON(protocol.AuthMsg{
		Envelope:           protocol.NewEnvelope("auth"),
		ViewerPubkeyBase64: viewerPub,
		CiphertextBase64:   ciphertext,
		DisplayName:        "Alex",
	}); err != nil {
		t.Fatalf("sending auth: %v", err)
	}
	_ = viewer.SetReadDeadline(time.Now().Add(2 * time.Second))
	var authResult protocol.AuthResultMsg
	if err := viewer.ReadJSON(&authResult); err != nil {
		t.Fatalf("ReadJSON auth_result: %v", err)
	}
	if !authResult.OK {
		t.Fatalf("auth failed, can't test chat")
	}

	if err := viewer.WriteJSON(protocol.ChatMsg{
		Envelope: protocol.NewEnvelope("chat_message"),
		Text:     "check the auth module first",
	}); err != nil {
		t.Fatalf("sending chat: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(chatOut.String(), "check the auth module first") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !strings.Contains(chatOut.String(), "check the auth module first") {
		t.Fatalf("host never saw the chat message; chatOut = %q", chatOut.String())
	}
	if !strings.Contains(chatOut.String(), "Alex") {
		t.Errorf("host's view of the chat doesn't include the sender's display name; chatOut = %q", chatOut.String())
	}
}

func TestFullAuthFlow_WrongPassword_Rejected(t *testing.T) {
	srv := relay.NewServer()
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	base := "ws" + strings.TrimPrefix(ts.URL, "http")

	ws, created, err := connectHost(base)
	if err != nil {
		t.Fatalf("connectHost: %v", err)
	}
	defer func() { _ = ws.Close() }()

	keys, err := hostauth.NewKeyPair()
	if err != nil {
		t.Fatalf("NewKeyPair: %v", err)
	}
	active, ptmxCh := dummyControlState()
	go runHostMessageLoop(ws, created.SessionID, "the-real-password", keys, active, ptmxCh, io.Discard)

	viewer, _, err := websocket.DefaultDialer.Dial(base+"/ws/viewer/"+created.SessionID, nil)
	if err != nil {
		t.Fatalf("dialing viewer: %v", err)
	}
	defer func() { _ = viewer.Close() }()

	viewerPub, ciphertext := encryptAsViewer(t, keys.PublicKeyBase64URL(), created.SessionID, "a-guess")
	if err := viewer.WriteJSON(protocol.AuthMsg{
		Envelope:           protocol.NewEnvelope("auth"),
		ViewerPubkeyBase64: viewerPub,
		CiphertextBase64:   ciphertext,
	}); err != nil {
		t.Fatalf("sending auth: %v", err)
	}

	_ = viewer.SetReadDeadline(time.Now().Add(2 * time.Second))
	var result protocol.AuthResultMsg
	if err := viewer.ReadJSON(&result); err != nil {
		t.Fatalf("ReadJSON auth_result: %v", err)
	}
	if result.OK {
		t.Fatal("auth_result.ok = true with the wrong password, want false")
	}
}
