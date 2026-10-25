package bot

import (
	"context"
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/dresar/AgyRemoteTele/internal/agy"
	"github.com/dresar/AgyRemoteTele/internal/reporter"
	"github.com/dresar/AgyRemoteTele/internal/session"
	"github.com/dresar/AgyRemoteTele/internal/system"
	"github.com/dresar/AgyRemoteTele/internal/workspace"
)

type Router struct {
	api            *tgbotapi.BotAPI
	authManager    *session.AuthManager
	sessionStore   *session.Store
	profileManager *agy.ProfileManager
	historyReader  *agy.HistoryReader
	executor       *agy.Executor
	renderer       *Renderer
	middleware     *Middleware
	execTimeout    time.Duration
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
		api:            api,
		authManager:    authManager,
		sessionStore:   sessionStore,
		profileManager: profileManager,
		historyReader:  historyReader,
		executor:       executor,
		renderer:       renderer,
		middleware:     middleware,
		execTimeout:    execTimeout,
	}
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

	r.cleanupPreviousMessages(chatID, sess)

	if strings.HasPrefix(text, "/resume_") {
		convID := strings.TrimPrefix(text, "/resume_")
		r.handleResume(chatID, sess, convID)
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
	case "login":
		r.handleLogin(chatID, text)
	case "logout":
		r.handleLogout(chatID)
	case "ask":
		r.handleAsk(ctx, chatID, sess, args)
	case "profile":
		r.handleProfile(chatID, sess, args)
	case "accounts", "workers":
		r.handleAccounts(ctx, chatID)
	case "refresh":
		r.handleRefresh(ctx, chatID)
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
			r.handleAsk(ctx, chatID, sess, text)
		} else if msg.IsCommand() {
			r.replyHTML(chatID, fmt.Sprintf("<b>❓ Unknown Command:</b> <code>/%s</code>\nType <code>/help</code> for available commands.", html.EscapeString(cmd)))
		}
	}
}

func (r *Router) cleanupPreviousMessages(chatID int64, sess *session.Session) {
	if sess == nil || len(sess.LastBotMessageIDs) == 0 {
		return
	}
	for _, msgID := range sess.LastBotMessageIDs {
		del := tgbotapi.NewDeleteMessage(chatID, msgID)
		_, _ = r.api.Request(del)
	}
	r.sessionStore.ClearLastBotMessageIDs(chatID)
}

func (r *Router) getStartMenuMarkup() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⚡ Status", "nav:status"),
			tgbotapi.NewInlineKeyboardButtonData("👤 Profiles", "nav:profile"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("👥 Accounts", "nav:accounts"),
			tgbotapi.NewInlineKeyboardButtonData("🔄 Refresh", "nav:refresh"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📜 History", "nav:history"),
			tgbotapi.NewInlineKeyboardButtonData("📁 Workspace", "nav:ls"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🌿 Git", "nav:git"),
			tgbotapi.NewInlineKeyboardButtonData("❓ Help", "nav:help"),
		),
	)
}

func (r *Router) getBackMarkup() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("« Menu Utama", "nav:start"),
		),
	)
}

