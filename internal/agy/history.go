package agy

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type ConversationSummary struct {
	ConversationID string    `json:"conversation_id"`
	Profile        string    `json:"profile"`
	StartedAt      time.Time `json:"started_at"`
	LastUpdatedAt  time.Time `json:"last_updated_at"`
	TurnCount      int       `json:"turn_count"`
	FirstPrompt    string    `json:"first_prompt"`
	LastResponse   string    `json:"last_response"`
}

type TranscriptItem struct {
	StepIndex int                  `json:"step_index"`
	Source    string               `json:"source"`
	Type      string               `json:"type"`
	Status    string               `json:"status"`
	CreatedAt string               `json:"created_at"`
	Content   string               `json:"content"`
	Thinking  string               `json:"thinking"`
	ToolCalls []TranscriptToolCall `json:"tool_calls"`
}

type TranscriptToolCall struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

type HistoryReader struct {
	profileManager *ProfileManager
}

func NewHistoryReader(pm *ProfileManager) *HistoryReader {
	return &HistoryReader{
		profileManager: pm,
	}
}

func (h *HistoryReader) ListRecentConversations(profile string, limit int) ([]ConversationSummary, error) {
	brainDir := filepath.Join(h.profileManager.GetProfileHome(profile), ".gemini", "antigravity-cli", "brain")
	entries, err := os.ReadDir(brainDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	if limit <= 0 {
		limit = 10
	}

	type candidate struct {
		convID  string
		logPath string
		modTime time.Time
	}

	var candidates []candidate
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		convID := entry.Name()
		logPath := filepath.Join(brainDir, convID, ".system_generated", "logs", "transcript.jsonl")
		info, err := os.Stat(logPath)
		if err != nil {
			continue
		}
		candidates = append(candidates, candidate{
			convID:  convID,
			logPath: logPath,
			modTime: info.ModTime(),
		})
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].modTime.After(candidates[j].modTime)
	})

	var summaries []ConversationSummary
	for i, c := range candidates {
		if i >= limit {
			break
		}

		sum, err := h.readConversationSummary(profile, c.convID, c.logPath, c.modTime)
		if err == nil {
			summaries = append(summaries, *sum)
		}
	}

	return summaries, nil
}

func (h *HistoryReader) readConversationSummary(profile, convID, logPath string, modTime time.Time) (*ConversationSummary, error) {
	file, err := os.Open(logPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024)

	var firstPrompt string
	var lastResponse string
	var startedAt time.Time
	turnCount := 0

	for scanner.Scan() {
		line := scanner.Text()
		var item TranscriptItem
		if err := json.Unmarshal([]byte(line), &item); err != nil {
			continue
		}

		if startedAt.IsZero() && item.CreatedAt != "" {
			if t, err := time.Parse(time.RFC3339Nano, item.CreatedAt); err == nil {
				startedAt = t
			} else if t, err := time.Parse(time.RFC3339, item.CreatedAt); err == nil {
				startedAt = t
			}
		}

		if item.Type == "USER_INPUT" {
			turnCount++
			if firstPrompt == "" {
				firstPrompt = cleanPromptText(item.Content)
			}
		}

		if item.Type == "PLANNER_RESPONSE" && item.Content != "" {
			lastResponse = cleanResponseText(item.Content)
		}
	}

	if startedAt.IsZero() {
		startedAt = modTime
	}

	if firstPrompt == "" {
		firstPrompt = "(No prompt text recorded)"
	}

	return &ConversationSummary{
		ConversationID: convID,
		Profile:        profile,
		StartedAt:      startedAt,
		LastUpdatedAt:  modTime,
		TurnCount:      turnCount,
		FirstPrompt:    firstPrompt,
		LastResponse:   lastResponse,
	}, nil
}

func (h *HistoryReader) ReadFullTranscript(profile, convID string) ([]TranscriptItem, error) {
	logPath := filepath.Join(h.profileManager.GetProfileHome(profile), ".gemini", "antigravity-cli", "brain", convID, ".system_generated", "logs", "transcript.jsonl")
	file, err := os.Open(logPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024)

	var items []TranscriptItem
	for scanner.Scan() {
		line := scanner.Text()
		var item TranscriptItem
		if err := json.Unmarshal([]byte(line), &item); err == nil {
			items = append(items, item)
		}
	}

	return items, scanner.Err()
}

func cleanPromptText(raw string) string {
	idx := strings.Index(raw, "<USER_REQUEST>")
	if idx != -1 {
		endIdx := strings.Index(raw, "</USER_REQUEST>")
		if endIdx != -1 && endIdx > idx {
			raw = raw[idx+len("<USER_REQUEST>") : endIdx]
		}
	}
	raw = strings.TrimSpace(raw)
	lines := strings.Split(raw, "\n")
	if len(lines) > 0 {
		first := strings.TrimSpace(lines[0])
		if len(first) > 120 {
			return first[:117] + "..."
		}
		return first
	}
	return raw
}

func cleanResponseText(raw string) string {
	raw = strings.TrimSpace(raw)
	lines := strings.Split(raw, "\n")
	if len(lines) > 0 {
		first := strings.TrimSpace(lines[0])
		if len(first) > 160 {
			return first[:157] + "..."
		}
		return first
	}
	return raw
}
