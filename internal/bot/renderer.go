package bot

import (
	"fmt"
	"html"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/dresar/AgyRemoteTele/internal/agy"
	"github.com/dresar/AgyRemoteTele/internal/backup"
	"github.com/dresar/AgyRemoteTele/internal/cluster"
	"github.com/dresar/AgyRemoteTele/internal/docker"
	"github.com/dresar/AgyRemoteTele/internal/nettools"
	"github.com/dresar/AgyRemoteTele/internal/scheduler"
	"github.com/dresar/AgyRemoteTele/internal/session"
	"github.com/dresar/AgyRemoteTele/internal/system"
	"github.com/dresar/AgyRemoteTele/internal/watchdog"
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
	sessionBadge := "🟡 Unpaired (Guest)"

	if sess != nil {
		if sess.ActiveProfile != "" {
			profile = sess.ActiveProfile
		}
		if sess.ActiveWorkingDir != "" {
			workDir = sess.ActiveWorkingDir
		}
		if sess.IsAuthenticated {
			sessionBadge = "🟢 Authorized Admin"
		}
	}

	b.WriteString(fmt.Sprintf("<b>Telegram Session:</b> %s\n", sessionBadge))
	b.WriteString(fmt.Sprintf("<b>Active Worker:</b> <code>%s</code>\n", html.EscapeString(profile)))
	b.WriteString(fmt.Sprintf("<b>Working Directory:</b> <code>%s</code>\n\n", html.EscapeString(workDir)))

	b.WriteString("<blockquote><b>Interactive Categories:</b>\n")
	b.WriteString("• 💻 <b>System:</b> <code>/status</code>, <code>/ps</code>, <code>/ports</code>, <code>/service</code>, <code>/alerts</code>\n")
	b.WriteString("• 🐳 <b>Docker:</b> <code>/docker ps</code>, <code>/docker stats</code>, <code>/docker logs</code>\n")
	b.WriteString("• 🌐 <b>Network:</b> <code>/ping</code>, <code>/curl</code>, <code>/dns</code>, <code>/netio</code>\n")
	b.WriteString("• ⏱ <b>Scheduler:</b> <code>/cron list</code>, <code>/cron add</code>, <code>/cron run</code>\n")
	b.WriteString("• 🐝 <b>Swarm:</b> <code>/swarm &lt;prompt&gt;</code>, <code>/workers</code>, <code>/tasks</code>\n")
	b.WriteString("• 📦 <b>Backup:</b> <code>/backup [path]</code> (auto-attaches archive &lt; 50MB)\n")
	b.WriteString("• 🤖 <b>Agent:</b> <code>/ask &lt;prompt&gt;</code>, <code>/profile</code>, <code>/history</code></blockquote>\n\n")

	b.WriteString("<i>Tap the buttons below to navigate:</i>")
	return b.String()
}

