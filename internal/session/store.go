package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Session struct {
	ChatID               int64     `json:"chat_id"`
	Username             string    `json:"username"`
	IsAuthenticated      bool      `json:"is_authenticated"`
	AuthenticatedAt      time.Time `json:"authenticated_at"`
	LastActiveAt         time.Time `json:"last_active_at"`
	ActiveProfile        string    `json:"active_profile"`
	ActiveConversationID string    `json:"active_conversation_id"`
	ActiveWorkingDir     string    `json:"active_working_dir"`
	LastBotMessageIDs    []int     `json:"last_bot_message_ids"`
	LastUserMessageID    int       `json:"last_user_message_id"`
}

type Store struct {
	mu                sync.RWMutex
	filePath          string
	sessions          map[int64]*Session
	defaultProfile    string
	defaultWorkingDir string
}

func NewStore(filePath string, defaultProfile string, defaultWorkingDir string) (*Store, error) {
	s := &Store{
		filePath:          filePath,
		sessions:          make(map[int64]*Session),
		defaultProfile:    defaultProfile,
		defaultWorkingDir: defaultWorkingDir,
	}

	if err := s.Load(); err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	return s, nil
}

func (s *Store) GetOrCreate(chatID int64, username string) *Session {
	s.mu.Lock()
	defer s.mu.Unlock()

	sess, exists := s.sessions[chatID]
	if !exists {
		sess = &Session{
			ChatID:           chatID,
			Username:         username,
			IsAuthenticated:  false,
			LastActiveAt:     time.Now().UTC(),
			ActiveProfile:    s.defaultProfile,
			ActiveWorkingDir: s.defaultWorkingDir,
		}
		s.sessions[chatID] = sess
		_ = s.saveLocked()
		return sess
	}

	if username != "" && sess.Username != username {
		sess.Username = username
	}
	sess.LastActiveAt = time.Now().UTC()
	return sess
}

func (s *Store) Get(chatID int64) (*Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sess, exists := s.sessions[chatID]
	if !exists {
		return nil, false
	}
	cpy := *sess
	return &cpy, true
}

func (s *Store) SetProfile(chatID int64, profile string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if sess, ok := s.sessions[chatID]; ok {
		sess.ActiveProfile = profile
		sess.LastActiveAt = time.Now().UTC()
		_ = s.saveLocked()
	}
}

func (s *Store) SetConversation(chatID int64, conversationID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if sess, ok := s.sessions[chatID]; ok {
		sess.ActiveConversationID = conversationID
		sess.LastActiveAt = time.Now().UTC()
		_ = s.saveLocked()
	}
}

func (s *Store) ClearConversation(chatID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if sess, ok := s.sessions[chatID]; ok {
		sess.ActiveConversationID = ""
		sess.LastActiveAt = time.Now().UTC()
		_ = s.saveLocked()
	}
}

func (s *Store) SetWorkingDir(chatID int64, dir string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if sess, ok := s.sessions[chatID]; ok {
		sess.ActiveWorkingDir = dir
		sess.LastActiveAt = time.Now().UTC()
		_ = s.saveLocked()
	}
}

func (s *Store) Authenticate(chatID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if sess, ok := s.sessions[chatID]; ok {
		sess.IsAuthenticated = true
		sess.AuthenticatedAt = time.Now().UTC()
		sess.LastActiveAt = time.Now().UTC()
		_ = s.saveLocked()
	}
}

func (s *Store) Deauthenticate(chatID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if sess, ok := s.sessions[chatID]; ok {
		sess.IsAuthenticated = false
		sess.ActiveConversationID = ""
		sess.LastActiveAt = time.Now().UTC()
		_ = s.saveLocked()
	}
}

func (s *Store) Touch(chatID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if sess, ok := s.sessions[chatID]; ok {
		sess.LastActiveAt = time.Now().UTC()
	}
}

func (s *Store) IsExpired(sess *Session, maxAge time.Duration) bool {
	if sess == nil {
		return true
	}
	if maxAge <= 0 {
		return false
	}
	return time.Since(sess.LastActiveAt) > maxAge
}

func (s *Store) SetLastBotMessageIDs(chatID int64, ids []int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if sess, ok := s.sessions[chatID]; ok {
		sess.LastBotMessageIDs = ids
		_ = s.saveLocked()
	}
}

func (s *Store) ClearLastBotMessageIDs(chatID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if sess, ok := s.sessions[chatID]; ok {
		sess.LastBotMessageIDs = nil
		_ = s.saveLocked()
	}
}

func (s *Store) SetLastUserMessageID(chatID int64, id int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if sess, ok := s.sessions[chatID]; ok {
		sess.LastUserMessageID = id
		_ = s.saveLocked()
	}
}

func (s *Store) Persist() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked()
}

func (s *Store) saveLocked() error {
	if s.filePath == "" {
		return nil
	}

	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(s.sessions, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.filePath, data, 0600)
}

func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.filePath == "" {
		return nil
	}

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return err
	}

	var loaded map[int64]*Session
	if err := json.Unmarshal(data, &loaded); err != nil {
		return err
	}

	s.sessions = loaded
	return nil
}
