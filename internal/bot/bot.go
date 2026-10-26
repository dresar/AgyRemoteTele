package bot

import (
	"context"
	"log"
	"sync"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/dresar/AgyRemoteTele/internal/session"
)

type Bot struct {
	api          *tgbotapi.BotAPI
	router       *Router
	sessionStore *session.Store
	stopChan     chan struct{}
	stopOnce     sync.Once
	wg           sync.WaitGroup
}

func NewBot(token string, router *Router, sessionStore *session.Store) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, err
	}

	router.api = api

	return &Bot{
		api:          api,
		router:       router,
		sessionStore: sessionStore,
		stopChan:     make(chan struct{}),
	}, nil
}

func (b *Bot) Start(ctx context.Context) error {
	log.Printf("AgyRemoteTele bot started as @%s (ID: %d)", b.api.Self.UserName, b.api.Self.ID)

	b.registerCommands()

	updateConfig := tgbotapi.NewUpdate(0)
	updateConfig.Timeout = 30

	updates := b.api.GetUpdatesChan(updateConfig)

	for {
		select {
		case <-ctx.Done():
			b.Stop()
			return nil
		case <-b.stopChan:
			return nil
		case update, ok := <-updates:
			if !ok {
				return nil
			}
			b.wg.Add(1)
			go func(up tgbotapi.Update) {
				defer b.wg.Done()
				b.router.HandleUpdate(ctx, up)
			}(update)
		}
	}
}

func (b *Bot) registerCommands() {
	commands := []tgbotapi.BotCommand{
		{Command: "start", Description: "Pusat kontrol interaktif AgyRemoteTele"},
		{Command: "help", Description: "Panduan bantuan penggunaan bot"},
		{Command: "status", Description: "Live telemetry VPS: CPU, RAM, Disk"},
		{Command: "accounts", Description: "Status worker accounts dan token"},
		{Command: "refresh", Description: "Refresh token worker account"},
		{Command: "workers", Description: "Status daemon worker agy01-agy10"},
		{Command: "tasks", Description: "Daftar task aktif dan riwayat worker"},
		{Command: "usage", Description: "Monitor penggunaan token dan resource cluster"},
		{Command: "send", Description: "Kirim prompt ke worker secara asynchronous"},
		{Command: "ask", Description: "Kirim instruksi prompt ke agent AGY"},
		{Command: "profile", Description: "Ganti profil worker (agy01 - agy15)"},
		{Command: "history", Description: "Lihat transcript sesi percakapan"},
		{Command: "resume", Description: "Lanjutkan percakapan sebelumnya"},
		{Command: "new", Description: "Mulai percakapan baru dari awal"},
		{Command: "ls", Description: "Jelajahi direktori file workspace"},
		{Command: "cd", Description: "Pindah direktori kerja aktif"},
		{Command: "cat", Description: "Lihat isi file di VPS"},
		{Command: "get", Description: "Download file dari VPS sebagai dokumen"},
		{Command: "sh", Description: "Jalankan perintah bash terminal"},
		{Command: "git", Description: "Cek git status, diff, dan sync"},
	}

	if b.api == nil {
		return
	}

	cfg := tgbotapi.NewSetMyCommands(commands...)
	if _, err := b.api.Request(cfg); err != nil {
		log.Printf("failed to register default bot commands: %v", err)
	}

	scopeCfg := tgbotapi.NewSetMyCommandsWithScope(tgbotapi.NewBotCommandScopeAllPrivateChats(), commands...)
	if _, err := b.api.Request(scopeCfg); err != nil {
		log.Printf("failed to register all_private_chats bot commands: %v", err)
	}
}

func (b *Bot) Stop() {
	b.stopOnce.Do(func() {
		b.api.StopReceivingUpdates()
		select {
		case <-b.stopChan:
		default:
			close(b.stopChan)
		}

		b.wg.Wait()
		_ = b.sessionStore.Persist()
		log.Println("AgyRemoteTele bot gracefully shut down and sessions persisted.")
	})
}