func (r *Renderer) RenderHelp() string {
	var b strings.Builder
	b.WriteString("<b>🛠 AGY REMOTE TELE — COMMAND REFERENCE</b>\n\n")

	b.WriteString("<b>💻 System & Diagnostics:</b>\n")
	b.WriteString("• <code>/status</code> — Real-time VPS telemetry (CPU, RAM, Disk)\n")
	b.WriteString("• <code>/ps [n] [sort]</code> — Top system processes (sort: cpu, mem, pid)\n")
	b.WriteString("• <code>/kill &lt;pid&gt; [sig]</code> — Terminate process with signal\n")
	b.WriteString("• <code>/ports</code> — Active listening network ports & sockets\n")
	b.WriteString("• <code>/service [list|status|restart] [name]</code> — Service manager\n")
	b.WriteString("• <code>/alerts</code> — Watchdog alerts history and health checks\n\n")

	b.WriteString("<b>🐳 Docker Management:</b>\n")
	b.WriteString("• <code>/docker ps</code> — List all Docker containers\n")
	b.WriteString("• <code>/docker stats</code> — Live container CPU, RAM & I/O stats\n")
	b.WriteString("• <code>/docker logs &lt;id&gt; [lines]</code> — View container logs\n")
	b.WriteString("• <code>/docker restart &lt;id&gt;</code> — Restart Docker container\n\n")

	b.WriteString("<b>🌐 Network Tools:</b>\n")
	b.WriteString("• <code>/ping &lt;host&gt;</code> — ICMP ping diagnostic\n")
	b.WriteString("• <code>/curl &lt;url&gt;</code> or <code>/http &lt;url&gt;</code> — Probe HTTP/HTTPS endpoint\n")
	b.WriteString("• <code>/dns &lt;domain&gt; [type]</code> — DNS record lookup (A, AAAA, MX, TXT)\n")
	b.WriteString("• <code>/netio</code> — Interface bandwidth & packet traffic\n\n")

	b.WriteString("<b>⏱ Task Scheduler:</b>\n")
	b.WriteString("• <code>/cron list</code> — View all scheduled jobs\n")
	b.WriteString("• <code>/cron add @every &lt;interval&gt; &lt;cmd&gt;</code> — Schedule interval job\n")
	b.WriteString("• <code>/cron add &lt;cron_expr&gt; &lt;cmd&gt;</code> — Schedule 5-field cron\n")
	b.WriteString("• <code>/cron run &lt;job_id&gt;</code> — Trigger job immediately\n")
	b.WriteString("• <code>/cron remove &lt;job_id&gt;</code> — Delete scheduled job\n\n")

	b.WriteString("<b>🐝 Cluster Swarm & Workers:</b>\n")
	b.WriteString("• <code>/swarm &lt;prompt&gt;</code> — Broadcast prompt across all workers\n")
	b.WriteString("• <code>/workers</code> — Daemon status of cluster workers\n")
	b.WriteString("• <code>/tasks</code> — View active & completed worker tasks\n")
	b.WriteString("• <code>/usage</code> — Cluster token usage & task completion\n")
	b.WriteString("• <code>/send &lt;account&gt; &lt;prompt&gt;</code> — Dispatch task to specific worker\n")
	b.WriteString("• <code>/accounts</code> & <code>/refresh</code> — Worker account management\n")
	b.WriteString("• <code>/up &lt;acc|semua&gt;</code>, <code>/hidupkan &lt;acc|semua&gt;</code> — Hidupkan worker / refresh token\n\n")

	b.WriteString("<b>📦 Backup & Archival:</b>\n")
	b.WriteString("• <code>/backup [dir]</code> — Create tar.gz archive (sends doc if &lt; 50MB)\n\n")

	b.WriteString("<b>📁 Workspace & Git:</b>\n")
	b.WriteString("• <code>/ls [path]</code>, <code>/cd &lt;path&gt;</code>, <code>/cat &lt;file&gt;</code>, <code>/get &lt;file&gt;</code>\n")
	b.WriteString("• <code>/git [cmd]</code>, <code>/sh &lt;cmd&gt;</code>\n\n")

	b.WriteString("<b>🤖 Local Agent & Auth:</b>\n")
	b.WriteString("• <code>/ask &lt;prompt&gt;</code>, <code>/plan &lt;task&gt;</code>, <code>/boost [learn]</code>, <code>/teamwork &lt;task&gt;</code>\n")
	b.WriteString("• <code>/tools</code>, <code>/continue</code>, <code>/new [prompt]</code>, <code>/profile [id]</code>, <code>/history</code>, <code>/resume &lt;id&gt;</code>\n")
	b.WriteString("• <code>/kill</code> (terminate active AGY task or process PID)\n")
	b.WriteString("• <code>/login &lt;key&gt;</code>, <code>/logout</code>")

	return b.String()
}

