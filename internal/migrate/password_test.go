package migrate

import (
	"strings"
	"testing"
)

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
