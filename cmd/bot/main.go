package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/dresar/AgyRemoteTele/internal/agy"
	"github.com/dresar/AgyRemoteTele/internal/bot"
	"github.com/dresar/AgyRemoteTele/internal/cluster"
	"github.com/dresar/AgyRemoteTele/internal/config"
	"github.com/dresar/AgyRemoteTele/internal/scheduler"
	"github.com/dresar/AgyRemoteTele/internal/session"
	"github.com/dresar/AgyRemoteTele/internal/watchdog"
)

func main() {
	lockPath := "/opt/data/home/AgyRemoteTele/data/agyremote.lock"
	if err := os.MkdirAll(filepath.Dir(lockPath), 0755); err != nil {
		log.Fatalf("failed to create lock directory: %v", err)
	}

	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		log.Fatalf("failed to open lock file: %v", err)
	}
	defer lockFile.Close()

	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		fmt.Fprintf(os.Stderr, "another instance of agyremote is already running: lock held on %s (%v)\n", lockPath, err)
		os.Exit(1)
	}
	defer syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)

	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("configuration initialization failed: %v", err)
	}

	sessionStore, err := session.NewStore(cfg.SessionFilePath, cfg.DefaultProfile, cfg.WorkspaceRoot)
	if err != nil {
		log.Fatalf("session store initialization failed: %v", err)
	}

	authManager := session.NewAuthManager(cfg.AdminAuthKey, cfg.AllowedChatIDs)
	profileManager := agy.NewProfileManager(cfg.ProfilesRoot, cfg.DefaultProfile)
	historyReader := agy.NewHistoryReader(profileManager)
	executor := agy.NewExecutor(profileManager, cfg.AgyBinPath)
	renderer := bot.NewRenderer()
	middleware := bot.NewMiddleware(authManager, sessionStore, cfg.RateLimitPerMinute, cfg.SessionExpirationHours)

	router := bot.NewRouter(
		nil,
		authManager,
		sessionStore,
		profileManager,
		historyReader,
		executor,
		renderer,
		middleware,
		cfg.ExecutionTimeout,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	schedStore := filepath.Join(filepath.Dir(cfg.SessionFilePath), "cron_jobs.json")
	sched := scheduler.NewScheduler(schedStore, func(chatID int64, cmd string) {
		up := tgbotapi.Update{
			Message: &tgbotapi.Message{
				Chat: &tgbotapi.Chat{ID: chatID},
				From: &tgbotapi.User{UserName: "scheduler"},
				Text: cmd,
			},
		}
		router.HandleUpdate(ctx, up)
	})

	wdCfg := watchdog.AlertConfig{
		CheckInterval:       60 * time.Second,
		HighMemThreshold:    85.0,
		CritMemThreshold:    92.0,
		HighDiskThreshold:   90.0,
		TokenWarningMinutes: 30,
	}

	var botInstance *bot.Bot
	wd := watchdog.NewWatchdog(wdCfg, func(level, title, message string) {
		if botInstance != nil {
			botInstance.BroadcastAlert(level, title, message)
		}
	})

	swarmMgr := cluster.NewSwarmManager(cfg.ProfilesRoot, "/opt/data/home/agy-profiles/orchestrator.py")

	router.SetScheduler(sched)
	router.SetWatchdog(wd)
	router.SetSwarmManager(swarmMgr)

	b, err := bot.NewBot(cfg.TelegramBotToken, router, sessionStore)
	if err != nil {
		log.Fatalf("failed to create telegram bot instance: %v", err)
	}
	b.SetAdminChatIDs(cfg.AllowedChatIDs)
	botInstance = b

	sched.Start(ctx)
	wd.Start(ctx)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		log.Printf("received signal %s, initiating graceful shutdown...", sig)
		cancel()
		b.Stop()
		wd.Stop()
		sched.Stop()
	}()

	if err := b.Start(ctx); err != nil {
		log.Fatalf("bot runner exited with error: %v", err)
	}

	wd.Stop()
	sched.Stop()
}
