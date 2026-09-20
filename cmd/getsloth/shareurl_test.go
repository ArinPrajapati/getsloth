package main

import "testing"

func TestShareURL(t *testing.T) {
	got := shareURL("https://getsloth.dev", "x7k2", "abc123")
	want := "https://getsloth.dev/s/x7k2#k=abc123"
	if got != want {
		t.Fatalf("shareURL() = %q, want %q", got, want)
	}
}

func TestQRShareURLIncludesPassword(t *testing.T) {
	got := qrShareURL("https://getsloth.dev", "x7k2", "abc123", "D4j7ztfHPP")
	want := "https://getsloth.dev/s/x7k2#k=abc123&p=D4j7ztfHPP"
	if got != want {
		t.Fatalf("qrShareURL() = %q, want %q", got, want)
	}
}

func TestQRShareURLEscapesPassword(t *testing.T) {
	// generatePassword only ever draws from passwordAlphabet (no
	// URL-hostile characters), but a founder-supplied GETSLOTH_PASSWORD
	// could contain anything - the fragment must stay parseable either way.
	got := qrShareURL("https://getsloth.dev", "x7k2", "abc123", "a b&c=d")
	want := "https://getsloth.dev/s/x7k2#k=abc123&p=a+b%26c%3Dd"
	if got != want {
		t.Fatalf("qrShareURL() = %q, want %q", got, want)
	}
}
