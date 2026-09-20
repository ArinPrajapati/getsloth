package hostauth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"math/big"
	"testing"

	"golang.org/x/crypto/hkdf"
)

// encryptAsViewer implements the viewer side of docs/protocol.md's
// Crypto wire format from scratch (not by calling anything in this
// package) - the same construction a TypeScript/WebCrypto
// implementation would need to replicate: fresh ephemeral P-256 key,
// ECDH, HKDF-SHA256(salt=empty, info="getsloth-v1-auth"), AES-256-GCM
// with a random 12-byte nonce prepended to the ciphertext, AAD bound to
// "getsloth-auth-v1:"+sessionID. Exercising decryption against an
// independent implementation of the same spec is a stronger test than
// round-tripping through this package's own encrypt function would be
// (there is no encrypt function in this package - the host only ever
// decrypts).
func encryptAsViewer(t *testing.T, hostPubBytes []byte, sessionID, password string) (viewerPubBase64, ciphertextBase64 string) {
	t.Helper()

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
	kdf := hkdf.New(sha256.New, shared, nil, []byte(hkdfInfo))
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

func hostPubKeyBytes(t *testing.T, keys *KeyPair) []byte {
	t.Helper()
	b, err := base64.RawURLEncoding.DecodeString(keys.PublicKeyBase64URL())
	if err != nil {
		t.Fatalf("decoding host pubkey fragment encoding: %v", err)
	}
	return b
}

func TestPublicKeyCompressedBase64URL_RoundTripsToSamePoint(t *testing.T) {
	keys, err := NewKeyPair()
	if err != nil {
		t.Fatalf("NewKeyPair: %v", err)
	}

	compressedBytes, err := base64.RawURLEncoding.DecodeString(keys.PublicKeyCompressedBase64URL())
	if err != nil {
		t.Fatalf("decoding compressed fragment encoding: %v", err)
	}
	if len(compressedBytes) != 33 {
		t.Fatalf("compressed public key length = %d, want 33", len(compressedBytes))
	}
	if compressedBytes[0] != 0x02 && compressedBytes[0] != 0x03 {
		t.Fatalf("compressed public key prefix = 0x%02x, want 0x02 or 0x03", compressedBytes[0])
	}

	x, y := elliptic.UnmarshalCompressed(elliptic.P256(), compressedBytes)
	if x == nil {
		t.Fatal("UnmarshalCompressed rejected PublicKeyCompressedBase64URL's own output")
	}
	recompressed := elliptic.MarshalCompressed(elliptic.P256(), x, y)

	uncompressedBytes := hostPubKeyBytes(t, keys)
	wantX := new(big.Int).SetBytes(uncompressedBytes[1:33])
	wantY := new(big.Int).SetBytes(uncompressedBytes[33:65])
	if x.Cmp(wantX) != 0 || y.Cmp(wantY) != 0 {
		t.Fatalf("decompressed point does not match PublicKeyBase64URL's own X/Y")
	}
	if string(recompressed) != string(compressedBytes) {
		t.Fatal("recompressing the decompressed point did not reproduce the original compressed bytes")
	}
}

func TestVerifyPassword_CorrectPassword(t *testing.T) {
	keys, err := NewKeyPair()
	if err != nil {
		t.Fatalf("NewKeyPair: %v", err)
	}

	viewerPub, ciphertext := encryptAsViewer(t, hostPubKeyBytes(t, keys), "sess-123", "correct-horse-battery-staple")

	if !keys.VerifyPassword("sess-123", viewerPub, ciphertext, "correct-horse-battery-staple") {
		t.Error("VerifyPassword = false, want true for a correctly encrypted matching password")
	}
}

func TestVerifyPassword_WrongPassword(t *testing.T) {
	keys, err := NewKeyPair()
	if err != nil {
		t.Fatalf("NewKeyPair: %v", err)
	}

	viewerPub, ciphertext := encryptAsViewer(t, hostPubKeyBytes(t, keys), "sess-123", "wrong-password")

	if keys.VerifyPassword("sess-123", viewerPub, ciphertext, "correct-horse-battery-staple") {
		t.Error("VerifyPassword = true, want false for a mismatched password")
	}
}

func TestVerifyPassword_WrongSessionID_FailsAAD(t *testing.T) {
	keys, err := NewKeyPair()
	if err != nil {
		t.Fatalf("NewKeyPair: %v", err)
	}

	viewerPub, ciphertext := encryptAsViewer(t, hostPubKeyBytes(t, keys), "sess-123", "correct-horse-battery-staple")

	// Encrypted for "sess-123" but verified against a different session
	// ID - the AAD binding must make this fail closed, not succeed.
	if keys.VerifyPassword("sess-999", viewerPub, ciphertext, "correct-horse-battery-staple") {
		t.Error("VerifyPassword = true across mismatched session IDs, want false (AAD binding not enforced)")
	}
}

func TestVerifyPassword_MalformedCiphertext_FailsClosed(t *testing.T) {
	keys, err := NewKeyPair()
	if err != nil {
		t.Fatalf("NewKeyPair: %v", err)
	}

	viewerPriv, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generating viewer key: %v", err)
	}
	viewerPub := base64.StdEncoding.EncodeToString(viewerPriv.PublicKey().Bytes())

	// Not valid ciphertext at all - decryption itself must fail, and per
	// docs/protocol.md that's treated identically to a wrong password,
	// not a distinct error.
	if keys.VerifyPassword("sess-123", viewerPub, "not-valid-base64-ciphertext!!!", "anything") {
		t.Error("VerifyPassword = true for malformed ciphertext, want false")
	}
}
