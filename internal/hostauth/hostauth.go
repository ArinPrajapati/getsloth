// Package hostauth implements password verification for the host CLI,
// per docs/protocol.md's Crypto wire format. This is the one place
// password-comparison logic is allowed to live - internal/relay must
// never import this package, enforced by the depguard rule in
// .golangci.yml.
package hostauth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"math/big"

	"golang.org/x/crypto/hkdf"
)

// hkdfInfo is the fixed HKDF context string for domain separation, per
// docs/protocol.md's Crypto wire format table.
const hkdfInfo = "getsloth-v1-auth"

// KeyPair holds the host's ephemeral P-256 key pair for one session.
// The private key never leaves the process, is never serialized, and is
// discarded when the process exits.
type KeyPair struct {
	priv *ecdh.PrivateKey
}

// NewKeyPair generates a fresh ephemeral P-256 key pair.
func NewKeyPair() (*KeyPair, error) {
	priv, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generating host key pair: %w", err)
	}
	return &KeyPair{priv: priv}, nil
}

// PublicKeyBase64URL returns the raw uncompressed public key point,
// base64url-encoded (unpadded) - exactly the form the share URL's #k=
// fragment carries. This is the only place the public key is ever
// encoded; it never crosses the relay in any form (see
// docs/protocol.md's Share link format).
func (k *KeyPair) PublicKeyBase64URL() string {
	return base64.RawURLEncoding.EncodeToString(k.priv.PublicKey().Bytes())
}

// PublicKeyCompressedBase64URL returns the SEC1 *compressed* public key
// point (33 bytes: a 0x02/0x03 parity prefix + the X coordinate),
// base64url-encoded (unpadded) - half the size of PublicKeyBase64URL's
// raw uncompressed form. Used only for the terminal QR code's link (see
// cmd/getsloth/shareurl.go's qrShareURL), where fewer bytes measurably
// shrinks the QR's module count; the plain copy/paste link keeps using
// the uncompressed form so its documented wire format in
// docs/protocol.md is untouched.
//
// WebCrypto's `importKey('raw', ...)` for ECDH does not accept a
// compressed point, so the browser must decompress it back to the
// uncompressed 65-byte form first - see web/src/ec-point.ts, which
// implements exactly the inverse of crypto/elliptic's
// MarshalCompressed/UnmarshalCompressed used here. This isn't hand-rolled
// curve math on either side: both ends delegate to an established
// implementation (Go's stdlib here, the audited @noble/curves library in
// the browser), verified to interoperate byte-for-byte as part of this
// change.
func (k *KeyPair) PublicKeyCompressedBase64URL() string {
	uncompressed := k.priv.PublicKey().Bytes() // 0x04 || X(32) || Y(32)
	x := new(big.Int).SetBytes(uncompressed[1:33])
	y := new(big.Int).SetBytes(uncompressed[33:65])
	compressed := elliptic.MarshalCompressed(elliptic.P256(), x, y)
	return base64.RawURLEncoding.EncodeToString(compressed)
}

// VerifyPassword decrypts a viewer's auth attempt and reports whether it
// matches actual. A decryption failure (malformed ciphertext, wrong
// key, tampered data, wrong sessionID) is treated identically to a
// wrong password - deliberately, so a failed attempt doesn't leak
// whether the ciphertext itself was the problem. See docs/protocol.md's
// Auth flow step 5.
func (k *KeyPair) VerifyPassword(sessionID, viewerPubkeyBase64, ciphertextBase64, actual string) bool {
	attempt, err := k.decrypt(sessionID, viewerPubkeyBase64, ciphertextBase64)
	if err != nil {
		return false
	}
	// The AEAD decrypt above already makes attempt tamper-proof, but the
	// final comparison still needs to not leak timing information about
	// how many leading bytes matched - subtle.ConstantTimeCompare, not
	// ==, is the correct primitive for any password/secret comparison.
	return subtle.ConstantTimeCompare([]byte(attempt), []byte(actual)) == 1
}

func (k *KeyPair) decrypt(sessionID, viewerPubkeyBase64, ciphertextBase64 string) (string, error) {
	viewerPubBytes, err := base64.StdEncoding.DecodeString(viewerPubkeyBase64)
	if err != nil {
		return "", fmt.Errorf("decoding viewer public key: %w", err)
	}
	viewerPub, err := ecdh.P256().NewPublicKey(viewerPubBytes)
	if err != nil {
		return "", fmt.Errorf("parsing viewer public key: %w", err)
	}

	shared, err := k.priv.ECDH(viewerPub)
	if err != nil {
		return "", fmt.Errorf("ECDH: %w", err)
	}

	aesKey := make([]byte, 32)
	kdf := hkdf.New(sha256.New, shared, nil, []byte(hkdfInfo))
	if _, err := io.ReadFull(kdf, aesKey); err != nil {
		return "", fmt.Errorf("HKDF: %w", err)
	}

	blob, err := base64.StdEncoding.DecodeString(ciphertextBase64)
	if err != nil {
		return "", fmt.Errorf("decoding ciphertext: %w", err)
	}

	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return "", fmt.Errorf("aes.NewCipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("cipher.NewGCM: %w", err)
	}

	if len(blob) < gcm.NonceSize() {
		return "", errors.New("ciphertext shorter than the nonce it must be prefixed with")
	}
	nonce, ct := blob[:gcm.NonceSize()], blob[gcm.NonceSize():]
	aad := []byte("getsloth-auth-v1:" + sessionID)

	plaintext, err := gcm.Open(nil, nonce, ct, aad)
	if err != nil {
		return "", fmt.Errorf("AEAD open: %w", err)
	}
	return string(plaintext), nil
}