func (r *Renderer) RenderStatus(m *system.Metrics, activeProfile string) string {
	var b strings.Builder
	b.WriteString("<b>🟢 VPS TELEMETRY & SYSTEM HEALTH</b>\n\n")

	b.WriteString(fmt.Sprintf("<b>Worker Profile:</b> <code>%s</code>\n", html.EscapeString(activeProfile)))
	b.WriteString(fmt.Sprintf("<b>Allocated CPU:</b> <code>%.0f vCPU</code>\n", m.AllocatedVCPU))
	b.WriteString(fmt.Sprintf("<b>System Uptime:</b> <code>%s</code>\n", system.FormatUptime(m.SystemUptime)))
	b.WriteString(fmt.Sprintf("<b>Load Average:</b> <code>%.2f</code> (1m) | <code>%.2f</code> (5m) | <code>%.2f</code> (15m)\n\n",
		m.LoadAvg1, m.LoadAvg5, m.LoadAvg15))

	ramBadge := "🟢"
	if m.MemPercent > 85 {
		ramBadge = "🔴"
	} else if m.MemPercent > 70 {
		ramBadge = "🟡"
	}
	b.WriteString(fmt.Sprintf("<b>%s Memory (RAM Limit 4 GB):</b> <code>%.1f%%</code>\n", ramBadge, m.MemPercent))
	b.WriteString(fmt.Sprintf("  • Used: <code>%d MB</code> / <code>%d MB</code> (Free: <code>%d MB</code>)\n\n",
		m.MemUsedMB, m.MemTotalMB, m.MemFreeMB))

	diskBadge := "🟢"
	if m.DiskFreeGB < 5.0 {
		diskBadge = "🔴"
	} else if m.DiskFreeGB < 15.0 {
		diskBadge = "🟡"
	}
	b.WriteString(fmt.Sprintf("<b>%s Storage Availability:</b>\n", diskBadge))
	b.WriteString(fmt.Sprintf("  • Free Space Available: <code>%.1f GB</code>\n", m.DiskFreeGB))
	b.WriteString(fmt.Sprintf("  • Partition Capacity: <code>%.1f GB</code>\n\n", m.DiskTotalGB))

	b.WriteString("<b>⚡ Bot Memory Footprint (Super Ringan):</b>\n")
	b.WriteString(fmt.Sprintf("  • Process Physical RSS: <code>%.1f MB</code>\n", m.BotRSSMB))
	b.WriteString(fmt.Sprintf("  • Heap In-Use: <code>%.2f MB</code> (Alloc: <code>%.2f MB</code> | Sys: <code>%.2f MB</code>)\n",
		m.HeapInUseMB, m.BotAllocMB, m.BotSysMB))
	b.WriteString(fmt.Sprintf("  • Stack In-Use: <code>%.2f MB</code>\n", m.StackInUseMB))
	b.WriteString(fmt.Sprintf("  • Active Goroutines: <code>%d</code> | GC Cycles: <code>%d</code>\n", m.Goroutines, m.NumGC))

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

func (r *Renderer) RenderProcesses(procs []system.ProcessInfo) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("<b>⚙️ SYSTEM PROCESSES (TOP %d)</b>\n\n", len(procs)))

	if len(procs) == 0 {
		b.WriteString("<i>No processes found.</i>")
		return b.String()
	}

	for _, p := range procs {
		cmd := p.Command
		if len(cmd) > 60 {
			cmd = cmd[:57] + "..."
		}
		b.WriteString(fmt.Sprintf("• <b>PID %d</b> [<code>%s</code>] — <code>%.1f%% CPU</code> | <code>%.1f MB RSS</code> (<code>%.1f%% RAM</code>)\n  User: <code>%s</code> | State: <code>%s</code>\n  <code>%s</code>\n\n",
			p.PID, html.EscapeString(p.State), p.CPU, p.RSSMB, p.Mem, html.EscapeString(p.User), html.EscapeString(p.State), html.EscapeString(cmd)))
	}

	b.WriteString("<i>Tip: Use <code>/kill &lt;pid&gt; [sig]</code> to terminate a process.</i>")
	return strings.TrimSpace(b.String())
}

func (r *Renderer) RenderListeningPorts(ports []system.PortInfo) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("<b>🔌 LISTENING PORTS & SOCKETS (%d)</b>\n\n", len(ports)))

	if len(ports) == 0 {
		b.WriteString("<i>No active listening ports detected.</i>")
		return b.String()
	}

	count := 0
	for _, p := range ports {
		if count >= 35 {
			b.WriteString(fmt.Sprintf("\n<i>... and %d more ports</i>\n", len(ports)-count))
			break
		}
		procInfo := ""
		if p.ProcessName != "" || p.PID > 0 {
			procInfo = fmt.Sprintf(" (<code>%s</code> - PID: <code>%d</code>)", html.EscapeString(p.ProcessName), p.PID)
		}
		b.WriteString(fmt.Sprintf("• <b>%s</b> <code>%s:%d</code> [%s]%s\n",
			strings.ToUpper(p.Protocol), html.EscapeString(p.LocalIP), p.LocalPort, html.EscapeString(p.State), procInfo))
		count++
	}

	return strings.TrimSpace(b.String())
}

