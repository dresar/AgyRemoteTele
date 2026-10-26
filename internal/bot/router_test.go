package bot

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/dresar/AgyRemoteTele/internal/agy"
	"github.com/dresar/AgyRemoteTele/internal/cluster"
	"github.com/dresar/AgyRemoteTele/internal/scheduler"
	"github.com/dresar/AgyRemoteTele/internal/session"
	"github.com/dresar/AgyRemoteTele/internal/watchdog"
)

func TestNormalizeAccount(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"1", "agy01"},
		{"agy2", "agy02"},
		{"agy03", "agy03"},
		{"account4", "agy04"},
		{"AGY05", "agy05"},
		{"agy10", "agy10"},
		{"15", "agy15"},
		{"", ""},
	}

	for _, tc := range cases {
		got := normalizeAccount(tc.input)
		if got != tc.expected {
			t.Errorf("normalizeAccount(%q) = %q; expected %q", tc.input, got, tc.expected)
		}
	}
}

func createTestRouter(t *testing.T) (*Router, *session.Store, *scheduler.Scheduler, *watchdog.Watchdog, *cluster.SwarmManager) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "sessions.json")
	cronPath := filepath.Join(tmpDir, "cron.json")

	store, err := session.NewStore(storePath, "agy01", tmpDir)
	if err != nil {
		t.Fatalf("failed to create session store: %v", err)
	}

	auth := session.NewAuthManager("admin123", []int64{1001})
	profiles := agy.NewProfileManager(tmpDir, "agy01")
	history := agy.NewHistoryReader(profiles)
	executor := agy.NewExecutor(profiles, "agy")
	renderer := NewRenderer()
	middleware := NewMiddleware(auth, store, 100, 24)

	router := NewRouter(
		nil,
		auth,
		store,
		profiles,
		history,
		executor,
		renderer,
		middleware,
		5*time.Second,
	)

	sched := scheduler.NewScheduler(cronPath, nil)
	wd := watchdog.NewWatchdog(watchdog.AlertConfig{}, nil)
	swarm := cluster.NewSwarmManager(tmpDir, "")

	router.SetScheduler(sched)
	router.SetWatchdog(wd)
	router.SetSwarmManager(swarm)

	return router, store, sched, wd, swarm
}

func TestRouterCommandHandling(t *testing.T) {
	router, store, sched, _, _ := createTestRouter(t)
	ctx := context.Background()

	sess := store.GetOrCreate(1001, "admin")
	sess.IsAuthenticated = true

	commands := []struct {
		text string
	}{
		{"/start"},
		{"/help"},
		{"/status"},
		{"/ps"},
		{"/ps 5 cpu"},
		{"/ports"},
		{"/service"},
		{"/service list"},
		{"/service status"},
		{"/service restart invalid_name"},
		{"/ping 127.0.0.1"},
		{"/curl http://127.0.0.1:9"},
		{"/dns localhost A"},
		{"/netio"},
		{"/docker ps"},
		{"/docker stats"},
		{"/docker logs c123 10"},
		{"/docker restart c123"},
		{"/cron list"},
		{"/cron add @every 1h /status"},
		{"/cron add 0 * * * * /status"},
		{"/alerts"},
		{"/backup"},
		{"/kill"},
		{"/kill abc"},
		{"/kill 999999 15"},
	}

	for _, cmd := range commands {
		cmdWord := strings.Fields(cmd.text)[0]
		up := tgbotapi.Update{
			UpdateID: 1,
			Message: &tgbotapi.Message{
				MessageID: 10,
				Chat:      &tgbotapi.Chat{ID: 1001},
				From:      &tgbotapi.User{UserName: "admin"},
				Text:      cmd.text,
				Entities: []tgbotapi.MessageEntity{
					{
						Type:   "bot_command",
						Offset: 0,
						Length: len(cmdWord),
					},
				},
			},
		}
		router.HandleUpdate(ctx, up)
	}

	jobs := sched.ListJobs()
	if len(jobs) != 2 {
		t.Errorf("expected 2 scheduled cron jobs, got %d", len(jobs))
	}
}

