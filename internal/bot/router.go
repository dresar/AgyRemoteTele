package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/dresar/AgyRemoteTele/internal/agy"
	"github.com/dresar/AgyRemoteTele/internal/backup"
	"github.com/dresar/AgyRemoteTele/internal/cluster"
	"github.com/dresar/AgyRemoteTele/internal/docker"
	"github.com/dresar/AgyRemoteTele/internal/nettools"
	"github.com/dresar/AgyRemoteTele/internal/reporter"
	"github.com/dresar/AgyRemoteTele/internal/scheduler"
	"github.com/dresar/AgyRemoteTele/internal/session"
	"github.com/dresar/AgyRemoteTele/internal/system"
	"github.com/dresar/AgyRemoteTele/internal/watchdog"
	"github.com/dresar/AgyRemoteTele/internal/workspace"
)

type Router struct {
	api             *tgbotapi.BotAPI
	authManager     *session.AuthManager
	sessionStore    *session.Store
	profileManager  *agy.ProfileManager
	historyReader   *agy.HistoryReader
	executor        *agy.Executor
	renderer        *Renderer
	middleware      *Middleware
	execTimeout     time.Duration
	scheduler       *scheduler.Scheduler
	watchdog        *watchdog.Watchdog
	swarmManager    *cluster.SwarmManager
	fastLoginScript string
}

func NewRouter(
	api *tgbotapi.BotAPI,
	authManager *session.AuthManager,
	sessionStore *session.Store,
	profileManager *agy.ProfileManager,
	historyReader *agy.HistoryReader,
	executor *agy.Executor,
	renderer *Renderer,
	middleware *Middleware,
	execTimeout time.Duration,
) *Router {
	return &Router{
		api:             api,
		authManager:     authManager,
		sessionStore:    sessionStore,
		profileManager:  profileManager,
		historyReader:   historyReader,
		executor:        executor,
		renderer:        renderer,
		middleware:      middleware,
		execTimeout:     execTimeout,
		fastLoginScript: "/opt/data/home/agy-profiles/fast_login.py",
	}
}

func (r *Router) SetFastLoginScript(path string) {
	r.fastLoginScript = path
}

func (r *Router) getFastLoginScript() string {
	if r.fastLoginScript != "" {
		return r.fastLoginScript
	}
	return "/opt/data/home/agy-profiles/fast_login.py"
}

func (r *Router) SetScheduler(s *scheduler.Scheduler) {
	r.scheduler = s
}

func (r *Router) SetWatchdog(w *watchdog.Watchdog) {
	r.watchdog = w
}

func (r *Router) SetSwarmManager(sm *cluster.SwarmManager) {
	r.swarmManager = sm
}

func (r *Router) HandleUpdate(ctx context.Context, update tgbotapi.Update) {
	if update.CallbackQuery != nil {
		r.handleCallbackQuery(ctx, update.CallbackQuery)
		return
	}

	if update.Message == nil {
		return
	}

	msg := update.Message
	chatID := msg.Chat.ID
	username := msg.From.UserName
	text := strings.TrimSpace(msg.Text)

	isLoginCmd := strings.HasPrefix(text, "/login")
	sess, authorized, rateLimited := r.middleware.ProcessAccess(chatID, username, isLoginCmd)

	if rateLimited {
		r.replyHTML(chatID, "<b>⚠️ RATE LIMIT EXCEEDED</b>\n\nPlease wait a few moments before sending more requests.")
		return
	}

	if !authorized {
		if isLoginCmd {
			r.handleLogin(chatID, text)
			return
		}
		r.replyHTML(chatID, r.renderer.RenderUnauthorized(chatID))
		return
	}

	if r.authManager != nil && r.authManager.IsAuthorized(chatID, sess, 0) && sess != nil && !sess.IsAuthenticated {
		sess.IsAuthenticated = true
		if r.sessionStore != nil {
			r.sessionStore.Authenticate(chatID)
		}
	}

	r.cleanupPreviousMessages(chatID, sess)

	if strings.HasPrefix(text, "/resume_") {
		convID := strings.TrimPrefix(text, "/resume_")
		r.handleResume(chatID, sess, convID)
		return
	}

	if strings.HasPrefix(text, "/cron_run_") {
		jobID := strings.TrimPrefix(text, "/cron_run_")
		r.handleCronRun(ctx, chatID, jobID)
		return
	}

	if strings.HasPrefix(text, "/cron_rm_") {
		jobID := strings.TrimPrefix(text, "/cron_rm_")
		r.handleCronRemove(chatID, jobID)
		return
	}

	if text == "/" {
		r.handleHelp(chatID)
		return
	}

	cmd := strings.ToLower(msg.Command())
	args := strings.TrimSpace(msg.CommandArguments())

	switch cmd {
	case "start":
		r.handleStart(chatID, sess)
	case "help":
		r.handleHelp(chatID)
	case "ps", "procs":
		r.handleProcesses(chatID, sess, args)
	case "kill":
		r.handleKill(chatID, args)
	case "ports", "netstat":
		r.handlePorts(chatID)
	case "service":
		r.handleServices(ctx, chatID, args)
	case "ping":
		r.handlePing(ctx, chatID, args)
	case "curl", "http":
		r.handleHTTP(ctx, chatID, args)
	case "dns":
		r.handleDNS(ctx, chatID, args)
	case "netio":
		r.handleNetIO(chatID)
	case "docker":
		r.handleDocker(ctx, chatID, args)
	case "cron":
		r.handleCron(ctx, chatID, args)
	case "alerts":
		r.handleAlerts(chatID)
	case "backup":
		r.handleBackup(chatID, sess, args)
	case "swarm":
		r.handleSwarm(ctx, chatID, sess, args)
	case "login":
		if args != "" && (r.authManager.ValidateAdminKey(args) || r.authManager.ConsumePairingToken(args)) {
			r.sessionStore.Authenticate(chatID)
			r.replyHTML(chatID, "<b>🟢 AUTHENTICATION SUCCESSFUL</b>\n\nDevice paired successfully. You now have full access to the control plane.")
		} else if args != "" {
			r.handleLoginAccount(ctx, chatID, args)
		} else {
			r.replyHTML(chatID, "<b>Usage:</b> <code>/login &lt;account&gt;</code>\nExample: <code>/login agy01</code>")
		}
	case "logout":
		if args != "" {
			r.handleLogoutAccount(ctx, chatID, args)
		} else {
			r.handleLogout(chatID)
		}
	case "switch":
		r.handleSwitch(chatID, sess, args)
	case "send":
		r.handleSend(ctx, chatID, args)
	case "tasks":
		r.handleTasks(ctx, chatID)
	case "workers":
		r.handleWorkers(ctx, chatID)
	case "usage":
		r.handleUsage(ctx, chatID)
	case "accounts":
		r.handleAccounts(ctx, chatID)
	case "up", "start_worker", "hidupkan":
		r.handleUp(ctx, chatID, args)
	case "refresh":
		if args != "" {
			r.handleUp(ctx, chatID, args)
		} else {
			r.handleUp(ctx, chatID, "semua")
		}
	case "ask":
		r.handleAsk(ctx, chatID, sess, args)
	case "profile":
		r.handleProfile(chatID, sess, args)
	case "history":
		r.handleHistory(chatID, sess)
	case "resume":
		r.handleResume(chatID, sess, args)
	case "new":
		r.handleNew(chatID, sess)
	case "status":
		r.handleStatus(chatID, sess)
	case "ls":
		r.handleLs(chatID, sess, args)
	case "cd":
		r.handleCd(chatID, sess, args)
	case "cat":
		r.handleCat(chatID, sess, args)
	case "get":
		r.handleGet(chatID, sess, args)
	case "sh":
		r.handleShell(ctx, chatID, sess, args)
	case "git":
		r.handleGit(ctx, chatID, sess, args)
	default:
		if !msg.IsCommand() && text != "" {
			lowerText := strings.ToLower(text)
			if strings.HasPrefix(lowerText, "hidupkan") {
				upArg := strings.TrimSpace(strings.TrimPrefix(text[len("hidupkan"):], ":"))
				r.handleUp(ctx, chatID, upArg)
				return
			}
			if strings.HasPrefix(lowerText, "start_worker") {
				upArg := strings.TrimSpace(strings.TrimPrefix(text[len("start_worker"):], ":"))
				r.handleUp(ctx, chatID, upArg)
				return
			}
			if lowerText == "up" || strings.HasPrefix(lowerText, "up ") {
				upArg := strings.TrimSpace(strings.TrimPrefix(text[len("up"):], ":"))
				r.handleUp(ctx, chatID, upArg)
				return
			}
			r.handleAsk(ctx, chatID, sess, text)
		} else if msg.IsCommand() {
			r.replyHTML(chatID, fmt.Sprintf("<b>❓ Unknown Command:</b> <code>/%s</code>\nType <code>/help</code> for available commands.", html.EscapeString(cmd)))
		}
	}
}

func (r *Router) cleanupPreviousMessages(chatID int64, sess *session.Session) {
	if sess == nil || len(sess.LastBotMessageIDs) == 0 || r.api == nil {
		return
	}
	for _, msgID := range sess.LastBotMessageIDs {
		del := tgbotapi.NewDeleteMessage(chatID, msgID)
		_, _ = r.api.Request(del)
	}
	r.sessionStore.ClearLastBotMessageIDs(chatID)
}

func (r *Router) getStartMenuMarkup() tgbotapi.InlineKeyboardMarkup {
	return r.renderer.GetStartKeyboard()
}

func (r *Router) getBackMarkup() tgbotapi.InlineKeyboardMarkup {
	return r.renderer.GetBackKeyboard()
}

func (r *Router) handleStart(chatID int64, sess *session.Session) {
	botUsername := "AgyRemoteTeleBot"
	if r.api != nil {
		botUsername = r.api.Self.UserName
	}
	text := r.renderer.RenderWelcome(botUsername, sess.ActiveProfile, sess)
	keyboard := r.getStartMenuMarkup()

	if r.api == nil {
		return
	}
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeHTML
	msg.ReplyMarkup = keyboard
	sent, err := r.api.Send(msg)
	if err == nil && r.sessionStore != nil {
		r.sessionStore.SetLastBotMessageIDs(chatID, []int{sent.MessageID})
	}
}

func (r *Router) handleHelp(chatID int64) {
	r.replyHTMLWithBack(chatID, r.renderer.RenderHelp())
}

func (r *Router) handleLogin(chatID int64, text string) {
	parts := strings.Fields(text)
	if len(parts) < 2 {
		r.replyHTML(chatID, "<b>Usage:</b> <code>/login &lt;auth_key&gt;</code>")
		return
	}

	key := parts[1]
	if r.authManager.ValidateAdminKey(key) || r.authManager.ConsumePairingToken(key) {
		r.sessionStore.Authenticate(chatID)
		r.replyHTML(chatID, "<b>🟢 AUTHENTICATION SUCCESSFUL</b>\n\nDevice paired successfully. You now have full access to the control plane.")
		return
	}

	r.replyHTML(chatID, "<b>🔴 AUTHENTICATION FAILED</b>\n\nInvalid pairing key or secret token.")
}

