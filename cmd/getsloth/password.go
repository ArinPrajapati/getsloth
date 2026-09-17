package main

import (
	"crypto/rand"
	"math/big"
)

// passwordAlphabet excludes visually ambiguous characters (0/O, 1/l/I,
// etc.) since this gets read off a terminal and typed on a phone
// keyboard - the whole point of this product.
const passwordAlphabet = "abcdefghijkmnpqrstuvwxyzACDEFGHJKLMNPQRTUVWXY34679"

const defaultPasswordLength = 10

// generatePassword returns a random password of the given length drawn
// from passwordAlphabet, used when the host doesn't supply one via
// --password / GETSLOTH_PASSWORD.
func generatePassword(length int) (string, error) {
	b := make([]byte, length)
	max := big.NewInt(int64(len(passwordAlphabet)))
	for i := range b {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		b[i] = passwordAlphabet[n.Int64()]
	}
	return string(b), nil
}
