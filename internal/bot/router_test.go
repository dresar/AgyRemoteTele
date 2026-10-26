package bot

import (
	"context"
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
