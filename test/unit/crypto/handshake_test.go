package crypto_test

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"testing"

	"github.com/p2p-folder-sync/p2p-sync/internal/crypto"
)

func TestHandshakeManager_Creation(t *testing.T) {
	psk := []byte("test-pre-shared-key")
	hm, err := crypto.NewHandshakeManager(psk)
	if err != nil {
		t.Fatalf("Failed to create handshake manager: %v", err)
	}
	defer hm.Stop()

	if hm == nil {
		t.Fatal("Handshake manager is nil")
	}
}

func TestHandshakeManager_FullHandshake(t *testing.T) {
	// Create two handshake managers (simulating two peers)
	psk := []byte("shared-secret-key")
	hmA, err := crypto.NewHandshakeManager(psk)
	if err != nil {
		t.Fatalf("Failed to create handshake manager A: %v", err)
	}
	defer hmA.Stop()

	hmB, err := crypto.NewHandshakeManager(psk)
	if err != nil {
		t.Fatalf("Failed to create handshake manager B: %v", err)
	}
	defer hmB.Stop()

	// Step 1: Peer A initiates handshake
	pubKeyA, nonceA, challengeA, err := hmA.InitiateHandshake()
	if err != nil {
		t.Fatalf("Failed to initiate handshake: %v", err)
	}

	if pubKeyA == "" {
		t.Fatal("Public key A is empty")
	}
	if len(nonceA) != crypto.NonceSize {
		t.Fatalf("Invalid nonce A size: %d", len(nonceA))
	}
	if len(challengeA) != crypto.ChallengeSize {
		t.Fatalf("Invalid challenge A size: %d", len(challengeA))
	}

	// Step 2: Peer B processes handshake and generates response
	authResponseB, pubKeyB, nonceB, challengeB, stateB, err := hmB.ProcessHandshake(pubKeyA, nonceA, challengeA)
	if err != nil {
		t.Fatalf("Failed to process handshake: %v", err)
	}

	if len(authResponseB) == 0 {
		t.Fatal("Auth response B is empty")
	}
	if pubKeyB == "" {
		t.Fatal("Public key B is empty")
	}
	if len(nonceB) != crypto.NonceSize {
		t.Fatalf("Invalid nonce B size: %d", len(nonceB))
	}
	if len(challengeB) != crypto.ChallengeSize {
		t.Fatalf("Invalid challenge B size: %d", len(challengeB))
	}

	// Generate Peer A's auth response to B's challenge manually
	sharedSecretA, err := crypto.ComputeSharedSecret(hmA.GetPrivateKey(), hmB.GetPublicKey())
	if err != nil {
		t.Fatalf("Failed to compute shared secret A: %v", err)
	}

	authResponseA := generateAuthResponse(sharedSecretA, psk, challengeB)

	// Step 3: Peer B completes handshake
	sessionB, err := hmB.CompleteHandshake(stateB, authResponseA, "peer-a")
	if err != nil {
		t.Fatalf("Failed to complete handshake B: %v", err)
	}

	if sessionB == nil {
		t.Fatal("Session B is nil")
	}
	if len(sessionB.Key) != crypto.SessionKeySize {
		t.Fatalf("Invalid session key B size: %d", len(sessionB.Key))
	}

	// Step 4: Peer A verifies B's response and completes
	sessionA, err := hmA.VerifyAndComplete(nonceA, challengeA, pubKeyB, nonceB, challengeB, authResponseB, "peer-b")
	if err != nil {
		t.Fatalf("Failed to verify and complete handshake: %v", err)
	}

	if sessionA == nil {
		t.Fatal("Session A is nil")
	}
	if len(sessionA.Key) != crypto.SessionKeySize {
		t.Fatalf("Invalid session key A size: %d", len(sessionA.Key))
	}

	// Both peers should have derived the same session key
	if !bytes.Equal(sessionA.Key, sessionB.Key) {
		t.Fatalf("Session keys do not match:\nA: %x\nB: %x", sessionA.Key, sessionB.Key)
	}

	t.Logf("Handshake completed successfully")
	t.Logf("Session key: %x", sessionA.Key[:16])
}