func (r *Router) handleLogout(chatID int64) {
	r.sessionStore.Deauthenticate(chatID)
	r.replyHTML(chatID, "<b>🔒 LOGGED OUT</b>\n\nYour active session has been invalidated.")
}

func (r *Router) handleProfile(chatID int64, sess *session.Session, targetProfile string) {
	if targetProfile != "" {
		clean := strings.ToLower(strings.TrimSpace(targetProfile))
		if !r.profileManager.ValidateProfile(clean) {
			r.replyHTML(chatID, fmt.Sprintf("<b>🔴 Error:</b> Profile <code>%s</code> is not valid or directory not found.", html.EscapeString(clean)))
			return
		}
		oldProfile := sess.ActiveProfile
		r.sessionStore.SetProfile(chatID, clean)
		r.replyHTMLWithBack(chatID, r.renderer.RenderProfileSwitched(oldProfile, clean))
		return
	}

	profiles, err := r.profileManager.ListProfiles()
	if err != nil || len(profiles) == 0 {
		r.replyHTMLWithBack(chatID, fmt.Sprintf("<b>Active Profile:</b> <code>%s</code>\nNo other profile directories discovered.", html.EscapeString(sess.ActiveProfile)))
		return
	}

	var rows [][]tgbotapi.InlineKeyboardButton
	var currentRow []tgbotapi.InlineKeyboardButton
	for _, p := range profiles {
		label := p
		if p == sess.ActiveProfile {
			label = "🟢 " + p
		}
		btn := tgbotapi.NewInlineKeyboardButtonData(label, "profile:"+p)
		currentRow = append(currentRow, btn)
		if len(currentRow) == 4 {
			rows = append(rows, currentRow)
			currentRow = nil
		}
	}
	if len(currentRow) > 0 {
		rows = append(rows, currentRow)
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("« Menu Utama", "nav:start"),
	))

	msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("<b>👤 PROFILE SWITCHER</b>\n\nCurrent Active: 🟢 <code>%s</code>\nSelect worker profile:", html.EscapeString(sess.ActiveProfile)))
	msg.ParseMode = tgbotapi.ModeHTML
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	sent, err := r.api.Send(msg)
	if err == nil {
		r.sessionStore.SetLastBotMessageIDs(chatID, []int{sent.MessageID})
	}
}

func (r *Router) handleHistory(chatID int64, sess *session.Session) {
	items, err := r.historyReader.ListRecentConversations(sess.ActiveProfile, 8)
	if err != nil {
		r.replyHTMLWithBack(chatID, fmt.Sprintf("<b>🔴 Error reading history:</b> %s", html.EscapeString(err.Error())))
		return
	}
	r.replyHTMLWithBack(chatID, r.renderer.RenderHistory(sess.ActiveProfile, items))
}

func (r *Router) handleResume(chatID int64, sess *session.Session, convID string) {
	cleanID := strings.TrimSpace(convID)
	if cleanID == "" {
		r.replyHTML(chatID, "<b>Usage:</b> <code>/resume &lt;conversation_id&gt;</code>")
		return
	}

	r.sessionStore.SetConversation(chatID, cleanID)
	r.replyHTMLWithBack(chatID, fmt.Sprintf("<b>⚡ SESSION RESUMED</b>\n\nActive Conversation: <code>%s</code>\nProfile: <code>%s</code>\n\nSubsequent <code>/ask</code> instructions will continue this conversation.",
		html.EscapeString(cleanID), html.EscapeString(sess.ActiveProfile)))
}

func (r *Router) handleNew(chatID int64, sess *session.Session) {
	r.sessionStore.ClearConversation(chatID)
	r.replyHTMLWithBack(chatID, fmt.Sprintf("<b>✨ NEW SESSION INITIALIZED</b>\n\nCleared active conversation context on <code>%s</code>. Next instruction will start fresh.", html.EscapeString(sess.ActiveProfile)))
}

func (r *Router) handleStatus(chatID int64, sess *session.Session) {
	metrics, err := system.CollectMetrics(sess.ActiveWorkingDir)
	if err != nil {
		r.replyHTMLWithBack(chatID, fmt.Sprintf("<b>🔴 Telemetry Error:</b> %s", html.EscapeString(err.Error())))
		return
	}
	r.replyHTMLWithBack(chatID, r.renderer.RenderStatus(metrics, sess.ActiveProfile))
}

func (r *Router) handleLs(chatID int64, sess *session.Session, targetDir string) {
	dir := sess.ActiveWorkingDir
	if targetDir != "" {
		if filepath.IsAbs(targetDir) {
			dir = targetDir
		} else {
			dir = filepath.Join(sess.ActiveWorkingDir, targetDir)
		}
	}

	items, err := workspace.ListDirectory(dir)
	if err != nil {
		r.replyHTMLWithBack(chatID, fmt.Sprintf("<b>🔴 Directory Error:</b> %s", html.EscapeString(err.Error())))
		return
	}

	r.replyHTMLWithBack(chatID, r.renderer.RenderFileList(dir, items))
}

func (r *Router) handleCd(chatID int64, sess *session.Session, targetDir string) {
	clean := strings.TrimSpace(targetDir)
	if clean == "" {
		r.replyHTML(chatID, "<b>Usage:</b> <code>/cd &lt;path&gt;</code>")
		return
	}

	dest := clean
	if !filepath.IsAbs(dest) {
		dest = filepath.Join(sess.ActiveWorkingDir, dest)
	}
	dest = filepath.Clean(dest)

	info, err := os.Stat(dest)
	if err != nil || !info.IsDir() {
		r.replyHTML(chatID, fmt.Sprintf("<b>🔴 Path Error:</b> <code>%s</code> is not a valid directory.", html.EscapeString(dest)))
		return
	}

	r.sessionStore.SetWorkingDir(chatID, dest)
	r.replyHTMLWithBack(chatID, fmt.Sprintf("<b>📁 Working directory updated:</b>\n<code>%s</code>", html.EscapeString(dest)))
}

func (r *Router) handleCat(chatID int64, sess *session.Session, targetFile string) {
	clean := strings.TrimSpace(targetFile)
	if clean == "" {
		r.replyHTML(chatID, "<b>Usage:</b> <code>/cat &lt;file_path&gt;</code>")
		return
	}

	filePath := clean
	if !filepath.IsAbs(filePath) {
		filePath = filepath.Join(sess.ActiveWorkingDir, filePath)
	}

	content, truncated, err := workspace.ReadFile(filePath, 32768)
	if err != nil {
		r.replyHTML(chatID, fmt.Sprintf("<b>🔴 Read Error:</b> %s", html.EscapeString(err.Error())))
		return
	}

	if r.renderer.ShouldSendAsDocument(content) {
		r.sendDocument(chatID, filePath)
		return
	}

	truncNotice := ""
	if truncated {
		truncNotice = "\n\n<i>(File truncated, download via /get for full content)</i>"
	}

	msgText := fmt.Sprintf("<b>📄 %s</b>\n<pre><code>%s</code></pre>%s",
		html.EscapeString(filepath.Base(filePath)), html.EscapeString(content), truncNotice)
	r.replyChunks(chatID, msgText)
}

func (r *Router) handleGet(chatID int64, sess *session.Session, targetFile string) {
	clean := strings.TrimSpace(targetFile)
	if clean == "" {
		r.replyHTML(chatID, "<b>Usage:</b> <code>/get &lt;file_path&gt;</code>")
		return
	}

	filePath := clean
	if !filepath.IsAbs(filePath) {
		filePath = filepath.Join(sess.ActiveWorkingDir, filePath)
	}

	realPath, err := workspace.ValidateRegularFile(filePath)
	if err != nil {
		r.replyHTML(chatID, fmt.Sprintf("<b>🔴 File Error:</b> %s", html.EscapeString(err.Error())))
		return
	}

	r.sendDocument(chatID, realPath)
}

func (r *Router) handleShell(ctx context.Context, chatID int64, sess *session.Session, cmdStr string) {
	clean := strings.TrimSpace(cmdStr)
	if clean == "" {
		r.replyHTML(chatID, "<b>Usage:</b> <code>/sh &lt;command&gt;</code>")
		return
	}

	execCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	r.sendTyping(chatID)

	out, err := system.RunCommand(execCtx, clean, sess.ActiveWorkingDir)
	if err != nil && out == nil {
		r.replyHTML(chatID, fmt.Sprintf("<b>🔴 Shell Execution Error:</b>\n<pre><code>%s</code></pre>", html.EscapeString(err.Error())))
		return
	}

	statusIcon := "🟢"
	if out.ExitCode != 0 {
		statusIcon = "🔴"
	}

	resp := fmt.Sprintf("<b>%s Shell Result</b> (Exit %d | %.2fs)\n<pre><code>%s</code></pre>",
		statusIcon, out.ExitCode, out.Duration.Seconds(), html.EscapeString(out.Combined))

	if len(out.Combined) == 0 {
		resp = fmt.Sprintf("<b>%s Shell Executed</b> (Exit %d | %.2fs)\n<i>(No stdout/stderr produced)</i>",
			statusIcon, out.ExitCode, out.Duration.Seconds())
	}

	r.replyChunks(chatID, resp)
}

func (r *Router) handleGit(ctx context.Context, chatID int64, sess *session.Session, arg string) {
	clean := strings.TrimSpace(arg)
	repoPath := sess.ActiveWorkingDir

	if strings.HasPrefix(clean, "push") || strings.HasPrefix(clean, "commit") {
		msg := "chore: auto-sync via AgyRemoteTele"
		parts := strings.SplitN(clean, " ", 2)
		if len(parts) == 2 && strings.TrimSpace(parts[1]) != "" {
			msg = strings.TrimSpace(parts[1])
		}
		r.sendTyping(chatID)
		out, err := workspace.CommitAndPush(ctx, repoPath, msg)
		if err != nil {
			r.replyHTMLWithBack(chatID, fmt.Sprintf("<b>🔴 Git Sync Failed:</b>\n<pre><code>%s</code></pre>", html.EscapeString(err.Error())))
			return
		}
		r.replyHTMLWithBack(chatID, fmt.Sprintf("<b>🟢 Git Sync Successful:</b>\n<pre><code>%s</code></pre>", html.EscapeString(out)))
		return
	}

	summary, err := workspace.InspectRepository(repoPath)
	if err != nil {
		r.replyHTMLWithBack(chatID, fmt.Sprintf("<b>🔴 Git Error:</b> %s", html.EscapeString(err.Error())))
		return
	}

	r.replyHTMLWithBack(chatID, r.renderer.RenderGitSummary(summary))
}

