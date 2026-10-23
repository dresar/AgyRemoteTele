package bot

import (
	"fmt"
	"html"
	"strings"

	"github.com/dresar/AgyRemoteTele/internal/agy"
	"github.com/dresar/AgyRemoteTele/internal/session"
	"github.com/dresar/AgyRemoteTele/internal/system"
	"github.com/dresar/AgyRemoteTele/internal/workspace"
)

type Renderer struct{}

func NewRenderer() *Renderer {
	return &Renderer{}
}

func (r *Renderer) ChunkMessage(text string, maxChunkLen int) []string {
	if maxChunkLen <= 0 {
		maxChunkLen = 4000
	}

	if len(text) <= maxChunkLen {
		return []string{text}
	}

	var chunks []string
	lines := strings.Split(text, "\n")
	var current strings.Builder

	for _, line := range lines {
		if current.Len()+len(line)+1 > maxChunkLen {
			if current.Len() > 0 {
				chunks = append(chunks, current.String())
				current.Reset()
			}

			for len(line) > maxChunkLen {
				chunks = append(chunks, line[:maxChunkLen])
				line = line[maxChunkLen:]
			}
		}

		if current.Len() > 0 {
			current.WriteString("\n")
		}
		current.WriteString(line)
	}

	if current.Len() > 0 {
		chunks = append(chunks, current.String())
	}

	total := len(chunks)
	if total > 1 {
		for i := range chunks {
			prefix := fmt.Sprintf("<b>[%d/%d]</b>\n", i+1, total)
			chunks[i] = prefix + chunks[i]
		}
	}

	return chunks
}

func (r *Renderer) ShouldSendAsDocument(text string) bool {
	return len(text) > 8000
}

func (r *Renderer) RenderWelcome(username string, defaultProfile string, sess *session.Session) string {
	var b strings.Builder
	b.WriteString("<b>⚡ AGY REMOTE TELEGRAM CONTROL PLANE</b>\n")
	b.WriteString("<i>Enterprise Orchestration for Google Antigravity</i>\n\n")

	profile := defaultProfile
	workDir := "/opt/data/home"
	statusBadge := "🟡 Unauthenticated"

	if sess != nil {
		if sess.ActiveProfile != "" {
			profile = sess.ActiveProfile
		}
		if sess.ActiveWorkingDir != "" {
			workDir = sess.ActiveWorkingDir
		}
		if sess.IsAuthenticated {
			statusBadge = "🟢 Authenticated"
		}
	}

	b.WriteString(fmt.Sprintf("<b>Status:</b> %s\n", statusBadge))
	b.WriteString(fmt.Sprintf("<b>Active Worker:</b> <code>%s</code>\n", html.EscapeString(profile)))
	b.WriteString(fmt.Sprintf("<b>Working Directory:</b> <code>%s</code>\n\n", html.EscapeString(workDir)))

	b.WriteString("<blockquote><b>Quick Access:</b>\n")
	b.WriteString("• <code>/ask &lt;prompt&gt;</code> - Dispatch agent task\n")
	b.WriteString("• <code>/profile [id]</code> - Switch worker profile (agy01-agy15)\n")
	b.WriteString("• <code>/status</code> - Real-time VPS telemetry\n")
	b.WriteString("• <code>/history</code> - Inspect & resume sessions\n")
	b.WriteString("• <code>/ls</code> & <code>/cat</code> - Workspace explorer\n")
	b.WriteString("• <code>/git</code> - Remote repository manager</blockquote>\n\n")

	b.WriteString("<i>Tap the buttons below to interact quickly:</i>")
	return b.String()
}

