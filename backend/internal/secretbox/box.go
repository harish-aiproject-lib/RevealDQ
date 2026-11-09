// Package secretbox encrypts connection secrets at rest (AES-256-GCM).
// Ciphertext is stored as "enc:v1:" + base64(nonce|ciphertext). Values without
// that prefix are treated as legacy plaintext so existing rows keep working
// until they are migrated.
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"strings"
)

const prefix = "enc:v1:"

type Box struct {
	aead cipher.AEAD
}

// New derives a 256-bit key. dataKey is preferred (DATA_ENCRYPTION_KEY).
// When it is empty the key is derived from the JWT secret so local installs
// still encrypt at rest. Rotating either value makes old ciphertext unreadable.
func New(dataKey, jwtSecret string) (*Box, error) {
	material := strings.TrimSpace(dataKey)
	if material == "" {
		material = "reality-reveal-conn:" + jwtSecret
	}
	sum := sha256.Sum256([]byte(material))
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: aead}, nil
}

func (b *Box) Seal(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	if strings.HasPrefix(plain, prefix) {
		return plain, nil
	}
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	out := b.aead.Seal(nonce, nonce, []byte(plain), nil)
	return prefix + base64.StdEncoding.EncodeToString(out), nil
}

func (b *Box) Open(stored string) (string, error) {
	if stored == "" || !strings.HasPrefix(stored, prefix) {
		return stored, nil
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, prefix))
	if err != nil {
		return "", err
	}
	ns := b.aead.NonceSize()
	if len(raw) < ns {
		return "", errors.New("ciphertext too short")
	}
	plain, err := b.aead.Open(nil, raw[:ns], raw[ns:], nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func IsSealed(stored string) bool {
	return strings.HasPrefix(stored, prefix)
}