func wrapPrompt(chatID int64, profile string, workingDir string, prompt string) string {
	return fmt.Sprintf(`[TELEGRAM AGY REMOTE GATEWAY — AUTONOMOUS OPERATOR DIRECTIVE]
Pengirim: Telegram Operator (Chat ID: %d)
Worker Profil: %s
Working Directory: %s

INSTRUKSI PENGGUNA:
%s

STANDAR OPERASIONAL CERDAS (FULL AUTONOMOUS & SELF-HEALING):
1. OTONOMI PENUH: Eksekusi tools, bash command, dan tindakan teknis secara langsung tanpa perlu meminta izin bolak-balik ke operator.
2. SELF-HEALING & MAINTENANCE: Jika menemukan bug, error sistem, atau kegagalan pada bot remote AgyRemoteTele atau script worker, agy WAJIB langsung mengaudit dan memperbaiki (edit) kode terkait hingga tuntas.
3. STANDAR NOKOMEN: DILARANG menulis komentar dalam kode yang dibuat atau diedit.
4. GIT-FIRST: Pastikan setiap perubahan kode yang teruji di-commit dan siap di-push ke GitHub repository.
5. FORMAT RESPON TELEGRAM: Berikan jawaban akhir yang SINGKAT, PADAT, dan TO-THE-POINT mengenai apa yang telah dikerjakan, status perbaikan, dan hasil verifikasi.`,
		chatID, profile, workingDir, prompt)
}

func (r *Router) handleAsk(ctx context.Context, chatID int64, sess *session.Session, prompt string) {
	cleanPrompt := strings.TrimSpace(prompt)
	if cleanPrompt == "" {
		r.replyHTML(chatID, "<b>Usage:</b> <code>/ask &lt;your prompt here&gt;</code>")
		return
	}

	placeholderText := fmt.Sprintf("⏳ <b>Processing task on [%s]...</b>\n<i>Prompt:</i> <code>%s</code>",
		html.EscapeString(sess.ActiveProfile), html.EscapeString(truncateString(cleanPrompt, 60)))
	initMsg := tgbotapi.NewMessage(chatID, placeholderText)
	initMsg.ParseMode = tgbotapi.ModeHTML
	sentMsg, err := r.api.Send(initMsg)

	execCtx, cancel := context.WithTimeout(ctx, r.execTimeout)
	defer cancel()

	typingDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(4 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-typingDone:
				return
			case <-ticker.C:
				r.sendTyping(chatID)
			}
		}
	}()

	wrappedPrompt := wrapPrompt(chatID, sess.ActiveProfile, sess.ActiveWorkingDir, cleanPrompt)

	req := agy.ExecutionRequest{
		Profile:        sess.ActiveProfile,
		Prompt:         wrappedPrompt,
		ConversationID: sess.ActiveConversationID,
		WorkingDir:     sess.ActiveWorkingDir,
	}

	result, execErr := r.executor.Execute(execCtx, req)
	close(typingDone)

	if result != nil && result.ConversationID != "" {
		r.sessionStore.SetConversation(chatID, result.ConversationID)
	}

	if err == nil {
		del := tgbotapi.NewDeleteMessage(chatID, sentMsg.MessageID)
		_, _ = r.api.Request(del)
	}

	if execErr != nil && (result == nil || result.Response == "") {
		r.replyHTML(chatID, fmt.Sprintf("<b>🔴 TASK EXECUTION FAILED</b>\n\n<code>%s</code>", html.EscapeString(execErr.Error())))
		return
	}

	rep := reporter.BuildReport(sess.ActiveProfile, result, sess.ActiveWorkingDir)
	reportHTML := rep.RenderHTML()

	var sentIDs []int

	msgRep := tgbotapi.NewMessage(chatID, reportHTML)
	msgRep.ParseMode = tgbotapi.ModeHTML
	if sent, err := r.api.Send(msgRep); err == nil {
		sentIDs = append(sentIDs, sent.MessageID)
	}

	if result.Response != "" {
		respHeader := fmt.Sprintf("<b>💬 Agent Response:</b>\n\n%s", html.EscapeString(result.Response))
		chunks := r.renderer.ChunkMessage(respHeader, 4000)
		for _, chunk := range chunks {
			msg := tgbotapi.NewMessage(chatID, chunk)
			msg.ParseMode = tgbotapi.ModeHTML
			if sent, err := r.api.Send(msg); err == nil {
				sentIDs = append(sentIDs, sent.MessageID)
			}
		}
	}

	r.sessionStore.SetLastBotMessageIDs(chatID, sentIDs)
}

func renderAccountsStatus(out []byte) (string, tgbotapi.InlineKeyboardMarkup) {
	lines := strings.Split(string(out), "\n")
	type accInfo struct {
		name      string
		status    string
		remaining string
		email     string
		active    bool
	}

	var accounts []accInfo
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.Contains(trimmed, "|") {
			continue
		}
		parts := strings.Split(trimmed, "|")
		if len(parts) != 4 {
			continue
		}
		name := strings.TrimSpace(parts[0])
		if !strings.HasPrefix(strings.ToLower(name), "agy") {
			continue
		}
		status := strings.TrimSpace(parts[1])
		remaining := strings.TrimSpace(parts[2])
		email := strings.TrimSpace(parts[3])
		isActive := strings.EqualFold(status, "Aktif")

		accounts = append(accounts, accInfo{
			name:      name,
			status:    status,
			remaining: remaining,
			email:     email,
			active:    isActive,
		})
	}

	if len(accounts) == 0 {
		markup := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("⚡ Hidupkan Semua", "nav:up_all"),
				tgbotapi.NewInlineKeyboardButtonData("« Menu Utama", "nav:start"),
			),
		)
		return fmt.Sprintf("<b>👥 AGY WORKER ACCOUNTS</b>\n\n<pre><code>%s</code></pre>", html.EscapeString(string(out))), markup
	}

	activeCount := 0
	for _, acc := range accounts {
		if acc.active {
			activeCount++
		}
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("<b>👥 AGY WORKER ACCOUNTS (01 - 10)</b>\nActive: <code>%d/%d</code> Workers\n\n", activeCount, len(accounts)))

	for _, acc := range accounts {
		emoji := "🔴"
		desc := acc.status
		lowerStatus := strings.ToLower(acc.status)
		isStandby := strings.EqualFold(acc.status, "Standby") ||
			strings.EqualFold(acc.status, "Siap Aktif") ||
			strings.Contains(lowerStatus, "standby") ||
			strings.Contains(lowerStatus, "siap aktif")

		if acc.active {
			emoji = "🟢"
		} else if isStandby {
			emoji = "🟡"
			desc = fmt.Sprintf("%s — Siap Dihidupkan", acc.status)
		}
		b.WriteString(fmt.Sprintf("%s <b>%s</b> • <b>%s</b>\n", emoji, html.EscapeString(acc.name), html.EscapeString(desc)))
		b.WriteString(fmt.Sprintf("  ├ ⏱ <code>%s</code>\n", html.EscapeString(acc.remaining)))
		b.WriteString(fmt.Sprintf("  └ 📧 <code>%s</code>\n\n", html.EscapeString(acc.email)))
	}

	var rows [][]tgbotapi.InlineKeyboardButton
	var currentRow []tgbotapi.InlineKeyboardButton
	for _, acc := range accounts {
		btn := tgbotapi.NewInlineKeyboardButtonData("⚡ "+acc.name, "up_acc:"+acc.name)
		currentRow = append(currentRow, btn)
		if len(currentRow) == 5 {
			rows = append(rows, currentRow)
			currentRow = nil
		}
	}
	if len(currentRow) > 0 {
		rows = append(rows, currentRow)
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("⚡ Hidupkan Semua", "nav:up_all"),
		tgbotapi.NewInlineKeyboardButtonData("« Menu Utama", "nav:start"),
	))

	return strings.TrimRight(b.String(), "\n"), tgbotapi.NewInlineKeyboardMarkup(rows...)
}

func renderUpSingle(profile, status, email, remaining, detail string, isErr bool) (string, tgbotapi.InlineKeyboardMarkup) {
	if profile == "" {
		profile = "Worker"
	}
	if status == "" {
		if isErr {
			status = "Gagal"
		} else {
			status = "Berhasil dihidupkan"
		}
	}
	emoji := "🟢"
	lowerSt := strings.ToLower(status)
	if isErr || strings.Contains(lowerSt, "gagal") || strings.Contains(lowerSt, "error") {
		emoji = "🔴"
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("<b>⚡ WORKER DIAKTIFKAN — %s</b>\n\n", strings.ToUpper(profile)))
	b.WriteString(fmt.Sprintf("Status: %s <b>%s</b>\n", emoji, html.EscapeString(status)))
	if remaining != "" && remaining != "-" {
		b.WriteString(fmt.Sprintf("⏱ <b>Sisa Waktu:</b> <code>%s</code>\n", html.EscapeString(remaining)))
	}
	if email != "" && email != "-" && email != "Unknown" {
		b.WriteString(fmt.Sprintf("📧 <b>Email Aktif:</b> <code>%s</code>\n", html.EscapeString(email)))
	}
	if detail != "" {
		b.WriteString(fmt.Sprintf("⚠️ <b>Detail:</b> <code>%s</code>\n", html.EscapeString(detail)))
	}

	markup := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("👥 Akun Worker", "nav:accounts"),
			tgbotapi.NewInlineKeyboardButtonData("« Menu Utama", "nav:start"),
		),
	)
	return strings.TrimRight(b.String(), "\n"), markup
}

func renderUpAll(outStr string, err error) (string, tgbotapi.InlineKeyboardMarkup) {
	reAll := regexp.MustCompile(`^\[(agy\d+)\]\s+(.*?)(?:\s*\((.*?)\))?\s*-\s*(.*)$`)
	var items []string
	lines := strings.Split(outStr, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[agy") {
			emoji := "🟢"
			lowerLine := strings.ToLower(trimmed)
			if strings.Contains(lowerLine, "gagal") || strings.Contains(lowerLine, "error") {
				emoji = "🔴"
			} else if strings.Contains(lowerLine, "dilewati") {
				emoji = "🟡"
			}

			m := reAll.FindStringSubmatch(trimmed)
			if len(m) >= 5 {
				accName := strings.ToUpper(m[1])
				statusDesc := m[4]
				emailVal := m[3]
				if emailVal != "" && emailVal != "Unknown" {
					items = append(items, fmt.Sprintf("%s <b>%s</b> • %s\n   └ 📧 <code>%s</code>", emoji, html.EscapeString(accName), html.EscapeString(statusDesc), html.EscapeString(emailVal)))
				} else {
					items = append(items, fmt.Sprintf("%s <b>%s</b> • %s", emoji, html.EscapeString(accName), html.EscapeString(statusDesc)))
				}
			} else {
				items = append(items, fmt.Sprintf("%s <code>%s</code>", emoji, html.EscapeString(trimmed)))
			}
		}
	}

	var b strings.Builder
	b.WriteString("<b>⚡ SEMUA WORKER DIAKTIFKAN (AGY01 - AGY10)</b>\n\n")
	if len(items) > 0 {
		b.WriteString(strings.Join(items, "\n"))
	} else if len(outStr) > 0 {
		b.WriteString(fmt.Sprintf("<pre><code>%s</code></pre>", html.EscapeString(outStr)))
	} else {
		b.WriteString("<i>Semua token worker berhasil diproses.</i>")
	}

	if err != nil && len(items) == 0 {
		b.WriteString(fmt.Sprintf("\n\n<b>Notice:</b> <code>%s</code>", html.EscapeString(err.Error())))
	}

	markup := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("👥 Akun Worker", "nav:accounts"),
			tgbotapi.NewInlineKeyboardButtonData("« Menu Utama", "nav:start"),
		),
	)
	return strings.TrimRight(b.String(), "\n"), markup
}

