package bot

import (
	"context"
	"fmt"
	"html"
	"log"
	"strings"
	"sync"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/dresar/AgyRemoteTele/internal/session"
)

type Bot struct {
	api          *tgbotapi.BotAPI
	router       *Router
	sessionStore *session.Store
	adminChatIDs []int64
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

func (b *Bot) SetAdminChatIDs(ids []int64) {
	b.adminChatIDs = ids
}

func (b *Bot) BroadcastAlert(level, title, message string) {
	if b.api == nil || len(b.adminChatIDs) == 0 {
		return
	}

	icon := "🚨"
	switch strings.ToUpper(level) {
	case "CRITICAL":
		icon = "🔴"
	case "WARNING":
		icon = "🟡"
	case "INFO":
		icon = "ℹ️"
	case "RESOLVED":
		icon = "🟢"
	}

	text := fmt.Sprintf("<b>%s WATCHDOG ALERT — %s</b>\n\n<b>Level:</b> <code>%s</code>\n<b>Message:</b> %s",
		icon, html.EscapeString(title), html.EscapeString(level), html.EscapeString(message))

	for _, chatID := range b.adminChatIDs {
		msg := tgbotapi.NewMessage(chatID, text)
		msg.ParseMode = tgbotapi.ModeHTML
		_, _ = b.api.Send(msg)
	}
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
		{Command: "ps", Description: "Daftar proses aktif sistem (top processes)"},
		{Command: "kill", Description: "Hentikan proses dengan PID: /kill <pid> [sig]"},
		{Command: "ports", Description: "Daftar port listening dan network sockets"},
		{Command: "service", Description: "Kelola status dan restart service sistem"},
		{Command: "ping", Description: "Uji konektivitas ICMP: /ping <host>"},
		{Command: "curl", Description: "Uji endpoint HTTP/HTTPS: /curl <url>"},
		{Command: "dns", Description: "Query DNS record domain: /dns <domain> [type]"},
		{Command: "netio", Description: "Statistik traffic interface jaringan (Net I/O)"},
		{Command: "docker", Description: "Kelola container Docker (ps, logs, stats, restart)"},
		{Command: "cron", Description: "Kelola tugas terjadwal / cron jobs"},
		{Command: "alerts", Description: "Lihat riwayat alert watchdog sistem"},
		{Command: "backup", Description: "Buat arsip backup tar.gz direktori"},
		{Command: "swarm", Description: "Broadcast prompt ke seluruh worker cluster"},
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