func (r *Renderer) RenderServices(services []system.ServiceInfo) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("<b>🛡 SYSTEM SERVICES STATUS (%d)</b>\n\n", len(services)))

	if len(services) == 0 {
		b.WriteString("<i>No common services detected.</i>")
		return b.String()
	}

	for _, s := range services {
		badge := "🟢"
		if !s.Active {
			badge = "🔴"
		}
		desc := ""
		if s.Description != "" {
			desc = fmt.Sprintf("\n  <i>%s</i>", html.EscapeString(s.Description))
		}
		b.WriteString(fmt.Sprintf("%s <b>%s</b> — <code>%s</code> (<code>%s</code>)%s\n",
			badge, html.EscapeString(s.Name), html.EscapeString(s.Status), html.EscapeString(s.SubState), desc))
	}

	b.WriteString("\n<i>Restart service: <code>/service restart &lt;name&gt;</code></i>")
	return strings.TrimSpace(b.String())
}

func (r *Renderer) RenderPing(res *nettools.PingResult) string {
	if res == nil {
		return "<i>No ping result available.</i>"
	}

	var b strings.Builder
	badge := "🟢"
	if res.PacketLoss >= 50.0 {
		badge = "🔴"
	} else if res.PacketLoss > 0 {
		badge = "🟡"
	}

	b.WriteString(fmt.Sprintf("<b>%s PING DIAGNOSTIC — %s</b>\n\n", badge, html.EscapeString(res.Host)))
	b.WriteString(fmt.Sprintf("• <b>Transmitted:</b> <code>%d packets</code>\n", res.PacketsSent))
	b.WriteString(fmt.Sprintf("• <b>Received:</b> <code>%d packets</code>\n", res.PacketsRecv))
	b.WriteString(fmt.Sprintf("• <b>Packet Loss:</b> <code>%.1f%%</code>\n\n", res.PacketLoss))

	b.WriteString("<b>Round-Trip Latency:</b>\n")
	b.WriteString(fmt.Sprintf("  ↳ Min: <code>%s</code>\n", formatDuration(res.MinRTT)))
	b.WriteString(fmt.Sprintf("  ↳ Avg: <code>%s</code>\n", formatDuration(res.AvgRTT)))
	b.WriteString(fmt.Sprintf("  ↳ Max: <code>%s</code>", formatDuration(res.MaxRTT)))

	return b.String()
}

func (r *Renderer) RenderHTTPProbe(res *nettools.HTTPProbeResult) string {
	if res == nil {
		return "<i>No probe result available.</i>"
	}

	var b strings.Builder
	badge := "🟢"
	if res.StatusCode >= 400 || res.StatusCode == 0 {
		badge = "🔴"
	} else if res.StatusCode >= 300 {
		badge = "🟡"
	}

	b.WriteString("<b>⚡ HTTP/HTTPS ENDPOINT PROBE</b>\n\n")
	b.WriteString(fmt.Sprintf("• <b>URL:</b> <code>%s</code>\n", html.EscapeString(res.URL)))
	b.WriteString(fmt.Sprintf("• <b>Status:</b> %s <code>%d %s</code> (<code>%s</code>)\n",
		badge, res.StatusCode, html.EscapeString(res.StatusText), html.EscapeString(res.Proto)))
	b.WriteString(fmt.Sprintf("• <b>Timing:</b> TTFB: <code>%s</code> | Total: <code>%s</code>\n",
		formatDuration(res.TTFB), formatDuration(res.TotalDuration)))

	contentLenStr := formatBytes(uint64(res.ContentLength))
	if res.ContentLength < 0 {
		contentLenStr = "chunked/unknown"
	}
	b.WriteString(fmt.Sprintf("• <b>Content:</b> <code>%s</code> (<code>%s</code>)\n",
		html.EscapeString(res.ContentType), contentLenStr))

	if res.TLSIssuer != "" {
		b.WriteString(fmt.Sprintf("• <b>TLS Issuer:</b> <code>%s</code>\n", html.EscapeString(res.TLSIssuer)))
	}
	if res.TLSExpiry != nil {
		b.WriteString(fmt.Sprintf("• <b>TLS Expiry:</b> <code>%s</code>\n", res.TLSExpiry.Format("2006-01-02 15:04:05 MST")))
	}

	return strings.TrimSpace(b.String())
}