func (r *Router) handleAccounts(ctx context.Context, chatID int64) {
	r.sendTyping(chatID)

	execCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(execCtx, "python3", r.getFastLoginScript(), "status")
	out, err := cmd.CombinedOutput()
	if err != nil && len(out) == 0 {
		r.replyHTMLWithBack(chatID, fmt.Sprintf("<b>🔴 Accounts Status Error:</b>\n<pre><code>%s</code></pre>", html.EscapeString(err.Error())))
		return
	}

	text, markup := renderAccountsStatus(out)
	r.replyHTMLWithMarkup(chatID, text, markup)
}

func (r *Router) handleUp(ctx context.Context, chatID int64, account string) {
	r.sendTyping(chatID)

	cleanArg := strings.ToLower(strings.TrimSpace(account))
	isAll := cleanArg == "" || cleanArg == "semua" || cleanArg == "all"

	execCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	typingDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(4 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-typingDone:
				return
			case <-ticker.C:
				r.sendTyping(chatID)
			}
		}
	}()

	scriptPath := r.getFastLoginScript()
	var cmd *exec.Cmd
	var targetAcc string
	if isAll {
		cmd = exec.CommandContext(execCtx, "python3", scriptPath, "refresh")
	} else {
		targetAcc = normalizeAccount(cleanArg)
		cmd = exec.CommandContext(execCtx, "python3", scriptPath, "refresh", targetAcc)
	}

	out, err := cmd.CombinedOutput()
	close(typingDone)

	if err != nil && len(out) == 0 {
		label := "Semua Worker"
		if !isAll {
			label = targetAcc
		}
		r.replyHTMLWithBack(chatID, fmt.Sprintf("<b>🔴 Gagal Menghidupkan Worker (%s):</b>\n<pre><code>%s</code></pre>", html.EscapeString(label), html.EscapeString(err.Error())))
		return
	}

	outStr := strings.TrimSpace(string(out))

	if !isAll {
		var prof, st, em, remaining, detail string
		for _, line := range strings.Split(outStr, "\n") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				k := strings.ToLower(strings.TrimSpace(parts[0]))
				v := strings.TrimSpace(parts[1])
				switch k {
				case "profil":
					prof = v
				case "status":
					st = v
				case "email":
					em = v
				case "sisa waktu":
					remaining = v
				case "detail":
					detail = v
				}
			}
		}
		if prof == "" {
			prof = targetAcc
		}
		text, markup := renderUpSingle(prof, st, em, remaining, detail, err != nil && strings.Contains(strings.ToLower(outStr), "error"))
		r.replyHTMLWithMarkup(chatID, text, markup)
		return
	}

	text, markup := renderUpAll(outStr, err)
	r.replyHTMLWithMarkup(chatID, text, markup)
}

func (r *Router) handleRefresh(ctx context.Context, chatID int64) {
	r.handleUp(ctx, chatID, "semua")
}

func (r *Router) handleCallbackQuery(ctx context.Context, cb *tgbotapi.CallbackQuery) {
	data := cb.Data
	chatID := cb.Message.Chat.ID
	msgID := cb.Message.MessageID
	username := cb.From.UserName

	sess, authorized, _ := r.middleware.ProcessAccess(chatID, username, false)
	if !authorized {
		callbackResp := tgbotapi.NewCallback(cb.ID, "Unauthorized device")
		_, _ = r.api.Request(callbackResp)
		return
	}

	if r.authManager != nil && r.authManager.IsAuthorized(chatID, sess, 0) && sess != nil && !sess.IsAuthenticated {
		sess.IsAuthenticated = true
		if r.sessionStore != nil {
			r.sessionStore.Authenticate(chatID)
		}
	}

	callbackResp := tgbotapi.NewCallback(cb.ID, "")
	if r.api != nil {
		_, _ = r.api.Request(callbackResp)
	}

	if data == "nav:start" {
		botUsername := "AgyRemoteTeleBot"
		if r.api != nil {
			botUsername = r.api.Self.UserName
		}
		text := r.renderer.RenderWelcome(botUsername, sess.ActiveProfile, sess)
		markup := r.getStartMenuMarkup()
		r.editMessage(chatID, msgID, text, &markup)
		return
	}

	if strings.HasPrefix(data, "profile:") {
		target := strings.TrimPrefix(data, "profile:")
		oldProfile := sess.ActiveProfile
		r.sessionStore.SetProfile(chatID, target)
		text := r.renderer.RenderProfileSwitched(oldProfile, target)
		markup := r.getBackMarkup()
		r.editMessage(chatID, msgID, text, &markup)
		return
	}

	if strings.HasPrefix(data, "resume:") {
		target := strings.TrimPrefix(data, "resume:")
		r.sessionStore.SetConversation(chatID, target)
		text := fmt.Sprintf("<b>⚡ SESSION RESUMED</b>\n\nActive Conversation: <code>%s</code>\nProfile: <code>%s</code>",
			html.EscapeString(target), html.EscapeString(sess.ActiveProfile))
		markup := r.getBackMarkup()
		r.editMessage(chatID, msgID, text, &markup)
		return
	}

	if strings.HasPrefix(data, "up_acc:") {
		target := strings.TrimPrefix(data, "up_acc:")
		r.handleUp(ctx, chatID, target)
		return
	}

	if strings.HasPrefix(data, "refresh_acc:") {
		target := strings.TrimPrefix(data, "refresh_acc:")
		r.handleUp(ctx, chatID, target)
		return
	}

	if strings.HasPrefix(data, "cron_run:") {
		target := strings.TrimPrefix(data, "cron_run:")
		r.handleCronRun(ctx, chatID, target)
		return
	}

	if strings.HasPrefix(data, "cron_rm:") {
		target := strings.TrimPrefix(data, "cron_rm:")
		r.handleCronRemove(chatID, target)
		return
	}

	switch data {
	case "nav:cat_system":
		text := "<b>💻 SYSTEM & OS CONTROL</b>\n\nMonitor active processes, network listening ports, system services, and watchdog alerts."
		markup := r.renderer.GetCategoryKeyboard("system")
		r.editMessage(chatID, msgID, text, &markup)

	case "nav:cat_docker":
		text := "<b>🐳 DOCKER CONTAINER MANAGEMENT</b>\n\nInspect container states, live resource consumption, and logs."
		markup := r.renderer.GetCategoryKeyboard("docker")
		r.editMessage(chatID, msgID, text, &markup)

	case "nav:cat_net":
		text := "<b>🌐 NETWORK TOOLS & DIAGNOSTICS</b>\n\nNetwork interface traffic, ICMP ping testing, and DNS lookups."
		markup := r.renderer.GetCategoryKeyboard("net")
		r.editMessage(chatID, msgID, text, &markup)

	case "nav:cat_scheduler":
		text := "<b>⏱ TASK SCHEDULER & CRON</b>\n\nView scheduled jobs or manage recurring tasks."
		markup := r.renderer.GetCategoryKeyboard("scheduler")
		r.editMessage(chatID, msgID, text, &markup)

	case "nav:cat_swarm":
		text := "<b>🐝 CLUSTER SWARM ORCHESTRATION</b>\n\nMulti-worker status, task queue, and resource metrics."
		markup := r.renderer.GetCategoryKeyboard("swarm")
		r.editMessage(chatID, msgID, text, &markup)

	case "nav:cat_backup":
		text := "<b>📦 BACKUP & ARCHIVAL</b>\n\nCreate tar.gz archives of workspace directories."
		markup := r.renderer.GetCategoryKeyboard("backup")
		r.editMessage(chatID, msgID, text, &markup)

	case "nav:ps":
		procs, err := system.ListProcesses(15, "cpu")
		text := ""
		if err != nil {
			text = fmt.Sprintf("<b>🔴 Process List Error:</b> %s", html.EscapeString(err.Error()))
		} else {
			text = r.renderer.RenderProcesses(procs)
		}
		markup := r.getBackMarkup()
		r.editMessage(chatID, msgID, text, &markup)

	case "nav:ports":
		ports, err := system.ListListeningPorts()
		text := ""
		if err != nil {
			text = fmt.Sprintf("<b>🔴 Ports Error:</b> %s", html.EscapeString(err.Error()))
		} else {
			text = r.renderer.RenderListeningPorts(ports)
		}
		markup := r.getBackMarkup()
		r.editMessage(chatID, msgID, text, &markup)

	case "nav:services":
		services, err := system.ListCommonServices(ctx)
		text := ""
		if err != nil {
			text = fmt.Sprintf("<b>🔴 Service Error:</b> %s", html.EscapeString(err.Error()))
		} else {
			text = r.renderer.RenderServices(services)
		}
		markup := r.getBackMarkup()
		r.editMessage(chatID, msgID, text, &markup)

	case "nav:alerts":
		var alerts []watchdog.AlertEvent
		if r.watchdog != nil {
			alerts = r.watchdog.GetRecentAlerts()
			if len(alerts) == 0 {
				alerts = r.watchdog.CheckNow()
			}
		}
		text := r.renderer.RenderAlerts(alerts)
		markup := r.getBackMarkup()
		r.editMessage(chatID, msgID, text, &markup)

	case "nav:docker_ps":
		containers, err := docker.ListContainers(ctx, true)
		text := ""
		if err != nil {
			text = fmt.Sprintf("<b>🔴 Docker PS Error:</b> %s", html.EscapeString(err.Error()))
		} else {
			text = r.renderer.RenderContainers(containers)
		}
		markup := r.getBackMarkup()
		r.editMessage(chatID, msgID, text, &markup)

	case "nav:docker_stats":
		stats, err := docker.GetContainerStats(ctx)
		text := ""
		if err != nil {
			text = fmt.Sprintf("<b>🔴 Docker Stats Error:</b> %s", html.EscapeString(err.Error()))
		} else {
			text = r.renderer.RenderContainerStats(stats)
		}
		markup := r.getBackMarkup()
		r.editMessage(chatID, msgID, text, &markup)

	case "nav:netio":
		netStats, err := nettools.GetNetworkIO()
		text := ""
		if err != nil {
			text = fmt.Sprintf("<b>🔴 Net I/O Error:</b> %s", html.EscapeString(err.Error()))
		} else {
			text = r.renderer.RenderNetworkIO(netStats)
		}
		markup := r.getBackMarkup()
		r.editMessage(chatID, msgID, text, &markup)

	case "nav:ping_default":
		res, err := nettools.Ping(ctx, "1.1.1.1", 3)
		text := ""
		if err != nil {
			text = fmt.Sprintf("<b>🔴 Ping Error:</b> %s", html.EscapeString(err.Error()))
		} else {
			text = r.renderer.RenderPing(res)
		}
		markup := r.getBackMarkup()
		r.editMessage(chatID, msgID, text, &markup)

	case "nav:cron_list":
		var jobs []*scheduler.CronJob
		if r.scheduler != nil {
			jobs = r.scheduler.ListJobs()
		}
		text := r.renderer.RenderCronJobs(jobs)
		markup := r.getBackMarkup()
		r.editMessage(chatID, msgID, text, &markup)

	case "nav:backup_now":
		r.handleBackup(chatID, sess, "")

	case "nav:status":
		metrics, err := system.CollectMetrics(sess.ActiveWorkingDir)
		text := ""
		if err != nil {
			text = fmt.Sprintf("<b>🔴 Telemetry Error:</b> %s", html.EscapeString(err.Error()))
		} else {
			text = r.renderer.RenderStatus(metrics, sess.ActiveProfile)
		}
		markup := r.getBackMarkup()
		r.editMessage(chatID, msgID, text, &markup)

	case "nav:accounts":
		r.handleAccounts(ctx, chatID)

	case "nav:up_all", "nav:refresh":
		r.handleUp(ctx, chatID, "semua")

	case "nav:workers":
		r.handleWorkers(ctx, chatID)

	case "nav:tasks":
		r.handleTasks(ctx, chatID)

	case "nav:usage":
		r.handleUsage(ctx, chatID)

	case "nav:profile":
		profiles, _ := r.profileManager.ListProfiles()
		var rows [][]tgbotapi.InlineKeyboardButton
		var currentRow []tgbotapi.InlineKeyboardButton
		for _, p := range profiles {
			label := p
			if p == sess.ActiveProfile {
				label = "🟢 " + p
			}
			btn := tgbotapi.NewInlineKeyboardButtonData(label, "profile:"+p)
			currentRow = append(currentRow, btn)
			if len(currentRow) == 4 {
				rows = append(rows, currentRow)
				currentRow = nil
			}
		}
		if len(currentRow) > 0 {
			rows = append(rows, currentRow)
		}
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("« Menu Utama", "nav:start"),
		))
		markup := tgbotapi.NewInlineKeyboardMarkup(rows...)
		text := fmt.Sprintf("<b>👤 PROFILE SWITCHER</b>\n\nCurrent Active: 🟢 <code>%s</code>\nSelect worker profile:", html.EscapeString(sess.ActiveProfile))
		r.editMessage(chatID, msgID, text, &markup)

	case "nav:history":
		items, _ := r.historyReader.ListRecentConversations(sess.ActiveProfile, 6)
		text := r.renderer.RenderHistory(sess.ActiveProfile, items)
		markup := r.getBackMarkup()
		r.editMessage(chatID, msgID, text, &markup)

	case "nav:ls":
		items, _ := workspace.ListDirectory(sess.ActiveWorkingDir)
		text := r.renderer.RenderFileList(sess.ActiveWorkingDir, items)
		markup := r.getBackMarkup()
		r.editMessage(chatID, msgID, text, &markup)

	case "nav:git":
		summary, _ := workspace.InspectRepository(sess.ActiveWorkingDir)
		text := r.renderer.RenderGitSummary(summary)
		markup := r.getBackMarkup()
		r.editMessage(chatID, msgID, text, &markup)

	case "nav:help":
		text := r.renderer.RenderHelp()
		markup := r.getBackMarkup()
		r.editMessage(chatID, msgID, text, &markup)
	}
}

