package bot

import (
	"strings"
	"testing"
)

func TestRendererChunking(t *testing.T) {
	r := NewRenderer()

	shortMsg := "This is a short message."
	chunks := r.ChunkMessage(shortMsg, 100)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}

	lines := make([]string, 50)
	for i := range lines {
		lines[i] = "Line of sample message output that has some length."
	}
	longMsg := strings.Join(lines, "\n")

	chunks = r.ChunkMessage(longMsg, 200)
	if len(chunks) <= 1 {
		t.Errorf("expected multiple chunks, got %d", len(chunks))
	}
	if !strings.HasPrefix(chunks[0], "<b>[1/") {
		t.Errorf("expected pagination indicator in first chunk, got %s", chunks[0])
	}
}

func TestRendererHelp(t *testing.T) {
	r := NewRenderer()
	help := r.RenderHelp()
	if !strings.Contains(help, "/ask") || !strings.Contains(help, "/profile") {
		t.Errorf("expected help card to include command definitions")
	}
}
