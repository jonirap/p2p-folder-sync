package crypto

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

// AuthMethod represents the authentication method
type AuthMethod string

const (
	AuthMethodPSK  AuthMethod = "psk"
	AuthMethodCert AuthMethod = "cert"
	AuthMethodTOFU AuthMethod = "tofu"
)

// Authenticator handles authentication
type Authenticator struct {
	method AuthMethod
	psk    []byte // Pre-shared key (if using PSK)
}

// NewAuthenticator creates a new authenticator
func NewAuthenticator(method AuthMethod, psk []byte) *Authenticator {
	return &Authenticator{
		method: method,
		psk:    psk,
	}
}

// GenerateChallenge generates an authentication challenge
func (a *Authenticator) GenerateChallenge() ([]byte, error) {
	challenge := make([]byte, 32)
	if _, err := rand.Read(challenge); err != nil {
		return nil, fmt.Errorf("failed to generate challenge: %w", err)
	}
	return challenge, nil
}

// GenerateResponse generates a response to a challenge (for PSK)
func (a *Authenticator) GenerateResponse(challenge []byte) ([]byte, error) {
	if a.method != AuthMethodPSK {
		return nil, fmt.Errorf("PSK response requires PSK method")
	}
	if len(a.psk) == 0 {
		return nil, fmt.Errorf("PSK not set")
	}

	mac := hmac.New(sha256.New, a.psk)
	mac.Write(challenge)
	return mac.Sum(nil), nil
}

// VerifyResponse verifies a challenge response
func (a *Authenticator) VerifyResponse(challenge, response []byte) bool {
	if a.method != AuthMethodPSK {
		// For other methods, verification would be different
		return false
	}
	if len(a.psk) == 0 {
		return false
	}

	expectedResponse, err := a.GenerateResponse(challenge)
	if err != nil {
		return false
	}

	return hmac.Equal(response, expectedResponse)
}

// EncodePSK encodes a PSK to base64
func EncodePSK(psk []byte) string {
	return base64.StdEncoding.EncodeToString(psk)
}

// DecodePSK decodes a base64-encoded PSK
func DecodePSK(encoded string) ([]byte, error) {
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("failed to decode PSK: %w", err)
	}
	return decoded, nil
}


