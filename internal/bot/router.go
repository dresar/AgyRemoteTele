package bot

import (
	"context"
	"fmt"
	"html"
	"os"
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

	if strings.HasPrefix(text, "/resume_") {
		convID := strings.TrimPrefix(text, "/resume_")
		r.handleResume(chatID, sess, convID)
		return
	}

	cmd := msg.Command()
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

func (r *Router) handleStart(chatID int64, sess *session.Session) {
	text := r.renderer.RenderWelcome(r.api.Self.UserName, sess.ActiveProfile, sess)
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⚡ Status", "nav:status"),
			tgbotapi.NewInlineKeyboardButtonData("👤 Profiles", "nav:profile"),
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

	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeHTML
	msg.ReplyMarkup = keyboard
	_, _ = r.api.Send(msg)
}

func (r *Router) handleHelp(chatID int64) {
	r.replyHTML(chatID, r.renderer.RenderHelp())
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
		r.replyHTML(chatID, r.renderer.RenderProfileSwitched(oldProfile, clean))
		return
	}

	profiles, err := r.profileManager.ListProfiles()
	if err != nil || len(profiles) == 0 {
		r.replyHTML(chatID, fmt.Sprintf("<b>Active Profile:</b> <code>%s</code>\nNo other profile directories discovered.", html.EscapeString(sess.ActiveProfile)))
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

	msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("<b>👤 PROFILE SWITCHER</b>\n\nCurrent Active: 🟢 <code>%s</code>\nSelect worker profile:", html.EscapeString(sess.ActiveProfile)))
	msg.ParseMode = tgbotapi.ModeHTML
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	_, _ = r.api.Send(msg)
}

func (r *Router) handleHistory(chatID int64, sess *session.Session) {
	items, err := r.historyReader.ListRecentConversations(sess.ActiveProfile, 8)
	if err != nil {
		r.replyHTML(chatID, fmt.Sprintf("<b>🔴 Error reading history:</b> %s", html.EscapeString(err.Error())))
		return
	}
	r.replyHTML(chatID, r.renderer.RenderHistory(sess.ActiveProfile, items))
}

func (r *Router) handleResume(chatID int64, sess *session.Session, convID string) {
	cleanID := strings.TrimSpace(convID)
	if cleanID == "" {
		r.replyHTML(chatID, "<b>Usage:</b> <code>/resume &lt;conversation_id&gt;</code>")
		return
	}

	r.sessionStore.SetConversation(chatID, cleanID)
	r.replyHTML(chatID, fmt.Sprintf("<b>⚡ SESSION RESUMED</b>\n\nActive Conversation: <code>%s</code>\nProfile: <code>%s</code>\n\nSubsequent <code>/ask</code> instructions will continue this conversation.",
		html.EscapeString(cleanID), html.EscapeString(sess.ActiveProfile)))
}

func (r *Router) handleNew(chatID int64, sess *session.Session) {
	r.sessionStore.ClearConversation(chatID)
	r.replyHTML(chatID, fmt.Sprintf("<b>✨ NEW SESSION INITIALIZED</b>\n\nCleared active conversation context on <code>%s</code>. Next instruction will start fresh.", html.EscapeString(sess.ActiveProfile)))
}

func (r *Router) handleStatus(chatID int64, sess *session.Session) {
	metrics, err := system.CollectMetrics(sess.ActiveWorkingDir)
	if err != nil {
		r.replyHTML(chatID, fmt.Sprintf("<b>🔴 Telemetry Error:</b> %s", html.EscapeString(err.Error())))
		return
	}
	r.replyHTML(chatID, r.renderer.RenderStatus(metrics, sess.ActiveProfile))
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
		r.replyHTML(chatID, fmt.Sprintf("<b>🔴 Directory Error:</b> %s", html.EscapeString(err.Error())))
		return
	}

	r.replyHTML(chatID, r.renderer.RenderFileList(dir, items))
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
	r.replyHTML(chatID, fmt.Sprintf("<b>📁 Working directory updated:</b>\n<code>%s</code>", html.EscapeString(dest)))
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
			r.replyHTML(chatID, fmt.Sprintf("<b>🔴 Git Sync Failed:</b>\n<pre><code>%s</code></pre>", html.EscapeString(err.Error())))
			return
		}
		r.replyHTML(chatID, fmt.Sprintf("<b>🟢 Git Sync Successful:</b>\n<pre><code>%s</code></pre>", html.EscapeString(out)))
		return
	}

	summary, err := workspace.InspectRepository(repoPath)
	if err != nil {
		r.replyHTML(chatID, fmt.Sprintf("<b>🔴 Git Error:</b> %s", html.EscapeString(err.Error())))
		return
	}

	r.replyHTML(chatID, r.renderer.RenderGitSummary(summary))
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

	req := agy.ExecutionRequest{
		Profile:        sess.ActiveProfile,
		Prompt:         cleanPrompt,
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

	r.replyHTML(chatID, reportHTML)

	if result.Response != "" {
		respHeader := fmt.Sprintf("<b>💬 Agent Response:</b>\n\n%s", html.EscapeString(result.Response))
		r.replyChunks(chatID, respHeader)
	}
}

func (r *Router) handleCallbackQuery(ctx context.Context, cb *tgbotapi.CallbackQuery) {
	data := cb.Data
	chatID := cb.Message.Chat.ID
	username := cb.From.UserName

	sess, authorized, _ := r.middleware.ProcessAccess(chatID, username, false)
	if !authorized {
		callbackResp := tgbotapi.NewCallback(cb.ID, "Unauthorized device")
		_, _ = r.api.Request(callbackResp)
		return
	}

	callbackResp := tgbotapi.NewCallback(cb.ID, "")
	_, _ = r.api.Request(callbackResp)

	if strings.HasPrefix(data, "profile:") {
		target := strings.TrimPrefix(data, "profile:")
		r.handleProfile(chatID, sess, target)
		return
	}

	if strings.HasPrefix(data, "resume:") {
		target := strings.TrimPrefix(data, "resume:")
		r.handleResume(chatID, sess, target)
		return
	}

	switch data {
	case "nav:status":
		r.handleStatus(chatID, sess)
	case "nav:profile":
		r.handleProfile(chatID, sess, "")
	case "nav:history":
		r.handleHistory(chatID, sess)
	case "nav:ls":
		r.handleLs(chatID, sess, "")
	case "nav:git":
		r.handleGit(ctx, chatID, sess, "")
	case "nav:help":
		r.handleHelp(chatID)
	}
}

func (r *Router) replyHTML(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeHTML
	_, _ = r.api.Send(msg)
}

func (r *Router) replyChunks(chatID int64, text string) {
	chunks := r.renderer.ChunkMessage(text, 4000)
	for _, chunk := range chunks {
		msg := tgbotapi.NewMessage(chatID, chunk)
		msg.ParseMode = tgbotapi.ModeHTML
		_, _ = r.api.Send(msg)
	}
}

func (r *Router) sendTyping(chatID int64) {
	action := tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping)
	_, _ = r.api.Send(action)
}

func (r *Router) sendDocument(chatID int64, filePath string) {
	doc := tgbotapi.NewDocument(chatID, tgbotapi.FilePath(filePath))
	_, _ = r.api.Send(doc)
}

func truncateString(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
