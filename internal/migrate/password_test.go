package migrate

import (
	"crypto/rand"
	"errors"
	"strings"
	"testing"
)

// failingReader always errors, so it deterministically exercises
// generatePassword's rand.Int failure branch without depending on a real
// randomness failure ever occurring.
type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("no entropy") }

func TestGeneratePassword_RandError(t *testing.T) {
	orig := rand.Reader
	rand.Reader = failingReader{}
	defer func() { rand.Reader = orig }()

	if _, err := generatePassword(); err == nil {
		t.Fatal("expected an error when the random source fails")
	}
}

func TestGeneratePassword(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 50; i++ {
		pw, err := generatePassword()
		if err != nil {
			t.Fatalf("generatePassword: %v", err)
		}
		if len(pw) != generatedPasswordLen {
			t.Fatalf("length = %d, want %d", len(pw), generatedPasswordLen)
		}
		for _, r := range pw {
			if !strings.ContainsRune(passwordAlphabet, r) {
				t.Fatalf("password %q contains out-of-alphabet rune %q", pw, r)
			}
		}
		if seen[pw] {
			t.Fatalf("generated a duplicate password %q — not random", pw)
		}
		seen[pw] = true
	}
}
