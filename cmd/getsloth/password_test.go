package main

import (
	"strings"
	"testing"
)

func TestGeneratePassword_RespectsLengthAndAlphabet(t *testing.T) {
	for _, length := range []int{0, 1, defaultPasswordLength, 128} {
		password, err := generatePassword(length)
		if err != nil {
			t.Fatalf("generatePassword(%d): %v", length, err)
		}
		if len(password) != length {
			t.Errorf("generatePassword(%d) length = %d, want %d", length, len(password), length)
		}
		if strings.Trim(password, passwordAlphabet) != "" {
			t.Errorf("generatePassword(%d) contains a character outside passwordAlphabet: %q", length, password)
		}
	}
}

func TestGeneratePassword_ProducesIndependentValues(t *testing.T) {
	seen := make(map[string]struct{})
	for i := 0; i < 8; i++ {
		password, err := generatePassword(defaultPasswordLength)
		if err != nil {
			t.Fatalf("generatePassword: %v", err)
		}
		seen[password] = struct{}{}
	}

	if len(seen) < 2 {
		t.Fatalf("eight generated passwords produced fewer than two distinct values: %#v", seen)
	}
}
