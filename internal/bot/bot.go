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