func TestRouterCronShortcuts(t *testing.T) {
	router, store, sched, _, _ := createTestRouter(t)
	ctx := context.Background()

	sess := store.GetOrCreate(1001, "admin")
	sess.IsAuthenticated = true

	job, err := sched.AddJob("@every 1h", "/status", 1001)
	if err != nil {
		t.Fatalf("failed to add job: %v", err)
	}

	runUp := tgbotapi.Update{
		UpdateID: 2,
		Message: &tgbotapi.Message{
			MessageID: 11,
			Chat:      &tgbotapi.Chat{ID: 1001},
			From:      &tgbotapi.User{UserName: "admin"},
			Text:      "/cron_run_" + job.ID,
		},
	}
	router.HandleUpdate(ctx, runUp)

	rmUp := tgbotapi.Update{
		UpdateID: 3,
		Message: &tgbotapi.Message{
			MessageID: 12,
			Chat:      &tgbotapi.Chat{ID: 1001},
			From:      &tgbotapi.User{UserName: "admin"},
			Text:      "/cron_rm_" + job.ID,
		},
	}
	router.HandleUpdate(ctx, rmUp)

	if len(sched.ListJobs()) != 0 {
		t.Errorf("expected job to be removed")
	}
}

func TestRouterCallbacks(t *testing.T) {
	router, store, _, _, _ := createTestRouter(t)
	ctx := context.Background()

	sess := store.GetOrCreate(1001, "admin")
	sess.IsAuthenticated = true

	callbacks := []string{
		"nav:start",
		"nav:cat_system",
		"nav:cat_docker",
		"nav:cat_net",
		"nav:cat_scheduler",
		"nav:cat_swarm",
		"nav:cat_backup",
		"nav:ps",
		"nav:ports",
		"nav:services",
		"nav:alerts",
		"nav:docker_ps",
		"nav:docker_stats",
		"nav:netio",
		"nav:ping_default",
		"nav:cron_list",
		"nav:status",
		"nav:help",
	}

	for _, cbData := range callbacks {
		up := tgbotapi.Update{
			UpdateID: 4,
			CallbackQuery: &tgbotapi.CallbackQuery{
				ID:   "cb_123",
				Data: cbData,
				From: &tgbotapi.User{UserName: "admin"},
				Message: &tgbotapi.Message{
					MessageID: 20,
					Chat:      &tgbotapi.Chat{ID: 1001},
				},
			},
		}
		router.HandleUpdate(ctx, up)
	}
}

func TestRouterSetters(t *testing.T) {
	router, _, _, _, _ := createTestRouter(t)

	newSched := scheduler.NewScheduler("", nil)
	newWd := watchdog.NewWatchdog(watchdog.AlertConfig{}, nil)
	newSwarm := cluster.NewSwarmManager("", "")

	router.SetScheduler(newSched)
	router.SetWatchdog(newWd)
	router.SetSwarmManager(newSwarm)

	if router.scheduler != newSched {
		t.Errorf("scheduler setter mismatch")
	}
	if router.watchdog != newWd {
		t.Errorf("watchdog setter mismatch")
	}
	if router.swarmManager != newSwarm {
		t.Errorf("swarm manager setter mismatch")
	}
}

func TestRouterAuthorizedAdminAutoAuth(t *testing.T) {
	router, store, _, _, _ := createTestRouter(t)
	ctx := context.Background()

	sess := store.GetOrCreate(1001, "admin")
	if sess.IsAuthenticated {
		t.Fatalf("expected initial session to be unauthenticated")
	}

	up := tgbotapi.Update{
		UpdateID: 100,
		Message: &tgbotapi.Message{
			MessageID: 10,
			Chat:      &tgbotapi.Chat{ID: 1001},
			From:      &tgbotapi.User{UserName: "admin"},
			Text:      "/start",
			Entities: []tgbotapi.MessageEntity{
				{
					Type:   "bot_command",
					Offset: 0,
					Length: 6,
				},
			},
		},
	}

	router.HandleUpdate(ctx, up)

	updatedSess, exists := store.Get(1001)
	if !exists {
		t.Fatalf("session for 1001 should exist")
	}
	if !updatedSess.IsAuthenticated {
		t.Errorf("expected sess.IsAuthenticated to be true for allowedChatID")
	}
}

