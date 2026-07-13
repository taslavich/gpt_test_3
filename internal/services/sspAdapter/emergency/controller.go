package emergency

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
)

type Controller struct {
	urls   []string
	client *http.Client
	bot    *utils.BotMessage
}

func NewController(urls []string, timeout time.Duration, bot *utils.BotMessage) *Controller {
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	return &Controller{urls: urls, client: &http.Client{Timeout: timeout}, bot: bot}
}
func (c *Controller) StopAndNotify(ctx context.Context, summary string) error {
	statuses := []string{}
	var first error
	for _, u := range c.urls {
		req, err := http.NewRequestWithContext(ctx, http.MethodPut, u, bytes.NewBufferString(""))
		if err == nil {
			q := req.URL.Query()
			q.Set("work", "false")
			req.URL.RawQuery = q.Encode()
			resp, err := c.client.Do(req)
			if err == nil {
				statuses = append(statuses, fmt.Sprintf("%s=%d", u, resp.StatusCode))
				_ = resp.Body.Close()
				if resp.StatusCode >= 300 && first == nil {
					first = fmt.Errorf("%s status %d", u, resp.StatusCode)
				}
			}
		}
		if err != nil {
			statuses = append(statuses, fmt.Sprintf("%s=%v", u, err))
			if first == nil {
				first = err
			}
		}
	}
	text := summary + "; adv_statuses=" + strings.Join(statuses, ",")
	if c.bot != nil {
		_ = c.bot.SendTextMessageToBot(ctx, text)
	}
	return first
}