func (r *Renderer) RenderHelp() string {
	var b strings.Builder
	b.WriteString("<b>🛠 AGY REMOTE TELE — COMMAND REFERENCE</b>\n\n")

	b.WriteString("<b>⚡ Agent Operations:</b>\n")
	b.WriteString("• <code>/ask &lt;prompt&gt;</code> — Send prompt to Antigravity CLI\n")
	b.WriteString("• <code>/profile [id]</code> — View or switch active profile\n")
	b.WriteString("• <code>/history</code> — Browse past session transcripts\n")
	b.WriteString("• <code>/resume &lt;id&gt;</code> — Resume previous conversation\n")
	b.WriteString("• <code>/new</code> — Clear session context and start fresh\n\n")

	b.WriteString("<b>📊 System & Telemetry:</b>\n")
	b.WriteString("• <code>/status</code> — Live CPU, RAM, Disk & Goroutines\n")
	b.WriteString("• <code>/sh &lt;cmd&gt;</code> — Run safe shell command\n\n")

	b.WriteString("<b>📁 Filesystem & Git:</b>\n")
	b.WriteString("• <code>/ls [path]</code> — List workspace directory\n")
	b.WriteString("• <code>/cd &lt;path&gt;</code> — Change working directory\n")
	b.WriteString("• <code>/cat &lt;file&gt;</code> — Preview file contents\n")
	b.WriteString("• <code>/get &lt;file&gt;</code> — Download file as Telegram document\n")
	b.WriteString("• <code>/git [cmd]</code> — Check git status / branch / sync\n\n")

	b.WriteString("<b>🔐 Authentication:</b>\n")
	b.WriteString("• <code>/login &lt;key&gt;</code> — Pair device with admin secret\n")
	b.WriteString("• <code>/logout</code> — Terminate current authorized session\n")

	return b.String()
}

func (r *Renderer) RenderStatus(m *system.Metrics, activeProfile string) string {
	var b strings.Builder
	b.WriteString("<b>🟢 VPS TELEMETRY & SYSTEM HEALTH</b>\n\n")

	b.WriteString(fmt.Sprintf("<b>Worker Profile:</b> <code>%s</code>\n", html.EscapeString(activeProfile)))
	b.WriteString(fmt.Sprintf("<b>System Uptime:</b> <code>%s</code>\n", system.FormatUptime(m.SystemUptime)))
	b.WriteString(fmt.Sprintf("<b>Load Average:</b> <code>%.2f</code> (1m) | <code>%.2f</code> (5m) | <code>%.2f</code> (15m)\n\n",
		m.LoadAvg1, m.LoadAvg5, m.LoadAvg15))

	ramBadge := "🟢"
	if m.MemPercent > 85 {
		ramBadge = "🔴"
	} else if m.MemPercent > 70 {
		ramBadge = "🟡"
	}
	b.WriteString(fmt.Sprintf("<b>%s Memory (RAM):</b> <code>%.1f%%</code>\n", ramBadge, m.MemPercent))
	b.WriteString(fmt.Sprintf("  • Used: <code>%d MB</code> / <code>%d MB</code> (Free: <code>%d MB</code>)\n\n",
		m.MemUsedMB, m.MemTotalMB, m.MemFreeMB))

	diskBadge := "🟢"
	if m.DiskPercent > 90 {
		diskBadge = "🔴"
	} else if m.DiskPercent > 75 {
		diskBadge = "🟡"
	}
	b.WriteString(fmt.Sprintf("<b>%s Disk Storage:</b> <code>%.1f%%</code>\n", diskBadge, m.DiskPercent))
	b.WriteString(fmt.Sprintf("  • Used: <code>%.1f GB</code> / <code>%.1f GB</code> (Free: <code>%.1f GB</code>)\n\n",
		m.DiskUsedGB, m.DiskTotalGB, m.DiskFreeGB))

	b.WriteString("<b>⚡ Runtime Diagnostics:</b>\n")
	b.WriteString(fmt.Sprintf("  • Active Goroutines: <code>%d</code>\n", m.Goroutines))
	b.WriteString(fmt.Sprintf("  • Process Heap Alloc: <code>%.2f MB</code> (Sys: <code>%.2f MB</code>)\n",
		m.BotAllocMB, m.BotSysMB))

	return b.String()
}