func (r *Renderer) RenderDNS(host string, recs []nettools.DNSRecord) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("<b>🔍 DNS LOOKUP — %s</b>\n\n", html.EscapeString(host)))

	if len(recs) == 0 {
		b.WriteString(fmt.Sprintf("<i>No DNS records found for %s.</i>", html.EscapeString(host)))
		return b.String()
	}

	for _, rec := range recs {
		b.WriteString(fmt.Sprintf("<b>[%s Records]</b>\n", html.EscapeString(rec.Type)))
		if len(rec.Values) == 0 {
			b.WriteString("  <i>(none)</i>\n")
		}
		for _, val := range rec.Values {
			b.WriteString(fmt.Sprintf("  • <code>%s</code>\n", html.EscapeString(val)))
		}
		b.WriteString("\n")
	}

	return strings.TrimSpace(b.String())
}

func (r *Renderer) RenderNetworkIO(stats []nettools.InterfaceStats) string {
	var b strings.Builder
	b.WriteString("<b>📊 NETWORK INTERFACE TRAFFIC (NET I/O)</b>\n\n")

	if len(stats) == 0 {
		b.WriteString("<i>No network interfaces detected.</i>")
		return b.String()
	}

	for _, s := range stats {
		b.WriteString(fmt.Sprintf("• <b>%s:</b>\n", html.EscapeString(s.Name)))
		b.WriteString(fmt.Sprintf("  ↳ RX: <code>%s</code> (<code>%d pkts</code>, <code>%d errs</code>)\n",
			formatBytes(s.RXBytes), s.RXPackets, s.RXErrors))
		b.WriteString(fmt.Sprintf("  ↳ TX: <code>%s</code> (<code>%d pkts</code>, <code>%d errs</code>)\n\n",
			formatBytes(s.TXBytes), s.TXPackets, s.TXErrors))
	}

	return strings.TrimSpace(b.String())
}

func (r *Renderer) RenderContainers(containers []docker.ContainerInfo) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("<b>🐳 DOCKER CONTAINERS (%d)</b>\n\n", len(containers)))

	if len(containers) == 0 {
		b.WriteString("<i>No containers found or Docker daemon inactive.</i>")
		return b.String()
	}

	for _, c := range containers {
		badge := "🟢"
		if strings.ToLower(c.State) != "running" {
			badge = "⚪"
		}
		cid := c.ID
		if len(cid) > 12 {
			cid = cid[:12]
		}
		b.WriteString(fmt.Sprintf("%s <b>%s</b> (<code>%s</code>)\n", badge, html.EscapeString(c.Names), html.EscapeString(cid)))
		b.WriteString(fmt.Sprintf("  • Image: <code>%s</code>\n", html.EscapeString(c.Image)))
		b.WriteString(fmt.Sprintf("  • Status: <i>%s</i>\n", html.EscapeString(c.Status)))
		if c.Ports != "" {
			b.WriteString(fmt.Sprintf("  • Ports: <code>%s</code>\n", html.EscapeString(c.Ports)))
		}
		b.WriteString("\n")
	}

	b.WriteString("<i>Actions: <code>/docker logs &lt;id&gt;</code> | <code>/docker restart &lt;id&gt;</code></i>")
	return strings.TrimSpace(b.String())
}

