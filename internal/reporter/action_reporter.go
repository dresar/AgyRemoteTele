package reporter

import (
	"fmt"
	"html"
	"strings"

	"github.com/dresar/AgyRemoteTele/internal/agy"
	"github.com/dresar/AgyRemoteTele/internal/workspace"
)

type ToolSummary struct {
	Name   string
	Detail string
	Status string
}

type ActionReport struct {
	Profile          string
	ConversationID   string
	DurationSeconds  float64
	ExecutiveSummary string
	ToolsInvoked     []ToolSummary
	FileChanges      string
}

func BuildReport(profile string, result *agy.StreamResult, workingDir string) *ActionReport {
	execSummary := extractExecutiveSummary(result.Response)

	var tools []ToolSummary
	seen := make(map[string]bool)

	for _, t := range result.ToolsInvoked {
		detail := extractToolDetail(t)
		key := fmt.Sprintf("%s:%s", t.Name, detail)
		if seen[key] {
			continue
		}
		seen[key] = true

		status := "DONE"
		if strings.Contains(strings.ToLower(t.Output), "failed") || strings.Contains(strings.ToLower(t.Output), "error") {
			status = "FAIL"
		} else if strings.Contains(strings.ToLower(t.Output), "pass") || strings.Contains(t.Output, "exited with code 0") {
			status = "PASS"
		}

		tools = append(tools, ToolSummary{
			Name:   t.Name,
			Detail: detail,
			Status: status,
		})
	}

	diff := workspace.GetDiffSummary(workingDir)

	return &ActionReport{
		Profile:          profile,
		ConversationID:   result.ConversationID,
		DurationSeconds:  result.DurationSeconds,
		ExecutiveSummary: execSummary,
		ToolsInvoked:     tools,
		FileChanges:      diff,
	}
}

func (r *ActionReport) RenderHTML() string {
	var b strings.Builder

	b.WriteString("<b>⚡ TASK EXECUTION COMPLETED</b>\n")
	b.WriteString(fmt.Sprintf("<b>Profile:</b> <code>%s</code> | <b>Duration:</b> <code>%.1fs</code>\n\n",
		html.EscapeString(r.Profile), r.DurationSeconds))

	if r.ExecutiveSummary != "" {
		b.WriteString("<blockquote><b>Executive Summary:</b>\n")
		b.WriteString(html.EscapeString(r.ExecutiveSummary))
		b.WriteString("</blockquote>\n\n")
	}

	if len(r.ToolsInvoked) > 0 {
		b.WriteString("<b>🛠 Tools Invoked:</b>\n")
		for _, t := range r.ToolsInvoked {
			b.WriteString(fmt.Sprintf("• <code>%s</code>: <code>%s</code>",
				html.EscapeString(t.Name), html.EscapeString(t.Detail)))
			if t.Status != "" && t.Status != "DONE" {
				b.WriteString(fmt.Sprintf(" (<b>%s</b>)", html.EscapeString(t.Status)))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	if r.FileChanges != "" && r.FileChanges != "No changes detected" {
		b.WriteString("<b>📁 File Changes:</b>\n")
		b.WriteString("<pre><code>")
		b.WriteString(html.EscapeString(r.FileChanges))
		b.WriteString("</code></pre>\n\n")
	}

	if r.ConversationID != "" {
		b.WriteString(fmt.Sprintf("<i>Session ID:</i> <code>%s</code>", html.EscapeString(r.ConversationID)))
	}

	return strings.TrimSpace(b.String())
}

func extractExecutiveSummary(resp string) string {
	trimmed := strings.TrimSpace(resp)
	if trimmed == "" {
		return "Execution finished with no text output."
	}

	paragraphs := strings.Split(trimmed, "\n\n")
	first := strings.TrimSpace(paragraphs[0])
	if len(first) > 300 {
		return first[:297] + "..."
	}
	return first
}

func extractToolDetail(t agy.ToolInvocation) string {
	if t.Parameters == nil {
		return ""
	}

	if p, ok := t.Parameters["CommandLine"]; ok {
		return fmt.Sprintf("%v", p)
	}
	if p, ok := t.Parameters["Path"]; ok {
		return fmt.Sprintf("%v", p)
	}
	if p, ok := t.Parameters["AbsolutePath"]; ok {
		return fmt.Sprintf("%v", p)
	}
	if p, ok := t.Parameters["Target"]; ok {
		return fmt.Sprintf("%v", p)
	}
	if p, ok := t.Parameters["query"]; ok {
		return fmt.Sprintf("%v", p)
	}

	return ""
}
