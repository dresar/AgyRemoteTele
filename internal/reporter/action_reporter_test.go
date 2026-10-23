package reporter

import (
	"strings"
	"testing"

	"github.com/dresar/AgyRemoteTele/internal/agy"
)

func TestBuildReport(t *testing.T) {
	streamRes := &agy.StreamResult{
		ConversationID:  "conv-999",
		Status:          "SUCCESS",
		Response:        "Created endpoint /api/auth and added unit tests.\n\nAll tests passed successfully.",
		DurationSeconds: 14.2,
		NumTurns:        1,
		ToolsInvoked: []agy.ToolInvocation{
			{
				Name: "write_file",
				Parameters: map[string]any{
					"Path": "internal/auth/handler.go",
				},
				Output: "file created",
			},
			{
				Name: "run_command",
				Parameters: map[string]any{
					"CommandLine": "go test ./...",
				},
				Output: "PASS",
			},
		},
	}

	report := BuildReport("agy04", streamRes, "/tmp")

	if report.Profile != "agy04" {
		t.Errorf("expected agy04, got %s", report.Profile)
	}
	if report.DurationSeconds != 14.2 {
		t.Errorf("expected 14.2, got %f", report.DurationSeconds)
	}
	if len(report.ToolsInvoked) != 2 {
		t.Fatalf("expected 2 tools invoked, got %d", len(report.ToolsInvoked))
	}

	htmlCard := report.RenderHTML()
	if !strings.Contains(htmlCard, "TASK EXECUTION COMPLETED") {
		t.Errorf("expected execution card header")
	}
	if !strings.Contains(htmlCard, "<code>agy04</code>") {
		t.Errorf("expected profile in card")
	}
	if !strings.Contains(htmlCard, "<code>write_file</code>") {
		t.Errorf("expected write_file in card")
	}
	if !strings.Contains(htmlCard, "<code>go test ./...</code>") {
		t.Errorf("expected run_command in card")
	}
}