func TestRouterGuestAutoAuth(t *testing.T) {
	router, store, _, _, _ := createTestRouter(t)
	ctx := context.Background()

	sess := store.GetOrCreate(2002, "guest")
	if sess.IsAuthenticated {
		t.Fatalf("expected initial guest session to be unauthenticated")
	}

	up := tgbotapi.Update{
		UpdateID: 101,
		Message: &tgbotapi.Message{
			MessageID: 11,
			Chat:      &tgbotapi.Chat{ID: 2002},
			From:      &tgbotapi.User{UserName: "guest"},
			Text:      "/start",
			Entities: []tgbotapi.MessageEntity{
				{
					Type:   "bot_command",
					Offset: 0,
					Length: 6,
				},
			},
		},
	}

	router.HandleUpdate(ctx, up)

	updatedSess, exists := store.Get(2002)
	if !exists {
		t.Fatalf("session for 2002 should exist")
	}
	if updatedSess.IsAuthenticated {
		t.Errorf("expected guest sess.IsAuthenticated to remain false")
	}
}

func TestSmartPromptWrapping(t *testing.T) {
	got := wrapPrompt(12345, "agy04", "/opt/data/home/AgyRemoteTele", "Perbaiki bot")

	expectedSubstrings := []string{
		"[TELEGRAM AGY REMOTE GATEWAY — AUTONOMOUS OPERATOR DIRECTIVE]",
		"Pengirim: Telegram Operator (Chat ID: 12345)",
		"Worker Profil: agy04",
		"Working Directory: /opt/data/home/AgyRemoteTele",
		"INSTRUKSI PENGGUNA:",
		"Perbaiki bot",
		"STANDAR OPERASIONAL CERDAS (FULL AUTONOMOUS & SELF-HEALING):",
		"1. OTONOMI PENUH: Eksekusi tools, bash command, dan tindakan teknis secara langsung",
		"2. SELF-HEALING & MAINTENANCE: Jika menemukan bug, error sistem, atau kegagalan",
		"3. STANDAR NOKOMEN: DILARANG menulis komentar dalam kode yang dibuat atau diedit.",
		"4. GIT-FIRST: Pastikan setiap perubahan kode yang teruji di-commit",
		"5. FORMAT RESPON TELEGRAM: Berikan jawaban akhir yang SINGKAT, PADAT, dan TO-THE-POINT",
	}

	for _, sub := range expectedSubstrings {
		if !strings.Contains(got, sub) {
			t.Errorf("wrapPrompt missing expected substring %q in output:\n%s", sub, got)
		}
	}
}

func TestRenderAccountsStatus(t *testing.T) {
	raw := `
PROFIL   | STATUS          | SISA WAKTU                  | EMAIL GOOGLE                       
------------------------------------------------------------------------------------------------
agy01    | Aktif           | 40 menit 42 detik           | eka.ckp16799@gmail.com             
agy02    | Standby         | -                           | Unknown                            
agy03    | Siap Aktif      | -                           | Unknown                            
agy04    | Kedaluwarsa     | Kedaluwarsa (15m lalu)      | Unknown                            
`
	text, markup := renderAccountsStatus([]byte(raw))

	if !strings.Contains(text, "🟢 <b>agy01</b> • <b>Aktif</b>") {
		t.Errorf("expected agy01 to be active with green emoji, got:\n%s", text)
	}
	if !strings.Contains(text, "🟡 <b>agy02</b> • <b>Standby — Siap Dihidupkan</b>") {
		t.Errorf("expected agy02 to be standby with yellow emoji, got:\n%s", text)
	}
	if !strings.Contains(text, "🟡 <b>agy03</b> • <b>Siap Aktif — Siap Dihidupkan</b>") {
		t.Errorf("expected agy03 to be siap aktif with yellow emoji, got:\n%s", text)
	}
	if !strings.Contains(text, "🔴 <b>agy04</b> • <b>Kedaluwarsa</b>") {
		t.Errorf("expected agy04 to be expired with red emoji, got:\n%s", text)
	}

	foundUpAll := false
	for _, row := range markup.InlineKeyboard {
		for _, btn := range row {
			if btn.Text == "⚡ Hidupkan Semua" && *btn.CallbackData == "nav:up_all" {
				foundUpAll = true
			}
		}
	}
	if !foundUpAll {
		t.Errorf("expected inline keyboard to have '⚡ Hidupkan Semua' button")
	}
}

