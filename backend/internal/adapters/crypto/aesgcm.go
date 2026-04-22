// Package crypto provides symmetric encryption for secrets stored at rest in
// the SQLite database (API keys, etc.).
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

// MinSecretLen is the minimum length of the PLANTRY_SECRET_KEY input. A long
// input does not improve cryptographic strength (we hash to 32 bytes) but it
// nudges operators away from trivially guessable keys.
const MinSecretLen = 32

// ErrSecretKeyMissing is returned by NilCipher when encryption is requested
// but no PLANTRY_SECRET_KEY was configured. Callers should surface this as a
// user-visible error pointing to env setup.
var ErrSecretKeyMissing = errors.New("PLANTRY_SECRET_KEY is not configured")

// Cipher encrypts and decrypts short values using AES-256-GCM with a random
// per-message nonce. The output of Encrypt is base64(nonce || ciphertext ||
// tag). The zero value is not usable; construct via New.
type Cipher interface {
	Encrypt(plaintext []byte) (string, error)
	Decrypt(encoded string) ([]byte, error)
	Available() bool
}

type aesCipher struct {
	aead cipher.AEAD
}

// New returns a Cipher derived from the given secret, or an error if the
// secret is too short. The secret is hashed with SHA-256 to produce a 32-byte
// AES key.
func New(secret string) (Cipher, error) {
	if len(secret) < MinSecretLen {
		return nil, fmt.Errorf("secret must be at least %d chars", MinSecretLen)
	}
	key := sha256.Sum256([]byte(secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("aes: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("gcm: %w", err)
	}
	return &aesCipher{aead: aead}, nil
}

func (c *aesCipher) Available() bool { return true }

func (c *aesCipher) Encrypt(plaintext []byte) (string, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("nonce: %w", err)
	}
	sealed := c.aead.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

func (c *aesCipher) Decrypt(encoded string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	ns := c.aead.NonceSize()
	if len(raw) < ns {
		return nil, errors.New("ciphertext too short")
	}
	nonce, ct := raw[:ns], raw[ns:]
	return c.aead.Open(nil, nonce, ct, nil)
}
