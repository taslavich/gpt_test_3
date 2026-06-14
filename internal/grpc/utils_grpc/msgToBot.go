package utils

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
)

type BotMessage struct {
	botURL         string
	internalSecret string
}

func NewBotMessage(botURL, internalSecret string) *BotMessage {
	return &BotMessage{
		botURL:         botURL,
		internalSecret: internalSecret,
	}
}

func (b *BotMessage) SendTextMessageToBot(ctx context.Context, text string) error {
	body := []byte(fmt.Sprintf(`{"text":%q}`, text))

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		b.botURL+"/internal/messages/send",
		bytes.NewReader(body),
	)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Bot-Secret", b.internalSecret)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("telegram bot returned status %d", resp.StatusCode)
	}

	return nil
}
