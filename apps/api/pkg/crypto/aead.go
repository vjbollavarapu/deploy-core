package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
)

const (
	aeadVersion     = 1
	dekSize         = 32
	gcmNonceSize    = 12
	platformKeySize = 32
)

var (
	ErrInvalidCiphertext = errors.New("invalid ciphertext")
	ErrWrongKeySize      = errors.New("platform key must be 32 bytes")
)

// Envelope is an AEAD ciphertext package stored for secrets.
// Format of Ciphertext:
//
//	version(1) || wrap_nonce(12) || wrapped_dek(48) || data_ciphertext
//
// Nonce holds the data-encryption nonce. KeyID identifies the platform key.
type Envelope struct {
	KeyID      string
	Nonce      []byte
	Ciphertext []byte
}

// NormalizePlatformKey accepts a raw 32-byte key or derives one via SHA-256
// from a longer passphrase (development convenience only).
func NormalizePlatformKey(raw []byte) ([]byte, error) {
	if len(raw) == platformKeySize {
		out := make([]byte, platformKeySize)
		copy(out, raw)
		return out, nil
	}
	if len(raw) == 0 {
		return nil, ErrWrongKeySize
	}
	sum := sha256.Sum256(raw)
	out := make([]byte, platformKeySize)
	copy(out, sum[:])
	return out, nil
}

// Seal encrypts plaintext with a per-record DEK wrapped by the platform key (AES-256-GCM).
func Seal(platformKey []byte, keyID string, plaintext []byte) (Envelope, error) {
	if len(platformKey) != platformKeySize {
		return Envelope{}, ErrWrongKeySize
	}
	if keyID == "" {
		return Envelope{}, errors.New("key id is required")
	}

	dek := make([]byte, dekSize)
	if _, err := rand.Read(dek); err != nil {
		return Envelope{}, fmt.Errorf("generate dek: %w", err)
	}

	dataNonce := make([]byte, gcmNonceSize)
	if _, err := rand.Read(dataNonce); err != nil {
		return Envelope{}, fmt.Errorf("generate data nonce: %w", err)
	}
	dataGCM, err := newGCM(dek)
	if err != nil {
		return Envelope{}, err
	}
	aad := []byte(keyID)
	dataCT := dataGCM.Seal(nil, dataNonce, plaintext, aad)

	wrapNonce := make([]byte, gcmNonceSize)
	if _, err := rand.Read(wrapNonce); err != nil {
		return Envelope{}, fmt.Errorf("generate wrap nonce: %w", err)
	}
	platformGCM, err := newGCM(platformKey)
	if err != nil {
		return Envelope{}, err
	}
	wrappedDEK := platformGCM.Seal(nil, wrapNonce, dek, aad)

	ct := make([]byte, 1+len(wrapNonce)+len(wrappedDEK)+len(dataCT))
	ct[0] = aeadVersion
	off := 1
	copy(ct[off:], wrapNonce)
	off += len(wrapNonce)
	copy(ct[off:], wrappedDEK)
	off += len(wrappedDEK)
	copy(ct[off:], dataCT)

	return Envelope{
		KeyID:      keyID,
		Nonce:      dataNonce,
		Ciphertext: ct,
	}, nil
}

// Open decrypts an Envelope produced by Seal.
func Open(platformKey []byte, env Envelope) ([]byte, error) {
	if len(platformKey) != platformKeySize {
		return nil, ErrWrongKeySize
	}
	if env.KeyID == "" || len(env.Nonce) != gcmNonceSize || len(env.Ciphertext) < 1+gcmNonceSize+dekSize+16 {
		return nil, ErrInvalidCiphertext
	}
	if env.Ciphertext[0] != aeadVersion {
		return nil, ErrInvalidCiphertext
	}

	aad := []byte(env.KeyID)
	off := 1
	wrapNonce := env.Ciphertext[off : off+gcmNonceSize]
	off += gcmNonceSize
	wrappedDEK := env.Ciphertext[off : off+dekSize+16]
	off += dekSize + 16
	dataCT := env.Ciphertext[off:]

	platformGCM, err := newGCM(platformKey)
	if err != nil {
		return nil, err
	}
	dek, err := platformGCM.Open(nil, wrapNonce, wrappedDEK, aad)
	if err != nil {
		return nil, ErrInvalidCiphertext
	}
	dataGCM, err := newGCM(dek)
	if err != nil {
		return nil, err
	}
	plain, err := dataGCM.Open(nil, env.Nonce, dataCT, aad)
	if err != nil {
		return nil, ErrInvalidCiphertext
	}
	return plain, nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