func (r *Router) editMessage(chatID int64, msgID int, text string, markup *tgbotapi.InlineKeyboardMarkup) {
	if r.api == nil {
		return
	}
	edit := tgbotapi.NewEditMessageText(chatID, msgID, text)
	edit.ParseMode = tgbotapi.ModeHTML
	if markup != nil {
		edit.ReplyMarkup = markup
	}
	_, _ = r.api.Request(edit)
}

func (r *Router) replyHTML(chatID int64, text string) {
	if r.api == nil {
		return
	}
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeHTML
	sent, err := r.api.Send(msg)
	if err == nil && r.sessionStore != nil {
		r.sessionStore.SetLastBotMessageIDs(chatID, []int{sent.MessageID})
	}
}

func (r *Router) replyHTMLWithBack(chatID int64, text string) {
	if r.api == nil {
		return
	}
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeHTML
	markup := r.getBackMarkup()
	msg.ReplyMarkup = markup
	sent, err := r.api.Send(msg)
	if err == nil && r.sessionStore != nil {
		r.sessionStore.SetLastBotMessageIDs(chatID, []int{sent.MessageID})
	}
}

func (r *Router) replyChunks(chatID int64, text string) {
	if r.api == nil {
		return
	}
	chunks := r.renderer.ChunkMessage(text, 4000)
	var ids []int
	for _, chunk := range chunks {
		msg := tgbotapi.NewMessage(chatID, chunk)
		msg.ParseMode = tgbotapi.ModeHTML
		if sent, err := r.api.Send(msg); err == nil {
			ids = append(ids, sent.MessageID)
		}
	}
	if len(ids) > 0 && r.sessionStore != nil {
		r.sessionStore.SetLastBotMessageIDs(chatID, ids)
	}
}

func (r *Router) sendTyping(chatID int64) {
	if r.api == nil {
		return
	}
	action := tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping)
	_, _ = r.api.Send(action)
}

func (r *Router) sendDocument(chatID int64, filePath string) {
	if r.api == nil {
		return
	}
	doc := tgbotapi.NewDocument(chatID, tgbotapi.FilePath(filePath))
	if sent, err := r.api.Send(doc); err == nil && r.sessionStore != nil {
		r.sessionStore.SetLastBotMessageIDs(chatID, []int{sent.MessageID})
	}
}

func truncateString(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func (r *Router) replyHTMLWithMarkup(chatID int64, text string, markup tgbotapi.InlineKeyboardMarkup) {
	if r.api == nil {
		return
	}
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeHTML
	msg.ReplyMarkup = markup
	sent, err := r.api.Send(msg)
	if err == nil && r.sessionStore != nil {
		r.sessionStore.SetLastBotMessageIDs(chatID, []int{sent.MessageID})
	}
}

func normalizeAccount(acc string) string {
	clean := strings.ToLower(strings.TrimSpace(acc))
	if clean == "" {
		return ""
	}
	if strings.HasPrefix(clean, "agy") && len(clean) == 5 {
		return clean
	}
	var digits strings.Builder
	for _, r := range clean {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
		}
	}
	if digits.Len() > 0 {
		var num int
		if _, err := fmt.Sscanf(digits.String(), "%d", &num); err == nil && num >= 1 && num <= 15 {
			return fmt.Sprintf("agy%02d", num)
		}
	}
	return clean
}

func (r *Router) handleLoginAccount(ctx context.Context, chatID int64, account string) {
	cleanAcc := normalizeAccount(account)
	if cleanAcc == "" {
		r.replyHTML(chatID, "<b>Usage:</b> <code>/login &lt;account&gt;</code>\nExample: <code>/login agy01</code>")
		return
	}

	r.sendTyping(chatID)
	execCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	cmd := exec.CommandContext(execCtx, "python3", "/opt/data/home/agy-profiles/fast_login.py", "login", cleanAcc)
	out, err := cmd.CombinedOutput()
	if err != nil && len(out) == 0 {
		r.replyHTMLWithBack(chatID, fmt.Sprintf("<b>🔴 Login Error (%s):</b>\n<pre><code>%s</code></pre>", html.EscapeString(cleanAcc), html.EscapeString(err.Error())))
		return
	}

	outStr := strings.TrimSpace(string(out))
	var b strings.Builder
	b.WriteString(fmt.Sprintf("<b>🔑 AGY ACCOUNT LOGIN — %s</b>\n\n", strings.ToUpper(cleanAcc)))

	var authURL string
	for _, line := range strings.Split(outStr, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "https://accounts.google.com/o/oauth2/auth") {
			parts := strings.Fields(trimmed)
			if len(parts) > 0 {
				authURL = parts[0]
			}
			break
		}
	}

	if authURL != "" {
		b.WriteString(fmt.Sprintf("🔗 <a href=\"%s\"><b>Klik di Sini untuk Otorisasi Google</b></a>\n\n", authURL))
		b.WriteString(fmt.Sprintf("<b>URL Otorisasi:</b>\n<code>%s</code>\n\n", html.EscapeString(authURL)))
		b.WriteString(fmt.Sprintf("Setelah menyalin kode dari browser, jalankan di terminal:\n<code>python3 /opt/data/home/agy-profiles/fast_login.py code %s &quot;&lt;KODE_OTORISASI&gt;&quot;</code>", html.EscapeString(cleanAcc)))
	} else {
		b.WriteString(fmt.Sprintf("<pre><code>%s</code></pre>", html.EscapeString(outStr)))
	}

	r.replyHTMLWithBack(chatID, b.String())
}

func (r *Router) handleLogoutAccount(ctx context.Context, chatID int64, account string) {
	cleanAcc := normalizeAccount(account)
	if cleanAcc == "" {
		r.replyHTML(chatID, "<b>Usage:</b> <code>/logout &lt;account&gt;</code>\nExample: <code>/logout agy01</code>")
		return
	}

	r.sendTyping(chatID)
	execCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(execCtx, "python3", "/opt/data/home/agy-profiles/fast_login.py", "logout", cleanAcc)
	out, err := cmd.CombinedOutput()
	if err != nil && len(out) == 0 {
		r.replyHTMLWithBack(chatID, fmt.Sprintf("<b>🔴 Logout Error (%s):</b>\n<pre><code>%s</code></pre>", html.EscapeString(cleanAcc), html.EscapeString(err.Error())))
		return
	}

	outStr := strings.TrimSpace(string(out))
	var b strings.Builder
	b.WriteString(fmt.Sprintf("<b>🔒 AGY ACCOUNT LOGOUT — %s</b>\n\n", strings.ToUpper(cleanAcc)))
	b.WriteString(fmt.Sprintf("<pre><code>%s</code></pre>", html.EscapeString(outStr)))

	r.replyHTMLWithBack(chatID, b.String())
}

func (r *Router) handleSwitch(chatID int64, sess *session.Session, targetProfile string) {
	clean := normalizeAccount(targetProfile)
	if clean == "" {
		r.replyHTML(chatID, "<b>Usage:</b> <code>/switch &lt;account&gt;</code>\nExample: <code>/switch agy02</code>")
		return
	}

	if !r.profileManager.ValidateProfile(clean) {
		r.replyHTML(chatID, fmt.Sprintf("<b>🔴 Error:</b> Profile <code>%s</code> is not valid or directory not found.", html.EscapeString(clean)))
		return
	}

	oldProfile := sess.ActiveProfile
	r.sessionStore.SetProfile(chatID, clean)
	r.replyHTMLWithBack(chatID, r.renderer.RenderProfileSwitched(oldProfile, clean))
}