func TestRenderUpSingle(t *testing.T) {
	textSuccess, markupSuccess := renderUpSingle("agy02", "Berhasil di-refresh", "user@example.com", "45 menit", "", false)
	if !strings.Contains(textSuccess, "⚡ WORKER DIAKTIFKAN — AGY02") {
		t.Errorf("unexpected title in success text: %s", textSuccess)
	}
	if !strings.Contains(textSuccess, "🟢 <b>Berhasil di-refresh</b>") {
		t.Errorf("unexpected status in success text: %s", textSuccess)
	}
	if !strings.Contains(textSuccess, "45 menit") {
		t.Errorf("unexpected remaining in success text: %s", textSuccess)
	}
	if !strings.Contains(textSuccess, "user@example.com") {
		t.Errorf("unexpected email in success text: %s", textSuccess)
	}
	if len(markupSuccess.InlineKeyboard) == 0 {
		t.Errorf("expected markup in renderUpSingle")
	}

	textErr, _ := renderUpSingle("agy03", "Gagal refresh", "-", "-", "Token missing", true)
	if !strings.Contains(textErr, "🔴 <b>Gagal refresh</b>") {
		t.Errorf("unexpected status in error text: %s", textErr)
	}
	if !strings.Contains(textErr, "Token missing") {
		t.Errorf("expected detail in error text: %s", textErr)
	}
}

func TestRenderUpAll(t *testing.T) {
	sampleOutput := `
[agy01] Berhasil (eka@example.com) - Aktif, sisa 30 menit 0 detik
[agy02] Dilewati: Belum login
[agy03] Gagal - Token expired
`
	text, markup := renderUpAll(sampleOutput, nil)
	if !strings.Contains(text, "⚡ SEMUA WORKER DIAKTIFKAN") {
		t.Errorf("unexpected header: %s", text)
	}
	if !strings.Contains(text, "🟢 <b>AGY01</b> • Aktif, sisa 30 menit 0 detik") {
		t.Errorf("expected agy01 success formatted, got: %s", text)
	}
	if !strings.Contains(text, "eka@example.com") {
		t.Errorf("expected email displayed, got: %s", text)
	}
	if !strings.Contains(text, "🟡 <code>[agy02] Dilewati: Belum login</code>") {
		t.Errorf("expected agy02 skipped with yellow emoji, got: %s", text)
	}
	if !strings.Contains(text, "🔴 <b>AGY03</b> • Token expired") {
		t.Errorf("expected agy03 failure with red emoji, got: %s", text)
	}
	if len(markup.InlineKeyboard) == 0 {
		t.Errorf("expected markup in renderUpAll")
	}
}

