package utils

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type BotMessage struct {
	botURL         string
	internalSecret string
	client         *http.Client
}

func NewBotMessage(botURL, internalSecret string) *BotMessage {
	return NewBotMessageWithTimeout(botURL, internalSecret, 5*time.Second)
}

func NewBotMessageWithTimeout(botURL, internalSecret string, timeout time.Duration) *BotMessage {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &BotMessage{
		botURL:         strings.TrimRight(strings.TrimSpace(botURL), "/"),
		internalSecret: strings.TrimSpace(internalSecret),
		client:         &http.Client{Timeout: timeout},
	}
}

func (b *BotMessage) SendTextMessageToBot(ctx context.Context, text string) error {
	if b == nil || b.client == nil {
		return fmt.Errorf("telegram bot client is nil")
	}
	if b.botURL == "" || b.internalSecret == "" {
		return fmt.Errorf("telegram bot URL or internal secret is empty")
	}
	body := []byte(fmt.Sprintf(`{"text":%q}`, text))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.botURL+"/internal/messages/send", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Bot-Secret", b.internalSecret)
	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("telegram bot returned status %d", resp.StatusCode)
	}
	return nil
}