func (r *Router) handleRefreshAccount(ctx context.Context, chatID int64, account string) {
	r.handleUp(ctx, chatID, account)
}

func (r *Router) handleSend(ctx context.Context, chatID int64, args string) {
	parts := strings.SplitN(strings.TrimSpace(args), " ", 2)
	if len(parts) < 2 || parts[0] == "" || strings.TrimSpace(parts[1]) == "" {
		r.replyHTML(chatID, "<b>Usage:</b> <code>/send &lt;account&gt; &lt;prompt&gt;</code>\nExample: <code>/send agy02 Cek status git</code>")
		return
	}

	targetAcc := normalizeAccount(parts[0])
	promptText := strings.TrimSpace(parts[1])

	r.sendTyping(chatID)

	dispatchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(dispatchCtx, "python3", "/opt/data/home/agy-profiles/orchestrator.py", "dispatch", targetAcc, promptText, "--workspace", "/opt/data/home/AgyRemoteTele")
	out, err := cmd.CombinedOutput()
	outStr := string(out)

	if err != nil && len(out) == 0 {
		r.replyHTMLWithBack(chatID, fmt.Sprintf("<b>🔴 Dispatch Failed:</b>\n<pre><code>%s</code></pre>", html.EscapeString(err.Error())))
		return
	}

	var taskID string
	for _, line := range strings.Split(outStr, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, "Task ID") {
			subParts := strings.Split(trimmed, ":")
			if len(subParts) >= 2 {
				taskID = strings.TrimSpace(subParts[1])
				break
			}
		}
	}

	if taskID == "" {
		re := regexp.MustCompile(`(agy\d+_\d+)`)
		m := re.FindStringSubmatch(outStr)
		if len(m) > 1 {
			taskID = m[1]
		}
	}

	if taskID == "" {
		r.replyHTMLWithBack(chatID, fmt.Sprintf("<b>🔴 Task Dispatch Output:</b>\n<pre><code>%s</code></pre>", html.EscapeString(outStr)))
		return
	}

	dispatchMsg := fmt.Sprintf("🚀 <b>task dispatched %s</b>\n\n• <b>Worker:</b> <code>%s</code>\n• <b>Workspace:</b> <code>/opt/data/home/AgyRemoteTele</code>\n• <b>Prompt:</b> <i>%s</i>\n\n<i>Monitoring execution in background (max 30m)...</i>",
		html.EscapeString(taskID), html.EscapeString(targetAcc), html.EscapeString(truncateString(promptText, 80)))
	r.replyHTML(chatID, dispatchMsg)

	go r.monitorTask(chatID, taskID, targetAcc)
}

func (r *Router) monitorTask(chatID int64, taskID string, targetAcc string) {
	timeoutCtx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	ticker := time.NewTicker(8 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeoutCtx.Done():
			r.replyHTML(chatID, fmt.Sprintf("<b>⏱ TASK TIMEOUT (30m)</b>\n\nTask <code>%s</code> on worker <code>%s</code> reached maximum execution duration.", html.EscapeString(taskID), html.EscapeString(targetAcc)))
			return
		case <-ticker.C:
			checkCtx, checkCancel := context.WithTimeout(timeoutCtx, 15*time.Second)
			cmd := exec.CommandContext(checkCtx, "python3", "/opt/data/home/agy-profiles/orchestrator.py", "status", "--task-id", taskID)
			out, err := cmd.CombinedOutput()
			checkCancel()

			if err != nil && len(out) == 0 {
				continue
			}

			outStr := string(out)
			var status string
			for _, line := range strings.Split(outStr, "\n") {
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(trimmed, "Status") {
					subParts := strings.Split(trimmed, ":")
					if len(subParts) >= 2 {
						status = strings.TrimSpace(subParts[1])
						break
					}
				}
			}

			if status == "FINISHED" || status == "FAILED" {
				var finalOutput strings.Builder
				badge := "✅"
				if status == "FAILED" {
					badge = "❌"
				}
				finalOutput.WriteString(fmt.Sprintf("%s <b>TASK %s — %s</b>\n", badge, html.EscapeString(status), html.EscapeString(taskID)))
				finalOutput.WriteString(fmt.Sprintf("• <b>Worker:</b> <code>%s</code>\n\n", html.EscapeString(targetAcc)))

				logPath := fmt.Sprintf("/opt/data/home/agy-profiles/orchestrator_tasks/%s.log", taskID)
				if logBytes, readErr := os.ReadFile(logPath); readErr == nil && len(logBytes) > 0 {
					lines := strings.Split(string(logBytes), "\n")
					start := 0
					if len(lines) > 40 {
						start = len(lines) - 40
					}
					snippet := strings.Join(lines[start:], "\n")
					finalOutput.WriteString("<b>Output Terakhir:</b>\n<pre><code>")
					finalOutput.WriteString(html.EscapeString(snippet))
					finalOutput.WriteString("</code></pre>")
				} else {
					finalOutput.WriteString("<pre><code>")
					finalOutput.WriteString(html.EscapeString(outStr))
					finalOutput.WriteString("</code></pre>")
				}

				r.replyChunks(chatID, finalOutput.String())
				return
			}
		}
	}
}

func (r *Router) handleTasks(ctx context.Context, chatID int64) {
	r.sendTyping(chatID)

	execCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(execCtx, "python3", "/opt/data/home/agy-profiles/orchestrator.py", "status")
	out, err := cmd.CombinedOutput()
	if err != nil && len(out) == 0 {
		r.replyHTMLWithBack(chatID, fmt.Sprintf("<b>🔴 Tasks Error:</b>\n<pre><code>%s</code></pre>", html.EscapeString(err.Error())))
		return
	}

	lines := strings.Split(string(out), "\n")
	type taskEntry struct {
		taskID string
		worker string
		status string
		prompt string
	}

	var tasks []taskEntry
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.Contains(trimmed, "|") {
			continue
		}
		parts := strings.Split(trimmed, "|")
		if len(parts) != 4 {
			continue
		}
		tid := strings.TrimSpace(parts[0])
		if strings.EqualFold(tid, "TASK ID") || strings.HasPrefix(tid, "-") || strings.HasPrefix(tid, "=") {
			continue
		}
		tasks = append(tasks, taskEntry{
			taskID: tid,
			worker: strings.TrimSpace(parts[1]),
			status: strings.TrimSpace(parts[2]),
			prompt: strings.TrimSpace(parts[3]),
		})
	}

	var b strings.Builder
	b.WriteString("<b>📋 ORCHESTRATOR TASKS & WORKERS</b>\n\n")

	if len(tasks) == 0 {
		b.WriteString(fmt.Sprintf("<pre><code>%s</code></pre>", html.EscapeString(string(out))))
	} else {
		for _, t := range tasks {
			badge := "⚪"
			if t.status == "RUNNING" {
				badge = "🟢"
			} else if t.status == "FAILED" {
				badge = "🔴"
			}
			b.WriteString(fmt.Sprintf("%s <b>%s</b> • <code>%s</code> (<b>%s</b>)\n", badge, html.EscapeString(t.worker), html.EscapeString(t.taskID), html.EscapeString(t.status)))
			b.WriteString(fmt.Sprintf("  └ <i>%s</i>\n\n", html.EscapeString(truncateString(t.prompt, 50))))
		}
	}

	r.replyHTMLWithBack(chatID, strings.TrimRight(b.String(), "\n"))
}

func (r *Router) handleWorkers(ctx context.Context, chatID int64) {
	r.sendTyping(chatID)

	execCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(execCtx, "python3", "/opt/data/home/agy-profiles/orchestrator.py", "workers")
	out, err := cmd.CombinedOutput()
	if err != nil && len(out) == 0 {
		r.replyHTMLWithBack(chatID, fmt.Sprintf("<b>🔴 Workers Error:</b>\n<pre><code>%s</code></pre>", html.EscapeString(err.Error())))
		return
	}

	lines := strings.Split(string(out), "\n")
	type workerInfo struct {
		profile  string
		remoteID string
		auth     string
		daemon   string
		email    string
	}

	var workers []workerInfo
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.Contains(trimmed, "|") {
			continue
		}
		parts := strings.Split(trimmed, "|")
		if len(parts) != 5 {
			continue
		}
		p := strings.TrimSpace(parts[0])
		if !strings.HasPrefix(strings.ToLower(p), "agy") {
			continue
		}
		workers = append(workers, workerInfo{
			profile:  p,
			remoteID: strings.TrimSpace(parts[1]),
			auth:     strings.TrimSpace(parts[2]),
			daemon:   strings.TrimSpace(parts[3]),
			email:    strings.TrimSpace(parts[4]),
		})
	}

	var b strings.Builder
	b.WriteString("<b>🤖 AGY WORKERS DAEMON STATUS (01 - 10)</b>\n\n")

	if len(workers) == 0 {
		b.WriteString(fmt.Sprintf("<pre><code>%s</code></pre>", html.EscapeString(string(out))))
	} else {
		for _, w := range workers {
			badge := "⚪"
			if strings.Contains(w.daemon, "PID") {
				badge = "🟢"
			} else if strings.Contains(strings.ToLower(w.auth), "standby") || strings.Contains(strings.ToLower(w.auth), "siap aktif") {
				badge = "🟡"
			}
			b.WriteString(fmt.Sprintf("%s <b>%s</b> • <code>%s</code>\n", badge, html.EscapeString(w.profile), html.EscapeString(w.remoteID)))
			b.WriteString(fmt.Sprintf("  ├ 🔐 Auth: %s\n", html.EscapeString(w.auth)))
			b.WriteString(fmt.Sprintf("  ├ ⚙️ Daemon: %s\n", html.EscapeString(w.daemon)))
			b.WriteString(fmt.Sprintf("  └ 📧 Email: <code>%s</code>\n\n", html.EscapeString(w.email)))
		}
	}

	markup := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⚡ Hidupkan Semua", "nav:up_all"),
			tgbotapi.NewInlineKeyboardButtonData("👥 Akun Worker", "nav:accounts"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("« Menu Utama", "nav:start"),
		),
	)
	r.replyHTMLWithMarkup(chatID, strings.TrimRight(b.String(), "\n"), markup)
}

