package generate

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// RandomSecret returns n bytes of crypto-random data, base64-encoded.
// Used for SESSION_SECRET.
func RandomSecret(nBytes int) (string, error) {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

// RandomPassword returns a human-typeable random password (base64url of n
// bytes, no padding). Strong enough for an admin login while remaining
// pasteable. The plaintext is shown once in the checklist and never stored.
func RandomPassword(nBytes int) (string, error) {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// BcryptHash hashes a password with bcrypt cost 10 (same as the CRM expects).
func BcryptHash(password string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(password), 10)
	if err != nil {
		return "", fmt.Errorf("bcrypt: %w", err)
	}
	return string(h), nil
}