func TestRouterUpCommandHandling(t *testing.T) {
	router, store, _, _, _ := createTestRouter(t)
	ctx := context.Background()

	sess := store.GetOrCreate(1001, "admin")
	sess.IsAuthenticated = true

	tmpDir := t.TempDir()
	mockScript := filepath.Join(tmpDir, "mock_fast_login.py")
	scriptContent := "#!/usr/bin/env python3\n" +
		"import sys\n" +
		"if len(sys.argv) > 1 and sys.argv[1] == 'status':\n" +
		"    print('agy01 | Aktif | 45m | user@example.com')\n" +
		"elif len(sys.argv) > 2 and sys.argv[1] == 'refresh':\n" +
		"    print('Profil : ' + sys.argv[2])\n" +
		"    print('Status : Berhasil di-refresh')\n" +
		"    print('Email : user@example.com')\n" +
		"    print('Sisa Waktu : 50 menit')\n" +
		"elif len(sys.argv) > 1 and sys.argv[1] == 'refresh':\n" +
		"    print('[agy01] Berhasil (user@example.com) - Aktif, sisa 50m')\n"
	if err := os.WriteFile(mockScript, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("failed to write mock script: %v", err)
	}
	router.SetFastLoginScript(mockScript)

	commands := []string{
		"/up",
		"/up agy02",
		"/up semua",
		"/hidupkan",
		"/hidupkan agy03",
		"/hidupkan semua",
		"/start_worker agy04",
		"/refresh",
		"/refresh agy05",
		"/accounts",
	}

	for _, cmdText := range commands {
		cmdWord := strings.Fields(cmdText)[0]
		up := tgbotapi.Update{
			UpdateID: 200,
			Message: &tgbotapi.Message{
				MessageID: 50,
				Chat:      &tgbotapi.Chat{ID: 1001},
				From:      &tgbotapi.User{UserName: "admin"},
				Text:      cmdText,
				Entities: []tgbotapi.MessageEntity{
					{
						Type:   "bot_command",
						Offset: 0,
						Length: len(cmdWord),
					},
				},
			},
		}
		router.HandleUpdate(ctx, up)
	}

	textMessages := []string{
		"hidupkan agy02",
		"hidupkan semua",
		"hidupkan",
		"start_worker agy03",
		"up agy04",
	}

	for _, msgText := range textMessages {
		up := tgbotapi.Update{
			UpdateID: 201,
			Message: &tgbotapi.Message{
				MessageID: 51,
				Chat:      &tgbotapi.Chat{ID: 1001},
				From:      &tgbotapi.User{UserName: "admin"},
				Text:      msgText,
			},
		}
		router.HandleUpdate(ctx, up)
	}

	callbacks := []string{
		"up_acc:agy01",
		"refresh_acc:agy02",
		"nav:up_all",
		"nav:accounts",
	}

	for _, cbData := range callbacks {
		up := tgbotapi.Update{
			UpdateID: 202,
			CallbackQuery: &tgbotapi.CallbackQuery{
				ID:   "cb_up",
				Data: cbData,
				From: &tgbotapi.User{UserName: "admin"},
				Message: &tgbotapi.Message{
					MessageID: 52,
					Chat:      &tgbotapi.Chat{ID: 1001},
				},
			},
		}
		router.HandleUpdate(ctx, up)
	}
}