func (r *Router) handleUsage(ctx context.Context, chatID int64) {
	r.sendTyping(chatID)

	type workerUsage struct {
		Account        string
		TokenRemaining string
		ActiveSession  string
		FinishedTasks  int
	}

	usageMap := make(map[string]*workerUsage)
	for i := 1; i <= 10; i++ {
		name := fmt.Sprintf("agy%02d", i)
		usageMap[name] = &workerUsage{
			Account:        name,
			TokenRemaining: "-",
			ActiveSession:  "⚪ Mati",
			FinishedTasks:  0,
		}
	}

	tasksDBPath := "/opt/data/home/agy-profiles/orchestrator_tasks/tasks.json"
	if data, err := os.ReadFile(tasksDBPath); err == nil {
		type taskItem struct {
			Account string `json:"account"`
			Status  string `json:"status"`
		}
		var rawTasks map[string]taskItem
		if err := json.Unmarshal(data, &rawTasks); err == nil {
			for _, t := range rawTasks {
				if t.Status == "FINISHED" {
					normAcc := normalizeAccount(t.Account)
					if u, ok := usageMap[normAcc]; ok {
						u.FinishedTasks++
					}
				}
			}
		}
	}

	execCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmdStat := exec.CommandContext(execCtx, "python3", "/opt/data/home/agy-profiles/fast_login.py", "status")
	if outStat, err := cmdStat.CombinedOutput(); err == nil {
		for _, line := range strings.Split(string(outStat), "\n") {
			trimmed := strings.TrimSpace(line)
			if !strings.Contains(trimmed, "|") {
				continue
			}
			parts := strings.Split(trimmed, "|")
			if len(parts) != 4 {
				continue
			}
			accName := normalizeAccount(parts[0])
			u, exists := usageMap[accName]
			if !exists {
				continue
			}
			status := strings.TrimSpace(parts[1])
			remaining := strings.TrimSpace(parts[2])
			if strings.EqualFold(status, "Aktif") {
				remClean := strings.ReplaceAll(remaining, " menit ", "m ")
				remClean = strings.ReplaceAll(remClean, " detik", "s")
				u.TokenRemaining = remClean
			} else if strings.EqualFold(status, "Belum Login") {
				u.TokenRemaining = "No Login"
			} else {
				u.TokenRemaining = status
			}
		}
	}

	cmdWork := exec.CommandContext(execCtx, "python3", "/opt/data/home/agy-profiles/orchestrator.py", "workers")
	if outWork, err := cmdWork.CombinedOutput(); err == nil {
		for _, line := range strings.Split(string(outWork), "\n") {
			trimmed := strings.TrimSpace(line)
			if !strings.Contains(trimmed, "|") {
				continue
			}
			parts := strings.Split(trimmed, "|")
			if len(parts) != 5 {
				continue
			}
			accName := normalizeAccount(parts[0])
			u, exists := usageMap[accName]
			if !exists {
				continue
			}
			daemonStr := strings.TrimSpace(parts[3])
			if strings.Contains(daemonStr, "PID") {
				u.ActiveSession = "🟢 Aktif"
			} else {
				u.ActiveSession = "⚪ Mati"
			}
		}
	}

	totalTasks := 0
	activeTokens := 0
	activeDaemons := 0

	var b strings.Builder
	b.WriteString("<b>📊 AGY CLUSTER USAGE MONITOR</b>\n\n")
	b.WriteString("<pre>")
	b.WriteString("AKUN   | TOKEN SISA   | SESI AKTIF | SELESAI\n")
	b.WriteString("-------+--------------+------------+--------\n")
	for i := 1; i <= 10; i++ {
		name := fmt.Sprintf("agy%02d", i)
		u := usageMap[name]
		totalTasks += u.FinishedTasks
		if strings.Contains(u.TokenRemaining, "m") {
			activeTokens++
		}
		if strings.Contains(u.ActiveSession, "🟢") {
			activeDaemons++
		}
		rem := u.TokenRemaining
		if len(rem) > 12 {
			rem = rem[:12]
		}
		b.WriteString(fmt.Sprintf("%-6s | %-12s | %-10s | %6d\n", u.Account, rem, u.ActiveSession, u.FinishedTasks))
	}
	b.WriteString("-------+--------------+------------+--------\n")
	b.WriteString(fmt.Sprintf("%-6s | %-12s | %-10s | %6d\n", "TOTAL", fmt.Sprintf("%d/10 Aktif", activeTokens), fmt.Sprintf("%d Daemon", activeDaemons), totalTasks))
	b.WriteString("</pre>\n\n")

	b.WriteString(fmt.Sprintf("• <b>Total Task Selesai:</b> <code>%d</code>\n", totalTasks))
	b.WriteString(fmt.Sprintf("• <b>Worker Token Aktif:</b> <code>%d/10</code>\n", activeTokens))
	b.WriteString(fmt.Sprintf("• <b>Daemon Aktif:</b> <code>%d/10</code>", activeDaemons))

	r.replyHTMLWithBack(chatID, b.String())
}

func (r *Router) handleProcesses(chatID int64, sess *session.Session, args string) {
	topN := 15
	sortBy := "cpu"
	parts := strings.Fields(args)
	if len(parts) > 0 {
		if n, err := strconv.Atoi(parts[0]); err == nil && n > 0 {
			topN = n
			if topN > 50 {
				topN = 50
			}
		} else {
			sortBy = strings.ToLower(parts[0])
		}
	}
	if len(parts) > 1 {
		sortBy = strings.ToLower(parts[1])
	}
	procs, err := system.ListProcesses(topN, sortBy)
	if err != nil {
		r.replyHTML(chatID, fmt.Sprintf("<b>🔴 Process List Error:</b> %s", html.EscapeString(err.Error())))
		return
	}
	r.replyHTMLWithBack(chatID, r.renderer.RenderProcesses(procs))
}

func (r *Router) handleKill(chatID int64, args string) {
	parts := strings.Fields(args)
	if len(parts) == 0 {
		r.replyHTML(chatID, "<b>Usage:</b> <code>/kill &lt;pid&gt; [signal]</code>\nExample: <code>/kill 12345 9</code> or <code>/kill 12345 SIGTERM</code>")
		return
	}
	pid, err := strconv.Atoi(parts[0])
	if err != nil || pid <= 0 {
		r.replyHTML(chatID, "<b>🔴 Invalid PID:</b> Must be a positive integer.")
		return
	}
	sig := syscall.SIGTERM
	if len(parts) > 1 {
		sStr := strings.ToUpper(parts[1])
		switch sStr {
		case "9", "SIGKILL", "KILL":
			sig = syscall.SIGKILL
		case "15", "SIGTERM", "TERM":
			sig = syscall.SIGTERM
		case "1", "SIGHUP", "HUP":
			sig = syscall.SIGHUP
		case "2", "SIGINT", "INT":
			sig = syscall.SIGINT
		default:
			if n, err := strconv.Atoi(sStr); err == nil {
				sig = syscall.Signal(n)
			}
		}
	}
	if err := system.KillProcess(pid, sig); err != nil {
		r.replyHTML(chatID, fmt.Sprintf("<b>🔴 Kill Error:</b> %s", html.EscapeString(err.Error())))
		return
	}
	r.replyHTML(chatID, fmt.Sprintf("<b>🟢 Process Terminated:</b> PID <code>%d</code> sent signal <code>%d</code>.", pid, sig))
}

func (r *Router) handlePorts(chatID int64) {
	ports, err := system.ListListeningPorts()
	if err != nil {
		r.replyHTML(chatID, fmt.Sprintf("<b>🔴 Ports Error:</b> %s", html.EscapeString(err.Error())))
		return
	}
	r.replyHTMLWithBack(chatID, r.renderer.RenderListeningPorts(ports))
}

func (r *Router) handleServices(ctx context.Context, chatID int64, args string) {
	parts := strings.Fields(args)
	subcmd := "list"
	serviceName := ""
	if len(parts) > 0 {
		subcmd = strings.ToLower(parts[0])
	}
	if len(parts) > 1 {
		serviceName = parts[1]
	}

	switch subcmd {
	case "restart":
		if serviceName == "" {
			r.replyHTML(chatID, "<b>Usage:</b> <code>/service restart &lt;service_name&gt;</code>\nAllowed services: docker, nginx, cloudflared, cron")
			return
		}
		out, err := system.RestartService(ctx, serviceName)
		if err != nil {
			r.replyHTML(chatID, fmt.Sprintf("<b>🔴 Service Restart Error:</b> %s", html.EscapeString(err.Error())))
			return
		}
		r.replyHTML(chatID, fmt.Sprintf("<b>🟢 Service Restarted:</b> <code>%s</code>\n<pre><code>%s</code></pre>", html.EscapeString(serviceName), html.EscapeString(out)))
	case "list", "status":
		fallthrough
	default:
		services, err := system.ListCommonServices(ctx)
		if err != nil {
			r.replyHTML(chatID, fmt.Sprintf("<b>🔴 Service Error:</b> %s", html.EscapeString(err.Error())))
			return
		}
		r.replyHTMLWithBack(chatID, r.renderer.RenderServices(services))
	}
}

func (r *Router) handlePing(ctx context.Context, chatID int64, args string) {
	host := strings.TrimSpace(args)
	if host == "" {
		r.replyHTML(chatID, "<b>Usage:</b> <code>/ping &lt;host&gt;</code>\nExample: <code>/ping 1.1.1.1</code> or <code>/ping google.com</code>")
		return
	}
	res, err := nettools.Ping(ctx, host, 3)
	if err != nil {
		r.replyHTML(chatID, fmt.Sprintf("<b>🔴 Ping Error:</b> %s", html.EscapeString(err.Error())))
		return
	}
	r.replyHTMLWithBack(chatID, r.renderer.RenderPing(res))
}

func (r *Router) handleHTTP(ctx context.Context, chatID int64, args string) {
	targetURL := strings.TrimSpace(args)
	if targetURL == "" {
		r.replyHTML(chatID, "<b>Usage:</b> <code>/curl &lt;url&gt;</code> or <code>/http &lt;url&gt;</code>\nExample: <code>/curl https://google.com</code>")
		return
	}
	if !strings.HasPrefix(targetURL, "http://") && !strings.HasPrefix(targetURL, "https://") {
		targetURL = "https://" + targetURL
	}
	res, err := nettools.ProbeHTTP(ctx, targetURL, 15*time.Second)
	if err != nil {
		r.replyHTML(chatID, fmt.Sprintf("<b>🔴 HTTP Probe Error:</b> %s", html.EscapeString(err.Error())))
		return
	}
	r.replyHTMLWithBack(chatID, r.renderer.RenderHTTPProbe(res))
}

func (r *Router) handleDNS(ctx context.Context, chatID int64, args string) {
	parts := strings.Fields(args)
	if len(parts) == 0 {
		r.replyHTML(chatID, "<b>Usage:</b> <code>/dns &lt;domain&gt; [type]</code>\nExample: <code>/dns google.com A</code> or <code>/dns cloudflare.com MX</code>")
		return
	}
	domain := parts[0]
	recType := "A"
	if len(parts) > 1 {
		recType = strings.ToUpper(parts[1])
	}
	recs, err := nettools.LookupDNS(ctx, domain, recType)
	if err != nil {
		r.replyHTML(chatID, fmt.Sprintf("<b>🔴 DNS Error:</b> %s", html.EscapeString(err.Error())))
		return
	}
	r.replyHTMLWithBack(chatID, r.renderer.RenderDNS(domain, recs))
}

