package bot

import (
	"sync"
	"time"

	"github.com/dresar/AgyRemoteTele/internal/session"
)

type RateLimiter struct {
	mu       sync.Mutex
	requests map[int64][]time.Time
	limit    int
	window   time.Duration
}

func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	if limit <= 0 {
		limit = 30
	}
	if window <= 0 {
		window = time.Minute
	}
	return &RateLimiter{
		requests: make(map[int64][]time.Time),
		limit:    limit,
		window:   window,
	}
}

func (rl *RateLimiter) Allow(chatID int64) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	timestamps := rl.requests[chatID]
	var valid []time.Time
	for _, t := range timestamps {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}

	if len(valid) >= rl.limit {
		rl.requests[chatID] = valid
		return false
	}

	valid = append(valid, now)
	rl.requests[chatID] = valid
	return true
}

type Middleware struct {
	authManager *session.AuthManager
	store       *session.Store
	rateLimiter *RateLimiter
	expiration  time.Duration
}

func NewMiddleware(authManager *session.AuthManager, store *session.Store, limit int, expirationHours int) *Middleware {
	exp := time.Duration(expirationHours) * time.Hour
	return &Middleware{
		authManager: authManager,
		store:       store,
		rateLimiter: NewRateLimiter(limit, time.Minute),
		expiration:  exp,
	}
}

func (m *Middleware) ProcessAccess(chatID int64, username string, isLoginCmd bool) (*session.Session, bool, bool) {
	sess := m.store.GetOrCreate(chatID, username)

	if !m.rateLimiter.Allow(chatID) {
		return sess, false, true
	}

	if isLoginCmd {
		return sess, true, false
	}

	authorized := m.authManager.IsAuthorized(chatID, sess, m.expiration)
	return sess, authorized, false
}
