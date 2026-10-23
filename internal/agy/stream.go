package agy

import (
	"encoding/json"
	"strings"
)

type ToolInvocation struct {
	Name            string         `json:"name"`
	Parameters      map[string]any `json:"parameters"`
	Output          string         `json:"output"`
	DurationSeconds float64        `json:"duration_seconds"`
	State           string         `json:"state"`
}

type StreamResult struct {
	ConversationID  string           `json:"conversation_id"`
	Status          string           `json:"status"`
	Response        string           `json:"response"`
	DurationSeconds float64          `json:"duration_seconds"`
	NumTurns        int              `json:"num_turns"`
	InputTokens     int              `json:"input_tokens"`
	OutputTokens    int              `json:"output_tokens"`
	TotalTokens     int              `json:"total_tokens"`
	ToolsInvoked    []ToolInvocation `json:"tools_invoked"`
}

type rawStreamEvent struct {
	Event          string          `json:"event"`
	ConversationID string          `json:"conversation_id"`
	Status         string          `json:"status"`
	Response       string          `json:"response"`
	DurationSec    float64         `json:"duration_seconds"`
	NumTurns       int             `json:"num_turns"`
	Usage          *rawUsage       `json:"usage"`
	StepUpdate     *rawStepUpdate  `json:"step_update"`
	Result         *rawResultBlock `json:"result"`
}

type rawResultBlock struct {
	ConversationID string    `json:"conversation_id"`
	Status         string    `json:"status"`
	Response       string    `json:"response"`
	DurationSec    float64   `json:"duration_seconds"`
	NumTurns       int       `json:"num_turns"`
	Usage          *rawUsage `json:"usage"`
}

type rawUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

type rawStepUpdate struct {
	ConversationID string       `json:"conversation_id"`
	StepIndex      int          `json:"step_index"`
	State          string       `json:"state"`
	StepType       string       `json:"step_type"`
	ToolName       string       `json:"tool_name"`
	TextDelta      string       `json:"text_delta"`
	DurationSec    float64      `json:"duration_seconds"`
	ToolInfo       *rawToolInfo `json:"tool_info"`
}

type rawToolInfo struct {
	Name       string         `json:"name"`
	Parameters map[string]any `json:"parameters"`
	Output     string         `json:"output"`
}

type StreamParser struct {
	Result        StreamResult
	responseParts strings.Builder
	activeTools   map[string]int
}

func NewStreamParser() *StreamParser {
	return &StreamParser{
		activeTools: make(map[string]int),
	}
}

func (p *StreamParser) FeedLine(line string) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return
	}

	var event rawStreamEvent
	if err := json.Unmarshal([]byte(trimmed), &event); err != nil {
		return
	}

	if event.Result != nil {
		p.Result.ConversationID = event.Result.ConversationID
		p.Result.Status = event.Result.Status
		if event.Result.Response != "" {
			p.Result.Response = event.Result.Response
		}
		p.Result.DurationSeconds = event.Result.DurationSec
		p.Result.NumTurns = event.Result.NumTurns
		if event.Result.Usage != nil {
			p.Result.InputTokens = event.Result.Usage.InputTokens
			p.Result.OutputTokens = event.Result.Usage.OutputTokens
			p.Result.TotalTokens = event.Result.Usage.TotalTokens
		}
		return
	}

	if event.ConversationID != "" && event.Status != "" {
		p.Result.ConversationID = event.ConversationID
		p.Result.Status = event.Status
		p.Result.Response = event.Response
		p.Result.DurationSeconds = event.DurationSec
		p.Result.NumTurns = event.NumTurns
		if event.Usage != nil {
			p.Result.InputTokens = event.Usage.InputTokens
			p.Result.OutputTokens = event.Usage.OutputTokens
			p.Result.TotalTokens = event.Usage.TotalTokens
		}
		return
	}

	if event.StepUpdate != nil {
		update := event.StepUpdate
		if update.ConversationID != "" && p.Result.ConversationID == "" {
			p.Result.ConversationID = update.ConversationID
		}

		if update.StepType == "agent_response" && update.TextDelta != "" {
			p.responseParts.WriteString(update.TextDelta)
		}

		if update.StepType == "tool" {
			name := update.ToolName
			if name == "" && update.ToolInfo != nil {
				name = update.ToolInfo.Name
			}

			var params map[string]any
			var output string
			if update.ToolInfo != nil {
				params = update.ToolInfo.Parameters
				output = update.ToolInfo.Output
			}

			inv := ToolInvocation{
				Name:            name,
				Parameters:      params,
				Output:          output,
				DurationSeconds: update.DurationSec,
				State:           update.State,
			}

			if update.State == "DONE" {
				p.Result.ToolsInvoked = append(p.Result.ToolsInvoked, inv)
			}
		}
	}
}

func (p *StreamParser) Finalize() StreamResult {
	if p.Result.Response == "" && p.responseParts.Len() > 0 {
		p.Result.Response = p.responseParts.String()
	}
	if p.Result.Status == "" {
		p.Result.Status = "COMPLETED"
	}
	return p.Result
}