func (r *Router) handleNetIO(chatID int64) {
	stats, err := nettools.GetNetworkIO()
	if err != nil {
		r.replyHTML(chatID, fmt.Sprintf("<b>🔴 Net I/O Error:</b> %s", html.EscapeString(err.Error())))
		return
	}
	r.replyHTMLWithBack(chatID, r.renderer.RenderNetworkIO(stats))
}

func (r *Router) handleDocker(ctx context.Context, chatID int64, args string) {
	parts := strings.Fields(args)
	subcmd := "ps"
	if len(parts) > 0 {
		subcmd = strings.ToLower(parts[0])
	}

	switch subcmd {
	case "ps":
		containers, err := docker.ListContainers(ctx, true)
		if err != nil {
			r.replyHTML(chatID, fmt.Sprintf("<b>🔴 Docker PS Error:</b> %s", html.EscapeString(err.Error())))
			return
		}
		r.replyHTMLWithBack(chatID, r.renderer.RenderContainers(containers))

	case "stats":
		stats, err := docker.GetContainerStats(ctx)
		if err != nil {
			r.replyHTML(chatID, fmt.Sprintf("<b>🔴 Docker Stats Error:</b> %s", html.EscapeString(err.Error())))
			return
		}
		r.replyHTMLWithBack(chatID, r.renderer.RenderContainerStats(stats))

	case "logs":
		if len(parts) < 2 {
			r.replyHTML(chatID, "<b>Usage:</b> <code>/docker logs &lt;container_id&gt; [lines]</code>")
			return
		}
		cid := parts[1]
		tail := 50
		if len(parts) > 2 {
			if n, err := strconv.Atoi(parts[2]); err == nil && n > 0 {
				tail = n
			}
		}
		logs, err := docker.GetContainerLogs(ctx, cid, tail)
		if err != nil {
			r.replyHTML(chatID, fmt.Sprintf("<b>🔴 Docker Logs Error:</b> %s", html.EscapeString(err.Error())))
			return
		}
		if logs == "" {
			logs = "(no log output available)"
		}
		text := fmt.Sprintf("<b>🐳 DOCKER LOGS:</b> <code>%s</code>\n<pre><code>%s</code></pre>", html.EscapeString(cid), html.EscapeString(logs))
		if len(text) > 4000 {
			r.replyChunks(chatID, text)
		} else {
			r.replyHTMLWithBack(chatID, text)
		}

	case "restart":
		if len(parts) < 2 {
			r.replyHTML(chatID, "<b>Usage:</b> <code>/docker restart &lt;container_id&gt;</code>")
			return
		}
		cid := parts[1]
		if err := docker.RestartContainer(ctx, cid); err != nil {
			r.replyHTML(chatID, fmt.Sprintf("<b>🔴 Docker Restart Error:</b> %s", html.EscapeString(err.Error())))
			return
		}
		r.replyHTML(chatID, fmt.Sprintf("<b>🟢 Container Restarted:</b> <code>%s</code>", html.EscapeString(cid)))

	default:
		r.replyHTML(chatID, "<b>Usage:</b> <code>/docker [ps|logs|restart|stats]</code>")
	}
}

func (r *Router) handleCron(ctx context.Context, chatID int64, args string) {
	if r.scheduler == nil {
		r.replyHTML(chatID, "<b>⚠️ Scheduler Error:</b> Scheduler service is not initialized.")
		return
	}

	parts := strings.Fields(args)
	subcmd := "list"
	if len(parts) > 0 {
		subcmd = strings.ToLower(parts[0])
	}

	switch subcmd {
	case "list":
		jobs := r.scheduler.ListJobs()
		r.replyHTMLWithBack(chatID, r.renderer.RenderCronJobs(jobs))

	case "add":
		if len(parts) < 3 {
			r.replyHTML(chatID, "<b>Usage:</b>\n• <code>/cron add @every &lt;interval&gt; &lt;cmd&gt;</code>\n• <code>/cron add &lt;m&gt; &lt;h&gt; &lt;dom&gt; &lt;mon&gt; &lt;dow&gt; &lt;cmd&gt;</code>\nExample: <code>/cron add @every 1h /status</code>")
			return
		}
		var spec string
		var cmdStr string
		if strings.HasPrefix(parts[1], "@every") {
			if len(parts) < 4 {
				r.replyHTML(chatID, "<b>Usage:</b> <code>/cron add @every &lt;interval&gt; &lt;cmd&gt;</code>\nExample: <code>/cron add @every 30m /status</code>")
				return
			}
			spec = parts[1] + " " + parts[2]
			cmdStr = strings.Join(parts[3:], " ")
		} else if strings.HasPrefix(parts[1], "@") {
			spec = parts[1]
			cmdStr = strings.Join(parts[2:], " ")
		} else {
			if len(parts) < 7 {
				r.replyHTML(chatID, "<b>Usage:</b> <code>/cron add &lt;m&gt; &lt;h&gt; &lt;dom&gt; &lt;mon&gt; &lt;dow&gt; &lt;cmd&gt;</code>")
				return
			}
			spec = strings.Join(parts[1:6], " ")
			cmdStr = strings.Join(parts[6:], " ")
		}

		job, err := r.scheduler.AddJob(spec, cmdStr, chatID)
		if err != nil {
			r.replyHTML(chatID, fmt.Sprintf("<b>🔴 Cron Add Error:</b> %s", html.EscapeString(err.Error())))
			return
		}
		_ = r.scheduler.Save()
		r.replyHTML(chatID, fmt.Sprintf("<b>🟢 Cron Job Scheduled:</b>\n• ID: <code>%s</code>\n• Spec: <code>%s</code>\n• Command: <code>%s</code>\n• Next Run: <code>%s</code>",
			job.ID, html.EscapeString(job.Spec), html.EscapeString(job.Command), job.NextRun.Format("2006-01-02 15:04:05")))

	case "remove", "rm", "delete":
		if len(parts) < 2 {
			r.replyHTML(chatID, "<b>Usage:</b> <code>/cron remove &lt;job_id&gt;</code>")
			return
		}
		r.handleCronRemove(chatID, parts[1])

	case "run":
		if len(parts) < 2 {
			r.replyHTML(chatID, "<b>Usage:</b> <code>/cron run &lt;job_id&gt;</code>")
			return
		}
		r.handleCronRun(ctx, chatID, parts[1])

	default:
		r.replyHTML(chatID, "<b>Usage:</b> <code>/cron [list|add|remove|run]</code>")
	}
}

func (r *Router) handleCronRun(ctx context.Context, chatID int64, jobID string) {
	if r.scheduler == nil {
		r.replyHTML(chatID, "<b>⚠️ Scheduler Error:</b> Scheduler service is not initialized.")
		return
	}
	if err := r.scheduler.RunJobNow(jobID); err != nil {
		r.replyHTML(chatID, fmt.Sprintf("<b>🔴 Cron Run Error:</b> %s", html.EscapeString(err.Error())))
		return
	}
	r.replyHTML(chatID, fmt.Sprintf("<b>🟢 Cron Job Executed:</b> <code>%s</code>", html.EscapeString(jobID)))
}

func (r *Router) handleCronRemove(chatID int64, jobID string) {
	if r.scheduler == nil {
		r.replyHTML(chatID, "<b>⚠️ Scheduler Error:</b> Scheduler service is not initialized.")
		return
	}
	if !r.scheduler.RemoveJob(jobID) {
		r.replyHTML(chatID, fmt.Sprintf("<b>🔴 Cron Remove Error:</b> Job <code>%s</code> not found.", html.EscapeString(jobID)))
		return
	}
	_ = r.scheduler.Save()
	r.replyHTML(chatID, fmt.Sprintf("<b>🟢 Cron Job Removed:</b> <code>%s</code>", html.EscapeString(jobID)))
}

func (r *Router) handleAlerts(chatID int64) {
	var alerts []watchdog.AlertEvent
	if r.watchdog != nil {
		alerts = r.watchdog.GetRecentAlerts()
		if len(alerts) == 0 {
			alerts = r.watchdog.CheckNow()
		}
	}
	r.replyHTMLWithBack(chatID, r.renderer.RenderAlerts(alerts))
}

func (r *Router) handleBackup(chatID int64, sess *session.Session, args string) {
	targetDir := strings.TrimSpace(args)
	if targetDir == "" {
		if sess != nil && sess.ActiveWorkingDir != "" {
			targetDir = sess.ActiveWorkingDir
		} else {
			targetDir = "/opt/data/home/AgyRemoteTele"
		}
	}

	backupDir := "/opt/data/home/AgyRemoteTele/data/backups"
	_ = os.MkdirAll(backupDir, 0755)
	destArchive := filepath.Join(backupDir, fmt.Sprintf("backup_%s.tar.gz", time.Now().Format("20060102_150405")))

	excludes := []string{
		"*.log",
		".git",
		"node_modules",
		".cache",
		"tmp",
		"data/backups",
	}

	res, err := backup.CreateTarGz([]string{targetDir}, destArchive, excludes)
	if err != nil {
		r.replyHTML(chatID, fmt.Sprintf("<b>🔴 Backup Error:</b> %s", html.EscapeString(err.Error())))
		return
	}

	summary := r.renderer.RenderBackup(res)
	r.replyHTML(chatID, summary)

	if res.TotalSizeBytes < 50*1024*1024 && r.api != nil {
		doc := tgbotapi.NewDocument(chatID, tgbotapi.FilePath(res.ArchivePath))
		doc.Caption = fmt.Sprintf("📦 Backup archive (%s, %d files)", filepath.Base(targetDir), res.FileCount)
		sent, err := r.api.Send(doc)
		if err == nil && r.sessionStore != nil {
			r.sessionStore.SetLastBotMessageIDs(chatID, []int{sent.MessageID})
		}
	}
}

func (r *Router) handleSwarm(ctx context.Context, chatID int64, sess *session.Session, args string) {
	prompt := strings.TrimSpace(args)
	if prompt == "" {
		r.replyHTML(chatID, "<b>Usage:</b> <code>/swarm &lt;prompt&gt;</code>\nExample: <code>/swarm git status check</code>")
		return
	}

	if r.swarmManager == nil {
		r.swarmManager = cluster.NewSwarmManager("", "")
	}

	workDir := ""
	if sess != nil {
		workDir = sess.ActiveWorkingDir
	}

	r.replyHTML(chatID, fmt.Sprintf("<b>🐝 SWARM BROADCAST DISPATCHED</b>\n\nPrompt: <i>%s</i>\nBroadcasting to workers...", html.EscapeString(prompt)))

	results, err := r.swarmManager.BroadcastPrompt(ctx, nil, prompt, workDir)
	if err != nil {
		r.replyHTML(chatID, fmt.Sprintf("<b>🔴 Swarm Broadcast Error:</b> %s", html.EscapeString(err.Error())))
		return
	}

	r.replyHTMLWithBack(chatID, r.renderer.RenderSwarm(results))
}