func (r *Renderer) RenderHistory(profile string, items []agy.ConversationSummary) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("<b>📜 RECENT SESSIONS — [%s]</b>\n\n", html.EscapeString(profile)))

	if len(items) == 0 {
		b.WriteString("<i>No previous conversation transcripts found for this profile.</i>\n")
		b.WriteString("Use <code>/ask &lt;prompt&gt;</code> to start a new task.")
		return b.String()
	}

	for i, item := range items {
		b.WriteString(fmt.Sprintf("<b>#%d Session:</b> <code>%s</code>\n", i+1, item.ConversationID))
		b.WriteString(fmt.Sprintf("• <b>Updated:</b> <code>%s</code> | <b>Turns:</b> <code>%d</code>\n",
			item.LastUpdatedAt.Format("02 Jan 15:04"), item.TurnCount))
		b.WriteString(fmt.Sprintf("• <b>Initial Prompt:</b> <i>%s</i>\n", html.EscapeString(item.FirstPrompt)))
		b.WriteString(fmt.Sprintf("• <b>Resume:</b> /resume_%s\n\n", item.ConversationID))
	}

	b.WriteString("<i>Tip: Click /resume_&lt;id&gt; or use /new to clear.</i>")
	return b.String()
}

func (r *Renderer) RenderFileList(dir string, files []workspace.FileItem) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("<b>📁 WORKSPACE BROWSER:</b> <code>%s</code>\n\n", html.EscapeString(dir)))

	if len(files) == 0 {
		b.WriteString("<i>(Directory is empty)</i>\n")
		return b.String()
	}

	count := 0
	for _, f := range files {
		if count >= 30 {
			b.WriteString(fmt.Sprintf("\n<i>... and %d more entries</i>\n", len(files)-count))
			break
		}

		icon := "📄"
		if f.IsDir {
			icon = "📁"
			b.WriteString(fmt.Sprintf("%s <code>%s/</code>\n", icon, html.EscapeString(f.Name)))
		} else {
			sizeStr := workspace.FormatSize(f.Size)
			b.WriteString(fmt.Sprintf("%s <code>%s</code> (<code>%s</code>)\n", icon, html.EscapeString(f.Name), sizeStr))
		}
		count++
	}

	b.WriteString("\n<i>View file: <code>/cat &lt;file&gt;</code> | Download: <code>/get &lt;file&gt;</code></i>")
	return b.String()
}

func (r *Renderer) RenderGitSummary(g *workspace.GitSummary) string {
	var b strings.Builder
	b.WriteString("<b>🌿 GIT REPOSITORY STATUS</b>\n\n")

	if !g.IsRepository {
		b.WriteString("🔴 Current directory is not a Git repository.")
		return b.String()
	}

	b.WriteString(fmt.Sprintf("<b>Active Branch:</b> <code>%s</code>\n\n", html.EscapeString(g.Branch)))

	if g.Status != "" {
		b.WriteString("<b>Working Tree Status:</b>\n")
		b.WriteString("<pre><code>")
		b.WriteString(html.EscapeString(g.Status))
		b.WriteString("</code></pre>\n\n")
	} else {
		b.WriteString("🟢 <i>Working tree clean. No uncommitted changes.</i>\n\n")
	}

	if g.DiffStat != "" {
		b.WriteString("<b>Uncommitted Diff:</b>\n")
		b.WriteString("<pre><code>")
		b.WriteString(html.EscapeString(g.DiffStat))
		b.WriteString("</code></pre>\n\n")
	}

	if g.RecentCommits != "" {
		b.WriteString("<b>Recent Commits:</b>\n")
		b.WriteString("<pre><code>")
		b.WriteString(html.EscapeString(g.RecentCommits))
		b.WriteString("</code></pre>\n")
	}

	return strings.TrimSpace(b.String())
}

func (r *Renderer) RenderUnauthorized(chatID int64) string {
	return fmt.Sprintf("<b>🔴 ACCESS RESTRICTED</b>\n\n"+
		"Your Telegram Chat ID (<code>%d</code>) is not authorized to interact with this Antigravity Control Plane.\n\n"+
		"If you have an administrative pairing key, authenticate via:\n"+
		"<code>/login &lt;auth_key&gt;</code>", chatID)
}

func (r *Renderer) RenderProfileSwitched(oldProfile string, newProfile string) string {
	return fmt.Sprintf("<b>⚡ WORKER PROFILE SWITCHED</b>\n\n"+
		"• Previous: <code>%s</code>\n"+
		"• Active: 🟢 <code>%s</code>\n\n"+
		"All subsequent <code>/ask</code> instructions and transcripts will route through this profile.",
		html.EscapeString(oldProfile), html.EscapeString(newProfile))
}
