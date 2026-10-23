package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/dresar/AgyRemoteTele/internal/agy"
	"github.com/dresar/AgyRemoteTele/internal/bot"
	"github.com/dresar/AgyRemoteTele/internal/config"
	"github.com/dresar/AgyRemoteTele/internal/session"
)

func main() {
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

	b, err := bot.NewBot(cfg.TelegramBotToken, router, sessionStore)
	if err != nil {
		log.Fatalf("failed to create telegram bot instance: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		log.Printf("received signal %s, initiating graceful shutdown...", sig)
		cancel()
		b.Stop()
	}()

	if err := b.Start(ctx); err != nil {
		log.Fatalf("bot runner exited with error: %v", err)
	}
}
