package security

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"strings"
)

func SignEd25519(private ed25519.PrivateKey, message []byte) string {
	public := private.Public().(ed25519.PublicKey)
	signature := ed25519.Sign(private, message)
	return base64.RawURLEncoding.EncodeToString(public) + "." + base64.RawURLEncoding.EncodeToString(signature)
}

func VerifyEd25519(encoded string, message []byte) bool {
	parts := strings.Split(encoded, ".")
	if len(parts) != 2 {
		return false
	}
	public, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || len(public) != ed25519.PublicKeySize {
		return false
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || len(signature) != ed25519.SignatureSize {
		return false
	}
	return ed25519.Verify(ed25519.PublicKey(public), message, signature)
}

func ValidatePrivateKey(encoded []byte) error {
	if len(encoded) != ed25519.PrivateKeySize {
		return fmt.Errorf("clave de firma inválida")
	}
	return nil
}