func TestCleanAgentResponse(t *testing.T) {
	rawNormal := "Halo, ini respon bersih dari agent."
	if got := cleanAgentResponse(rawNormal); got != rawNormal {
		t.Errorf("cleanAgentResponse(%q) = %q, expected unchanged", rawNormal, got)
	}

	rawMultilinePython := `⚡ TASK EXECUTION COMPLETED
Profile: agy10 | Duration: 206.2s

Executive Summary:
Hasil pengecekan riwayat chat untuk Telegram Operator (Chat ID: 8638950192):

🛠 Tools Invoked:
• run_command: ls -la /opt/data/home/AgyRemoteTele/data (PASS)
• read_file: /opt/data/home/AgyRemoteTele/data/sessions.json
• run_command: ls -la /opt/data/home/agy-profiles/agy10/.gemini/antigravity-cli/brain/99323ee4-56fa-4204-b709-801578eff3d1/.system_generated/logs/ (PASS)
• run_command: python3 -c "
import json
with open('/opt/data/home/agy-profiles/agy10/.gemini/antigravity-cli/brain/99323ee4-56fa-4204-b709-801578eff3d1/.system_generated/logs/transcript.jsonl') as f:
    for i, line in enumerate(f):
        data = json.loads(line)
        stype = data.get('type')
        created_at = data.get('created_at')
        if stype == 'USER_INPUT':
            content = data.get('content', '')
            print(f'Step {i} [{created_at}] USER_INPUT: {content[:100]}...')
        elif stype == 'PLANN… (PASS)
• run_command: ls -la /opt/data/home/agy-profiles/agy10/.gemini/antigravity-cli/brain/ (PASS)
• run_command: grep -n "8638950192" /opt/data/home/AgyRemoteTele/data/bot.log | head -n 30 (PASS)
• run_command: tail -n 60 /opt/data/home/AgyRemoteTele/data/bot.log (FAIL)
• run_command: ls -la /opt/data/home/AgyRemoteTele/data/backups/ (PASS)
• run_command: tar -tzf /opt/data/home/AgyRemoteTele/data/backups/backup_20261026_083227.tar.gz (PASS)
• run_command: tar -xzf /opt/data/home/AgyRemoteTele/data/backups/backup_20261026_083227.tar.gz -O 001/sessions.json (PASS)
• run_command: python3 -c "
import json
with open('/opt/data/home/agy-profiles/agy10/.gemini/antigravity-cli/brain/99323ee4-56fa-4204-b709-801578eff3d1/.system_generated/logs/transcript.jsonl') as f:
    for i, line in enumerate(f):
        data = json.loads(line)
        if data.get('type') == 'USER_INPUT':
            print(f'=== Step {i} at {data.get(\"created_at\")} ===')
            print(data.get('content'))
" (FAIL)
• run_command: python3 -c "
import glob, json, os

profiles = glob.glob('/opt/data/home/agy-profiles/' + '*' + '/.gemini/antigravity-cli/brain/' + '*' + '/.system_generated/logs/transcript.jsonl')
found = []
for p in profiles:
    try:
        with open(p, 'r') as f:
            first_line = f.readline()
            if '8638950192' in first_line:
                conv_id = p.split('/brain/')[1].split('/')[0]
                worker = p.split('agy-profiles/')[1].split('/')[0]
                mtime = os.path.getmtime(p)
                found.app… (PASS)

Session ID: 99323ee4-56fa-4204-b709-801578eff3d1

Riwayat percakapan berhasil ditemukan dan siap dianalisis.`

	gotMultiline := cleanAgentResponse(rawMultilinePython)
	if strings.Contains(gotMultiline, "TASK EXECUTION COMPLETED") {
		t.Errorf("expected TASK EXECUTION COMPLETED to be stripped, got: %s", gotMultiline)
	}
	if strings.Contains(gotMultiline, "Tools Invoked:") {
		t.Errorf("expected Tools Invoked to be stripped, got: %s", gotMultiline)
	}
	if strings.Contains(gotMultiline, "import json") || strings.Contains(gotMultiline, "run_command:") {
		t.Errorf("expected multiline python commands to be stripped, got: %s", gotMultiline)
	}
	if strings.Contains(gotMultiline, "Session ID:") {
		t.Errorf("expected Session ID to be stripped, got: %s", gotMultiline)
	}
	if !strings.Contains(gotMultiline, "Hasil pengecekan riwayat chat untuk Telegram Operator (Chat ID: 8638950192):") {
		t.Errorf("expected summary text to be preserved, got: %s", gotMultiline)
	}
	if !strings.Contains(gotMultiline, "Riwayat percakapan berhasil ditemukan dan siap dianalisis.") {
		t.Errorf("expected final answer to be preserved, got: %s", gotMultiline)
	}

	rawHTML := `<b>⚡ TASK EXECUTION COMPLETED</b>
<b>Profile:</b> <code>agy10</code> | <b>Duration:</b> <code>12.3s</code>

<blockquote><b>Executive Summary:</b>
Server normal dan sehat.
</blockquote>

<b>🛠 Tools Invoked:</b>
• <code>run_command</code>: <code>top -b -n 1</code> (<b>PASS</b>)

<i>Session ID:</i> <code>99323ee4</code>`

	gotHTML := cleanAgentResponse(rawHTML)
	if strings.Contains(gotHTML, "TASK EXECUTION COMPLETED") || strings.Contains(gotHTML, "Tools Invoked") || strings.Contains(gotHTML, "Session ID") {
		t.Errorf("expected HTML card to be cleaned, got: %s", gotHTML)
	}
	if !strings.Contains(gotHTML, "Server normal dan sehat.") {
		t.Errorf("expected HTML summary text to be preserved, got: %s", gotHTML)
	}

	rawCode := "Berikut solusi kode:\n```go\npackage main\nfunc main() {}\n```\nSelesai."
	gotCode := cleanAgentResponse(rawCode)
	if gotCode != rawCode {
		t.Errorf("expected code block to be preserved, got: %s", gotCode)
	}
}