func (r *Router) handleStart(chatID int64, sess *session.Session) {
	text := r.renderer.RenderWelcome(r.api.Self.UserName, sess.ActiveProfile, sess)
	keyboard := r.getStartMenuMarkup()

	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeHTML
	msg.ReplyMarkup = keyboard
	sent, err := r.api.Send(msg)
	if err == nil {
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

	wrappedPrompt := fmt.Sprintf(`[TELEGRAM AGY REMOTE GATEWAY — OPERATOR REQUEST]
Pengirim: Telegram Operator (Chat ID: %d)
Worker Profil: %s
Working Directory: %s

INSTRUKSI PENGGUNA:
%s

STANDAR OPERASIONAL (ZERO TOLERANCE):
1. Pahami instruksi dan eksekusi tindakan yang diperlukan secara presisi.
2. STANDAR NOKOMEN: Jangan menulis komentar dalam kode yang dibuat/diedit.
3. QUALITY GATE: Pastikan kode bebas dari error kompilasi/sintaks.
4. FORMAT RESPON TELEGRAM: Berikan jawaban akhir yang SINGKAT, PADAT, dan TO-THE-POINT mengenai apa yang telah dikerjakan, status perubahan, dan verifikasi. Hindari narasi panjang bertele-tele.`,
		chatID, sess.ActiveProfile, sess.ActiveWorkingDir, cleanPrompt)

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

func (r *Router) handleAccounts(ctx context.Context, chatID int64) {
	r.sendTyping(chatID)

	execCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(execCtx, "python3", "/opt/data/home/agy-profiles/fast_login.py", "status")
	out, err := cmd.CombinedOutput()
	if err != nil && len(out) == 0 {
		r.replyHTMLWithBack(chatID, fmt.Sprintf("<b>🔴 Accounts Status Error:</b>\n<pre><code>%s</code></pre>", html.EscapeString(err.Error())))
		return
	}

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
		r.replyHTMLWithBack(chatID, fmt.Sprintf("<b>👥 AGY WORKER ACCOUNTS</b>\n\n<pre><code>%s</code></pre>", html.EscapeString(string(out))))
		return
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
		if acc.active {
			emoji = "🟢"
		}
		b.WriteString(fmt.Sprintf("%s <b>%s</b> • <b>%s</b>\n", emoji, html.EscapeString(acc.name), html.EscapeString(acc.status)))
		b.WriteString(fmt.Sprintf("  ├ ⏱ <code>%s</code>\n", html.EscapeString(acc.remaining)))
		b.WriteString(fmt.Sprintf("  └ 📧 <code>%s</code>\n\n", html.EscapeString(acc.email)))
	}

	r.replyHTMLWithBack(chatID, strings.TrimRight(b.String(), "\n"))
}

func (r *Router) handleRefresh(ctx context.Context, chatID int64) {
	r.sendTyping(chatID)

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

	cmd := exec.CommandContext(execCtx, "python3", "/opt/data/home/agy-profiles/fast_login.py", "refresh")
	out, err := cmd.CombinedOutput()
	close(typingDone)

	if err != nil && len(out) == 0 {
		r.replyHTMLWithBack(chatID, fmt.Sprintf("<b>🔴 Refresh Execution Failed:</b>\n<pre><code>%s</code></pre>", html.EscapeString(err.Error())))
		return
	}

	lines := strings.Split(string(out), "\n")
	var items []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[agy") {
			emoji := "🟢"
			if strings.Contains(trimmed, "Gagal") || strings.Contains(trimmed, "Error") {
				emoji = "🔴"
			} else if strings.Contains(trimmed, "Dilewati") {
				emoji = "⚪"
			}
			items = append(items, fmt.Sprintf("%s <code>%s</code>", emoji, html.EscapeString(trimmed)))
		}
	}

	var b strings.Builder
	b.WriteString("<b>🔄 AGY WORKER TOKENS REFRESH</b>\n\n")

	if len(items) > 0 {
		b.WriteString(strings.Join(items, "\n"))
	} else if len(out) > 0 {
		b.WriteString(fmt.Sprintf("<pre><code>%s</code></pre>", html.EscapeString(string(out))))
	} else {
		b.WriteString("<i>Command completed with no output.</i>")
	}

	if err != nil {
		b.WriteString(fmt.Sprintf("\n\n<b>Notice:</b> <code>%s</code>", html.EscapeString(err.Error())))
	}

	r.replyHTMLWithBack(chatID, b.String())
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

	callbackResp := tgbotapi.NewCallback(cb.ID, "")
	_, _ = r.api.Request(callbackResp)

	if data == "nav:start" {
		text := r.renderer.RenderWelcome(r.api.Self.UserName, sess.ActiveProfile, sess)
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

	switch data {
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

	case "nav:refresh":
		r.handleRefresh(ctx, chatID)

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
	edit := tgbotapi.NewEditMessageText(chatID, msgID, text)
	edit.ParseMode = tgbotapi.ModeHTML
	if markup != nil {
		edit.ReplyMarkup = markup
	}
	_, _ = r.api.Request(edit)
}

func (r *Router) replyHTML(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeHTML
	sent, err := r.api.Send(msg)
	if err == nil {
		r.sessionStore.SetLastBotMessageIDs(chatID, []int{sent.MessageID})
	}
}

func (r *Router) replyHTMLWithBack(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeHTML
	markup := r.getBackMarkup()
	msg.ReplyMarkup = markup
	sent, err := r.api.Send(msg)
	if err == nil {
		r.sessionStore.SetLastBotMessageIDs(chatID, []int{sent.MessageID})
	}
}

func (r *Router) replyChunks(chatID int64, text string) {
	chunks := r.renderer.ChunkMessage(text, 4000)
	var ids []int
	for _, chunk := range chunks {
		msg := tgbotapi.NewMessage(chatID, chunk)
		msg.ParseMode = tgbotapi.ModeHTML
		if sent, err := r.api.Send(msg); err == nil {
			ids = append(ids, sent.MessageID)
		}
	}
	if len(ids) > 0 {
		r.sessionStore.SetLastBotMessageIDs(chatID, ids)
	}
}

func (r *Router) sendTyping(chatID int64) {
	action := tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping)
	_, _ = r.api.Send(action)
}

func (r *Router) sendDocument(chatID int64, filePath string) {
	doc := tgbotapi.NewDocument(chatID, tgbotapi.FilePath(filePath))
	if sent, err := r.api.Send(doc); err == nil {
		r.sessionStore.SetLastBotMessageIDs(chatID, []int{sent.MessageID})
	}
}

func truncateString(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
