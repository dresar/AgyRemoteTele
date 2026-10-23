package session

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"sync"
	"time"
)

type AuthManager struct {
	mu             sync.RWMutex
	adminKey       string
	allowedChatIDs map[int64]bool
	pairingTokens  map[string]time.Time
}

func NewAuthManager(adminKey string, allowedIDs []int64) *AuthManager {
	allowedMap := make(map[int64]bool)
	for _, id := range allowedIDs {
		allowedMap[id] = true
	}

	return &AuthManager{
		adminKey:       strings.TrimSpace(adminKey),
		allowedChatIDs: allowedMap,
		pairingTokens:  make(map[string]time.Time),
	}
}

func (a *AuthManager) IsAuthorized(chatID int64, sess *Session, expiration time.Duration) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if len(a.allowedChatIDs) > 0 && a.allowedChatIDs[chatID] {
		return true
	}

	if sess != nil && sess.IsAuthenticated {
		if expiration > 0 && time.Since(sess.LastActiveAt) > expiration {
			return false
		}
		return true
	}

	return false
}

func (a *AuthManager) ValidateAdminKey(key string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.adminKey == "" {
		return false
	}
	return strings.TrimSpace(key) == a.adminKey
}

func (a *AuthManager) GeneratePairingToken() (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}

	token := hex.EncodeToString(bytes)
	a.pairingTokens[token] = time.Now().Add(15 * time.Minute)
	return token, nil
}

func (a *AuthManager) ConsumePairingToken(token string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	cleanToken := strings.TrimSpace(token)
	expiry, exists := a.pairingTokens[cleanToken]
	if !exists {
		return false
	}

	delete(a.pairingTokens, cleanToken)
	return time.Now().Before(expiry)
}

func (a *AuthManager) AddAllowedChatID(chatID int64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.allowedChatIDs[chatID] = true
}
