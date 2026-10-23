package agy

import (
	"testing"
)

func TestStreamParser(t *testing.T) {
	parser := NewStreamParser()

	l1 := `{"event":"step_update","step_update":{"conversation_id":"conv-test-1","step_index":1,"state":"DONE","step_type":"tool","tool_name":"run_command","tool_info":{"name":"run_command","parameters":{"CommandLine":"go test ./..."},"output":"PASS"}}}`
	l2 := `{"event":"step_update","step_update":{"conversation_id":"conv-test-1","step_index":2,"state":"DONE","step_type":"agent_response","text_delta":"Task completed successfully."}}`
	l3 := `{"event":"result","result":{"conversation_id":"conv-test-1","status":"SUCCESS","response":"Task completed successfully.","duration_seconds":3.5,"num_turns":1,"usage":{"input_tokens":100,"output_tokens":50,"total_tokens":150}}}`

	parser.FeedLine(l1)
	parser.FeedLine(l2)
	parser.FeedLine(l3)

	res := parser.Finalize()

	if res.ConversationID != "conv-test-1" {
		t.Errorf("expected conv-test-1, got %s", res.ConversationID)
	}
	if res.Status != "SUCCESS" {
		t.Errorf("expected SUCCESS, got %s", res.Status)
	}
	if res.Response != "Task completed successfully." {
		t.Errorf("expected task response, got %s", res.Response)
	}
	if len(res.ToolsInvoked) != 1 {
		t.Fatalf("expected 1 tool invocation, got %d", len(res.ToolsInvoked))
	}
	if res.ToolsInvoked[0].Name != "run_command" {
		t.Errorf("expected run_command tool, got %s", res.ToolsInvoked[0].Name)
	}
	if res.TotalTokens != 150 {
		t.Errorf("expected 150 tokens, got %d", res.TotalTokens)
	}
}
