package crypto

// NilCipher is used when PLANTRY_SECRET_KEY is unset. All encryption
// operations return ErrSecretKeyMissing; callers treat this as a runtime
// configuration error rather than a hard failure.
type NilCipher struct{}

func (NilCipher) Available() bool { return false }

func (NilCipher) Encrypt([]byte) (string, error) {
	return "", ErrSecretKeyMissing
}

func (NilCipher) Decrypt(string) ([]byte, error) {
	return nil, ErrSecretKeyMissing
}
