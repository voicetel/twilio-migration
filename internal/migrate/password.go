package migrate

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

// passwordAlphabet is the character set for generated SIP credential passwords:
// letters and digits only, so the secret needs no escaping wherever it is later
// serialized (SIP digest, config, storage).
const passwordAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

// generatedPasswordLen is the length of a generated SIP credential password.
const generatedPasswordLen = 20

// generatePassword returns a cryptographically-random password.
//
// IMPORTANT: this tool never reads or copies a Twilio credential's password —
// Twilio does not expose it. Migrated credentials get a brand-new secret minted
// here on the VoiceML side; the caller is responsible for reporting it so the
// operator can redistribute it to devices. The plaintext is returned to the
// caller and is never written to disk by this package.
func generatePassword() (string, error) {
	b := make([]byte, generatedPasswordLen)
	max := big.NewInt(int64(len(passwordAlphabet)))
	for i := range b {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", fmt.Errorf("generate password: %w", err)
		}
		b[i] = passwordAlphabet[n.Int64()]
	}
	return string(b), nil
}