func TestRendererTools(t *testing.T) {
	renderer := NewRenderer()
	toolsText := renderer.RenderTools()

	expectedSubstrings := []string{
		"ANTIGRAVITY TOOLS & AUTONOMOUS ENGINE",
		"run_command",
		"read_file",
		"write_to_file",
		"find_by_name",
		"grep_search",
		"list_dir",
		"manage_task",
		"--dangerously-skip-permissions",
		"Autonomous Operator Directive",
		"Clean Output Stream",
		"agy-code-review",
		"agy-operating-protocol",
	}

	for _, sub := range expectedSubstrings {
		if !strings.Contains(toolsText, sub) {
			t.Errorf("RenderTools() missing %q in output:\n%s", sub, toolsText)
		}
	}
}

func TestRouterNewAndContinueCommands(t *testing.T) {
	router, store, _, _, _ := createTestRouter(t)
	ctx := context.Background()

	sess := store.GetOrCreate(1001, "admin")
	sess.IsAuthenticated = true
	sess.ActiveConversationID = "conv-12345"

	upNew := tgbotapi.Update{
		UpdateID: 301,
		Message: &tgbotapi.Message{
			MessageID: 101,
			Chat:      &tgbotapi.Chat{ID: 1001},
			From:      &tgbotapi.User{UserName: "admin"},
			Text:      "/new",
			Entities: []tgbotapi.MessageEntity{
				{Type: "bot_command", Offset: 0, Length: 4},
			},
		},
	}
	router.HandleUpdate(ctx, upNew)

	updatedSess, _ := store.Get(1001)
	if updatedSess.ActiveConversationID != "" {
		t.Errorf("expected /new to clear active conversation ID, got: %s", updatedSess.ActiveConversationID)
	}

	upContinueEmpty := tgbotapi.Update{
		UpdateID: 302,
		Message: &tgbotapi.Message{
			MessageID: 102,
			Chat:      &tgbotapi.Chat{ID: 1001},
			From:      &tgbotapi.User{UserName: "admin"},
			Text:      "/continue",
			Entities: []tgbotapi.MessageEntity{
				{Type: "bot_command", Offset: 0, Length: 9},
			},
		},
	}
	router.HandleUpdate(ctx, upContinueEmpty)
}