func (r *Renderer) RenderContainerStats(stats []docker.ContainerStats) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("<b>📈 DOCKER LIVE RESOURCE STATS (%d)</b>\n\n", len(stats)))

	if len(stats) == 0 {
		b.WriteString("<i>No container resource statistics available.</i>")
		return b.String()
	}

	for _, s := range stats {
		cid := s.ID
		if len(cid) > 12 {
			cid = cid[:12]
		}
		b.WriteString(fmt.Sprintf("• <b>%s</b> (<code>%s</code>):\n", html.EscapeString(s.Name), html.EscapeString(cid)))
		b.WriteString(fmt.Sprintf("  ↳ CPU: <code>%s</code> | RAM: <code>%s</code> (<code>%s</code>)\n",
			html.EscapeString(s.CPUPerc), html.EscapeString(s.MemUsage), html.EscapeString(s.MemPerc)))
		b.WriteString(fmt.Sprintf("  ↳ Net I/O: <code>%s</code> | Block I/O: <code>%s</code>\n\n",
			html.EscapeString(s.NetIO), html.EscapeString(s.BlockIO)))
	}

	return strings.TrimSpace(b.String())
}

func (r *Renderer) RenderCronJobs(jobs []*scheduler.CronJob) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("<b>⏱ SCHEDULED CRON JOBS (%d)</b>\n\n", len(jobs)))

	if len(jobs) == 0 {
		b.WriteString("<i>No scheduled cron jobs configured.</i>\n\n")
		b.WriteString("<i>Tip: Use <code>/cron add &lt;spec&gt; &lt;cmd&gt;</code> to create one.</i>")
		return b.String()
	}

	for _, j := range jobs {
		badge := "🟢"
		if !j.Enabled {
			badge = "⚪"
		}
		b.WriteString(fmt.Sprintf("%s <b>Job ID:</b> <code>%s</code>\n", badge, html.EscapeString(j.ID)))
		b.WriteString(fmt.Sprintf("  • Spec: <code>%s</code>\n", html.EscapeString(j.Spec)))
		b.WriteString(fmt.Sprintf("  • Command: <code>%s</code>\n", html.EscapeString(j.Command)))
		b.WriteString(fmt.Sprintf("  • Next Run: <code>%s</code>\n", j.NextRun.Format("2006-01-02 15:04:05")))
		b.WriteString(fmt.Sprintf("  • Run Count: <code>%d</code>\n", j.RunCount))
		if j.LastRun != nil {
			b.WriteString(fmt.Sprintf("  • Last Run: <code>%s</code>\n", j.LastRun.Format("2006-01-02 15:04:05")))
		}
		if j.LastResult != "" {
			resSnippet := j.LastResult
			if len(resSnippet) > 50 {
				resSnippet = resSnippet[:47] + "..."
			}
			b.WriteString(fmt.Sprintf("  • Last Result: <i>%s</i>\n", html.EscapeString(resSnippet)))
		}
		b.WriteString(fmt.Sprintf("  • Actions: /cron_run_%s | /cron_rm_%s\n\n", j.ID, j.ID))
	}

	return strings.TrimSpace(b.String())
}

func (r *Renderer) RenderAlerts(alerts []watchdog.AlertEvent) string {
	var b strings.Builder
	b.WriteString("<b>🚨 SYSTEM WATCHDOG & ALERTS</b>\n\n")

	if len(alerts) == 0 {
		b.WriteString("🟢 <i>All systems nominal. No watchdog alerts recorded.</i>")
		return b.String()
	}

	for _, a := range alerts {
		badge := "🟡"
		switch strings.ToUpper(a.Level) {
		case "CRITICAL":
			badge = "🔴"
		case "WARNING":
			badge = "🟡"
		case "RESOLVED":
			badge = "🟢"
		case "INFO":
			badge = "ℹ️"
		}
		b.WriteString(fmt.Sprintf("%s <b>[%s] %s</b> (<code>%s</code>)\n",
			badge, html.EscapeString(a.Level), html.EscapeString(a.Metric), html.EscapeString(a.Timestamp.Format("15:04:05"))))
		b.WriteString(fmt.Sprintf("  %s\n\n", html.EscapeString(a.Message)))
	}

	return strings.TrimSpace(b.String())
}

