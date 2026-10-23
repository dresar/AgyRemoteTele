package session

import (
	"testing"
	"time"
)

func TestAuthManager(t *testing.T) {
	allowedIDs := []int64{111, 222}
	mgr := NewAuthManager("secret-key", allowedIDs)

	if !mgr.ValidateAdminKey("secret-key") {
		t.Error("expected valid admin key")
	}
	if mgr.ValidateAdminKey("wrong-key") {
		t.Error("expected invalid admin key")
	}

	if !mgr.IsAuthorized(111, nil, 0) {
		t.Error("expected whitelisted chat id to be authorized")
	}
	if mgr.IsAuthorized(333, nil, 0) {
		t.Error("expected unknown chat id to be unauthorized")
	}

	sess := &Session{
		ChatID:          333,
		IsAuthenticated: true,
		LastActiveAt:    time.Now(),
	}
	if !mgr.IsAuthorized(333, sess, 1*time.Hour) {
		t.Error("expected active session to be authorized")
	}

	expiredSess := &Session{
		ChatID:          333,
		IsAuthenticated: true,
		LastActiveAt:    time.Now().Add(-2 * time.Hour),
	}
	if mgr.IsAuthorized(333, expiredSess, 1*time.Hour) {
		t.Error("expected expired session to be unauthorized")
	}

	token, err := mgr.GeneratePairingToken()
	if err != nil || token == "" {
		t.Fatalf("failed to generate pairing token: %v", err)
	}

	if !mgr.ConsumePairingToken(token) {
		t.Error("expected valid token consumption")
	}
	if mgr.ConsumePairingToken(token) {
		t.Error("expected token to be consumed only once")
	}
}