func TestHandshakeManager_InvalidAuthentication(t *testing.T) {
	psk := []byte("shared-secret-key")
	hmA, err := crypto.NewHandshakeManager(psk)
	if err != nil {
		t.Fatalf("Failed to create handshake manager A: %v", err)
	}
	defer hmA.Stop()

	// Create second manager with different PSK
	differentPSK := []byte("different-secret-key")
	hmB, err := crypto.NewHandshakeManager(differentPSK)
	if err != nil {
		t.Fatalf("Failed to create handshake manager B: %v", err)
	}
	defer hmB.Stop()

	// Step 1: Peer A initiates handshake
	pubKeyA, nonceA, challengeA, err := hmA.InitiateHandshake()
	if err != nil {
		t.Fatalf("Failed to initiate handshake: %v", err)
	}

	// Step 2: Peer B processes handshake with different PSK
	authResponseB, pubKeyB, nonceB, challengeB, _, err := hmB.ProcessHandshake(pubKeyA, nonceA, challengeA)
	if err != nil {
		t.Fatalf("Failed to process handshake: %v", err)
	}

	// Step 3: Peer A tries to verify - should fail due to mismatched PSK
	_, err = hmA.VerifyAndComplete(nonceA, challengeA, pubKeyB, nonceB, challengeB, authResponseB, "peer-b")
	if err == nil {
		t.Fatal("Expected authentication to fail with mismatched PSK, but it succeeded")
	}

	if err.Error() != "authentication failed: invalid response" {
		t.Fatalf("Unexpected error message: %v", err)
	}

	t.Logf("Authentication correctly failed with mismatched PSK: %v", err)
}

func TestHandshakeManager_SessionRetrieval(t *testing.T) {
	psk := []byte("test-key")
	hm, err := crypto.NewHandshakeManager(psk)
	if err != nil {
		t.Fatalf("Failed to create handshake manager: %v", err)
	}
	defer hm.Stop()

	// Initially no session
	_, err = hm.GetSessionKey("peer-1")
	if err == nil {
		t.Fatal("Expected error for non-existent session, got nil")
	}

	// Test session validation
	isValid := hm.IsSessionValid("peer-1")
	if isValid {
		t.Fatal("Session should not be valid before handshake")
	}
}

func TestHandshakeManager_ChallengeResponse(t *testing.T) {
	psk := []byte("test-psk")
	hm, err := crypto.NewHandshakeManager(psk)
	if err != nil {
		t.Fatalf("Failed to create handshake manager: %v", err)
	}
	defer hm.Stop()

	// Create another manager to get a different public key
	hmPeer, err := crypto.NewHandshakeManager(psk)
	if err != nil {
		t.Fatalf("Failed to create peer handshake manager: %v", err)
	}
	defer hmPeer.Stop()

	// Generate handshake from peer
	pubKeyPeer, noncePeer, challengePeer, err := hmPeer.InitiateHandshake()
	if err != nil {
		t.Fatalf("Failed to initiate peer handshake: %v", err)
	}

	// Process handshake and generate response
	authResponse, _, _, _, _, err := hm.ProcessHandshake(pubKeyPeer, noncePeer, challengePeer)
	if err != nil {
		t.Fatalf("Failed to process handshake: %v", err)
	}

	// Auth response should be non-empty and correct length (HMAC-SHA256 = 32 bytes)
	if len(authResponse) != 32 {
		t.Fatalf("Invalid auth response length: %d, expected 32", len(authResponse))
	}

	t.Logf("Challenge-response generated successfully: %x", authResponse[:8])
}

func TestHandshakeManager_RemoveSession(t *testing.T) {
	psk := []byte("test-key")
	hm, err := crypto.NewHandshakeManager(psk)
	if err != nil {
		t.Fatalf("Failed to create handshake manager: %v", err)
	}
	defer hm.Stop()

	// Remove non-existent session should not panic
	hm.RemoveSession("non-existent-peer")

	// Test that removal works
	hm.RemoveSession("test-peer")
	isValid := hm.IsSessionValid("test-peer")
	if isValid {
		t.Fatal("Session should not be valid after removal")
	}

	t.Logf("Session removal works correctly")
}

// Helper function to generate auth response for testing
func generateAuthResponse(sharedSecret, psk, challenge []byte) []byte {
	key := sharedSecret
	if psk != nil {
		h := sha256.New()
		h.Write(sharedSecret)
		h.Write(psk)
		key = h.Sum(nil)
	}

	mac := hmac.New(sha256.New, key)
	mac.Write(challenge)
	return mac.Sum(nil)
}