func (r *Renderer) RenderBackup(res *backup.BackupResult) string {
	if res == nil {
		return "<i>No backup result available.</i>"
	}

	var b strings.Builder
	b.WriteString("<b>📦 BACKUP ARCHIVE CREATED</b>\n\n")
	b.WriteString(fmt.Sprintf("• <b>Archive:</b> <code>%s</code>\n", html.EscapeString(res.ArchivePath)))
	b.WriteString(fmt.Sprintf("• <b>Files Included:</b> <code>%d files</code>\n", res.FileCount))
	b.WriteString(fmt.Sprintf("• <b>Total Size:</b> <code>%s</code>\n", formatBytes(uint64(res.TotalSizeBytes))))
	b.WriteString(fmt.Sprintf("• <b>Duration:</b> <code>%s</code>\n\n", formatDuration(res.Duration)))
	b.WriteString("🟢 <i>Backup successfully stored on VPS filesystem.</i>")

	return b.String()
}

func (r *Renderer) RenderSwarm(results []cluster.SwarmTaskResult) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("<b>🐝 CLUSTER SWARM BROADCAST RESULTS (%d)</b>\n\n", len(results)))

	if len(results) == 0 {
		b.WriteString("<i>No swarm task results returned.</i>")
		return b.String()
	}

	for _, res := range results {
		badge := "🟢"
		if res.Error != nil {
			badge = "🔴"
		}
		b.WriteString(fmt.Sprintf("%s <b>Worker:</b> <code>%s</code>\n", badge, html.EscapeString(res.Account)))
		if res.TaskID != "" {
			b.WriteString(fmt.Sprintf("  • Task ID: <code>%s</code>\n", html.EscapeString(res.TaskID)))
		}
		b.WriteString(fmt.Sprintf("  • Duration: <code>%s</code>\n", formatDuration(res.Duration)))
		if res.Error != nil {
			b.WriteString(fmt.Sprintf("  • Error: <i>%s</i>\n\n", html.EscapeString(res.Error.Error())))
		} else {
			snippet := strings.TrimSpace(res.Output)
			if len(snippet) > 200 {
				snippet = snippet[:197] + "..."
			}
			if snippet != "" {
				b.WriteString(fmt.Sprintf("  • Output: <pre><code>%s</code></pre>\n\n", html.EscapeString(snippet)))
			} else {
				b.WriteString("  • Output: <i>(empty output)</i>\n\n")
			}
		}
	}

	return strings.TrimSpace(b.String())
}

func (r *Renderer) RenderTools() string {
	var b strings.Builder
	b.WriteString("<b>🛠 ANTIGRAVITY TOOLS & AUTONOMOUS ENGINE</b>\n\n")

	b.WriteString("<b>⚡ Native Execution Tools:</b>\n")
	b.WriteString("• <code>run_command</code> — Eksekusi command bash & terminal di VPS\n")
	b.WriteString("• <code>read_file</code> / <code>view_file</code> — Membaca file dan konten kode\n")
	b.WriteString("• <code>write_to_file</code> / <code>replace_file_content</code> — Menulis dan mengedit file kode\n")
	b.WriteString("• <code>find_by_name</code> — Pencarian file berbasis pola nama (glob)\n")
	b.WriteString("• <code>grep_search</code> — Pencarian regex cepat dengan ripgrep\n")
	b.WriteString("• <code>list_dir</code> — Navigasi dan inspeksi struktur direktori\n")
	b.WriteString("• <code>read_url_content</code> / <code>search_web</code> — Web fetch & pencarian informasi\n")
	b.WriteString("• <code>manage_task</code> / <code>schedule</code> — Manajemen background task & timer\n")
	b.WriteString("• <code>send_message</code> — Komunikasi antar-worker cluster\n\n")

	b.WriteString("<b>🛡 Anti-Detection & Autonomous Engine:</b>\n")
	b.WriteString("• <code>--dangerously-skip-permissions</code> — Bypass konfirmasi izin tools otomatis\n")
	b.WriteString("• <code>Autonomous Operator Directive</code> — Otonomi penuh tanpa meminta izin berulang\n")
	b.WriteString("• <code>Clean Output Stream</code> — Log CLI disupresi, hanya kirim jawaban bersih\n\n")

	b.WriteString("<b>🧩 Specialized Cluster Skills:</b>\n")
	b.WriteString("• <code>agy-code-review</code>, <code>agy-error-recovery</code>, <code>agy-git-worktree-workflow</code>\n")
	b.WriteString("• <code>agy-incident-recovery</code>, <code>agy-merge-conflict-resolution</code>, <code>agy-operating-protocol</code>\n")
	b.WriteString("• <code>agy-regression-risk-analysis</code>, <code>agy-resource-aware-execution</code>, <code>agy-result-verification</code>\n")
	b.WriteString("• <code>agy-security-baseline</code>, <code>agy-task-boundaries</code>, <code>antislop</code>\n\n")

	b.WriteString("<i>Semua tools terpasang dan otomatis dipahami oleh Antigravity untuk menyelesaikan setiap instruksi.</i>")
	return b.String()
}

