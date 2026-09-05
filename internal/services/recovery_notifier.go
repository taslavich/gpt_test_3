package services

import (
	"context"
	"log"
	"sync"
	"time"

	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
)

type RecoveryNotifier struct {
	mu        sync.Mutex
	bot       *utils.BotMessage
	interval  time.Duration
	failing   bool
	lastAlert time.Time
}

func NewRecoveryNotifier(bot *utils.BotMessage, interval time.Duration) *RecoveryNotifier {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	return &RecoveryNotifier{bot: bot, interval: interval}
}

func (n *RecoveryNotifier) Failure(ctx context.Context, message string) {
	if n == nil {
		return
	}
	now := time.Now()
	n.mu.Lock()
	shouldSend := !n.failing || n.lastAlert.IsZero() || now.Sub(n.lastAlert) >= n.interval
	n.failing = true
	if shouldSend {
		n.lastAlert = now
	}
	n.mu.Unlock()
	if !shouldSend || n.bot == nil {
		return
	}
	if err := n.bot.SendTextMessageToBot(ctx, message); err != nil {
		log.Printf("failed to send recovery notifier alert: %v", err)
	}
}

func (n *RecoveryNotifier) Recovered(ctx context.Context, message string) {
	if n == nil {
		return
	}
	n.mu.Lock()
	wasFailing := n.failing
	n.failing = false
	n.lastAlert = time.Time{}
	n.mu.Unlock()
	if !wasFailing || n.bot == nil {
		return
	}
	if err := n.bot.SendTextMessageToBot(ctx, message); err != nil {
		log.Printf("failed to send recovery notifier recovery: %v", err)
	}
}