func TestRouterActiveTaskProtectionAndKill(t *testing.T) {
	router, store, _, _, _ := createTestRouter(t)
	ctx := context.Background()

	sess := store.GetOrCreate(1001, "admin")
	sess.IsAuthenticated = true

	canceled := false
	_, cancel := context.WithCancel(ctx)
	defer cancel()

	wrappedCancel := func() {
		canceled = true
		cancel()
	}

	router.activeTasksMu.Lock()
	router.activeTasks[1001] = &ActiveTask{
		Cancel:    wrappedCancel,
		Profile:   "agy10",
		Prompt:    "sedang memproses tugas berat",
		StartedAt: time.Now().Add(-10 * time.Second),
	}
	router.activeTasksMu.Unlock()

	upWhileRunning := tgbotapi.Update{
		UpdateID: 401,
		Message: &tgbotapi.Message{
			MessageID: 201,
			Chat:      &tgbotapi.Chat{ID: 1001},
			From:      &tgbotapi.User{UserName: "admin"},
			Text:      "Tolong periksa status",
		},
	}
	router.HandleUpdate(ctx, upWhileRunning)

	router.activeTasksMu.Lock()
	_, stillExists := router.activeTasks[1001]
	router.activeTasksMu.Unlock()
	if !stillExists {
		t.Fatalf("expected active task to remain present after busy prompt")
	}

	upKillPID := tgbotapi.Update{
		UpdateID: 4015,
		Message: &tgbotapi.Message{
			MessageID: 2015,
			Chat:      &tgbotapi.Chat{ID: 1001},
			From:      &tgbotapi.User{UserName: "admin"},
			Text:      "/kill 999999",
			Entities: []tgbotapi.MessageEntity{
				{Type: "bot_command", Offset: 0, Length: 5},
			},
		},
	}
	router.HandleUpdate(ctx, upKillPID)

	if canceled {
		t.Fatalf("expected /kill <pid> not to cancel active task")
	}

	upKill := tgbotapi.Update{
		UpdateID: 402,
		Message: &tgbotapi.Message{
			MessageID: 202,
			Chat:      &tgbotapi.Chat{ID: 1001},
			From:      &tgbotapi.User{UserName: "admin"},
			Text:      "/kill",
			Entities: []tgbotapi.MessageEntity{
				{Type: "bot_command", Offset: 0, Length: 5},
			},
		},
	}
	router.HandleUpdate(ctx, upKill)

	if !canceled {
		t.Errorf("expected active task cancel function to be called on /kill")
	}

	router.activeTasksMu.Lock()
	_, existsAfterKill := router.activeTasks[1001]
	router.activeTasksMu.Unlock()
	if existsAfterKill {
		t.Errorf("expected active task to be removed after /kill")
	}

	upKillEmpty := tgbotapi.Update{
		UpdateID: 403,
		Message: &tgbotapi.Message{
			MessageID: 203,
			Chat:      &tgbotapi.Chat{ID: 1001},
			From:      &tgbotapi.User{UserName: "admin"},
			Text:      "/kill",
			Entities: []tgbotapi.MessageEntity{
				{Type: "bot_command", Offset: 0, Length: 5},
			},
		},
	}
	router.HandleUpdate(ctx, upKillEmpty)
}

func TestRouterAdvancedCommands(t *testing.T) {
	router, store, _, _, _ := createTestRouter(t)
	ctx := context.Background()

	sess := store.GetOrCreate(1001, "admin")
	sess.IsAuthenticated = true

	commands := []string{
		"/plan buatkan arsitektur microservices",
		"/boost learn",
		"/teamwork koordinasi refactor cluster",
		"/tools",
		"/lanjut",
		"/new kerjakan tugas pertama",
	}

	for i, cmdStr := range commands {
		cmdWord := strings.Fields(cmdStr)[0]
		up := tgbotapi.Update{
			UpdateID: 500 + i,
			Message: &tgbotapi.Message{
				MessageID: 300 + i,
				Chat:      &tgbotapi.Chat{ID: 1001},
				From:      &tgbotapi.User{UserName: "admin"},
				Text:      cmdStr,
				Entities: []tgbotapi.MessageEntity{
					{Type: "bot_command", Offset: 0, Length: len(cmdWord)},
				},
			},
		}
		router.HandleUpdate(ctx, up)
	}

	textTriggers := []string{
		"new",
		"new mulai proyek baru",
		"plan",
		"plan audit performa vps",
		"teamwork",
		"teamwork audit keamanan cluster",
		"boost learn",
		"tools",
		"lanjut",
		"kill",
	}

	for i, txt := range textTriggers {
		up := tgbotapi.Update{
			UpdateID: 600 + i,
			Message: &tgbotapi.Message{
				MessageID: 400 + i,
				Chat:      &tgbotapi.Chat{ID: 1001},
				From:      &tgbotapi.User{UserName: "admin"},
				Text:      txt,
			},
		}
		router.HandleUpdate(ctx, up)
	}

	callbackUp := tgbotapi.Update{
		UpdateID: 701,
		CallbackQuery: &tgbotapi.CallbackQuery{
			ID:   "cb_tools",
			Data: "nav:tools",
			From: &tgbotapi.User{UserName: "admin"},
			Message: &tgbotapi.Message{
				MessageID: 501,
				Chat:      &tgbotapi.Chat{ID: 1001},
			},
		},
	}
	router.HandleUpdate(ctx, callbackUp)
}