func (r *Renderer) GetStartKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("💻 System", "nav:cat_system"),
			tgbotapi.NewInlineKeyboardButtonData("🐳 Docker", "nav:cat_docker"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🌐 Net", "nav:cat_net"),
			tgbotapi.NewInlineKeyboardButtonData("⏱ Scheduler", "nav:cat_scheduler"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🐝 Swarm", "nav:cat_swarm"),
			tgbotapi.NewInlineKeyboardButtonData("📦 Backup", "nav:cat_backup"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🤖 Workers", "nav:workers"),
			tgbotapi.NewInlineKeyboardButtonData("📊 Usage", "nav:usage"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🛠 Tools AGY", "nav:tools"),
			tgbotapi.NewInlineKeyboardButtonData("📁 Workspace", "nav:ls"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❓ Help", "nav:help"),
		),
	)
}

func (r *Renderer) GetBackKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("« Menu Utama", "nav:start"),
		),
	)
}

func (r *Renderer) GetCategoryKeyboard(cat string) tgbotapi.InlineKeyboardMarkup {
	switch cat {
	case "system":
		return tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("⚡ Telemetry", "nav:status"),
				tgbotapi.NewInlineKeyboardButtonData("⚙️ Top Procs", "nav:ps"),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("🔌 Ports", "nav:ports"),
				tgbotapi.NewInlineKeyboardButtonData("🛡 Services", "nav:services"),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("🚨 Alerts", "nav:alerts"),
				tgbotapi.NewInlineKeyboardButtonData("« Menu Utama", "nav:start"),
			),
		)
	case "docker":
		return tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("📋 Containers", "nav:docker_ps"),
				tgbotapi.NewInlineKeyboardButtonData("📈 Live Stats", "nav:docker_stats"),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("« Menu Utama", "nav:start"),
			),
		)
	case "net":
		return tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("📊 Net I/O", "nav:netio"),
				tgbotapi.NewInlineKeyboardButtonData("🌐 Ping (1.1.1.1)", "nav:ping_default"),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("« Menu Utama", "nav:start"),
			),
		)
	case "scheduler":
		return tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("📋 Active Jobs", "nav:cron_list"),
				tgbotapi.NewInlineKeyboardButtonData("« Menu Utama", "nav:start"),
			),
		)
	case "swarm":
		return tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("🤖 Workers Status", "nav:workers"),
				tgbotapi.NewInlineKeyboardButtonData("📊 Cluster Usage", "nav:usage"),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("📋 Tasks", "nav:tasks"),
				tgbotapi.NewInlineKeyboardButtonData("« Menu Utama", "nav:start"),
			),
		)
	case "backup":
		return tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("📦 Backup Workspace", "nav:backup_now"),
				tgbotapi.NewInlineKeyboardButtonData("« Menu Utama", "nav:start"),
			),
		)
	default:
		return r.GetBackKeyboard()
	}
}

func formatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func formatDuration(d time.Duration) string {
	if d < time.Millisecond {
		return fmt.Sprintf("%d µs", d.Microseconds())
	}
	if d < time.Second {
		return fmt.Sprintf("%.2f ms", float64(d.Microseconds())/1000.0)
	}
	return fmt.Sprintf("%.2f s", d.Seconds())
}
