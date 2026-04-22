package crypto

import (
	"errors"
	"strings"
	"testing"
)

const testSecret = "0123456789abcdef0123456789abcdef" // 32 chars

func TestNew_RejectsShortSecret(t *testing.T) {
	if _, err := New("short"); err == nil {
		t.Fatal("expected error for short secret")
	}
}

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	c, err := New(testSecret)
	if err != nil {
		t.Fatal(err)
	}
	plaintext := []byte("sk-live-abc123")
	enc, err := c.Encrypt(plaintext)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(enc, string(plaintext)) {
		t.Fatal("encrypted output contains plaintext")
	}
	got, err := c.Decrypt(enc)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(plaintext) {
		t.Fatalf("round-trip mismatch: %q vs %q", got, plaintext)
	}
}

func TestDecrypt_Tamper(t *testing.T) {
	c, _ := New(testSecret)
	enc, _ := c.Encrypt([]byte("hello"))
	// flip a byte in the middle
	tampered := enc[:len(enc)-5] + "AAAAA"
	if _, err := c.Decrypt(tampered); err == nil {
		t.Fatal("expected decrypt error on tamper")
	}
}

func TestEncrypt_NonceUniqueness(t *testing.T) {
	c, _ := New(testSecret)
	seen := make(map[string]struct{}, 1000)
	for i := 0; i < 1000; i++ {
		enc, err := c.Encrypt([]byte("x"))
		if err != nil {
			t.Fatal(err)
		}
		if _, dup := seen[enc]; dup {
			t.Fatalf("duplicate ciphertext at iteration %d (nonce reuse)", i)
		}
		seen[enc] = struct{}{}
	}
}

func TestNilCipher(t *testing.T) {
	var c Cipher = NilCipher{}
	if c.Available() {
		t.Fatal("NilCipher.Available should be false")
	}
	if _, err := c.Encrypt([]byte("x")); !errors.Is(err, ErrSecretKeyMissing) {
		t.Fatalf("want ErrSecretKeyMissing, got %v", err)
	}
	if _, err := c.Decrypt("x"); !errors.Is(err, ErrSecretKeyMissing) {
		t.Fatalf("want ErrSecretKeyMissing, got %v", err)
	}
}
