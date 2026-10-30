package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
)

type Cipher struct{ aead cipher.AEAD }

func NewCipher(encodedKey string) (*Cipher, error) {
	key, err := base64.StdEncoding.DecodeString(encodedKey)
	if err != nil {
		return nil, fmt.Errorf("DATA_ENCRYPTION_KEY debe ser Base64")
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("DATA_ENCRYPTION_KEY debe decodificar a 32 bytes (AES-256)")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Cipher{aead: aead}, nil
}
func (c *Cipher) Encrypt(plain []byte) ([]byte, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return c.aead.Seal(nonce, nonce, plain, nil), nil
}
func (c *Cipher) Decrypt(sealed []byte) ([]byte, error) {
	if len(sealed) < c.aead.NonceSize() {
		return nil, fmt.Errorf("dato cifrado inválido")
	}
	return c.aead.Open(nil, sealed[:c.aead.NonceSize()], sealed[c.aead.NonceSize():], nil)
}
func Sign(secret []byte, message []byte) string {
	m := hmac.New(sha256.New, secret)
	m.Write(message)
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}
