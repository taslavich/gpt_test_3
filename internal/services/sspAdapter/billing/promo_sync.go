package billing

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const promoSyncPath = "/internal/promo-spend-remaining"

type PromoSyncFunc func(context.Context, string, float64, int64, int64) error

type promoSyncRequest struct {
	UserID     string  `json:"user_id"`
	Remaining  float64 `json:"remaining"`
	Revision   int64   `json:"revision"`
	Generation int64   `json:"generation"`
}

// NewHTTPPromoSync returns a synchronous durability boundary for the billing
// path: PostgreSQL has already committed when it runs, and Apply does not ACK
// the durable intent until every configured ADV instance has accepted the
// local runtime update. No network I/O is added to the auction hot path.
func NewHTTPPromoSync(endpoints []string) PromoSyncFunc {
	clean := make([]string, 0, len(endpoints))
	for _, raw := range endpoints {
		if endpoint := strings.TrimSpace(raw); endpoint != "" {
			clean = append(clean, endpoint)
		}
	}

	client := &http.Client{Timeout: 3 * time.Second}
	return func(ctx context.Context, userID string, remaining float64, revision, generation int64) error {
		userID = strings.TrimSpace(userID)
		if userID == "" {
			return fmt.Errorf("promo runtime sync has empty user_id")
		}
		if revision < 0 {
			return fmt.Errorf("promo runtime sync has invalid revision %d", revision)
		}
		if generation < 0 {
			return fmt.Errorf("promo runtime sync has invalid generation %d", generation)
		}
		if len(clean) == 0 {
			return fmt.Errorf("promo runtime sync has no ADV endpoints")
		}
		payload, err := json.Marshal(promoSyncRequest{UserID: userID, Remaining: remaining, Revision: revision, Generation: generation})
		if err != nil {
			return fmt.Errorf("marshal promo runtime sync: %w", err)
		}

		type result struct {
			endpoint string
			err      error
		}
		results := make(chan result, len(clean))
		var wg sync.WaitGroup
		for _, endpoint := range clean {
			endpoint := endpoint
			wg.Add(1)
			go func() {
				defer wg.Done()
				requestURL, err := buildADVPromoSyncURL(endpoint)
				if err != nil {
					results <- result{endpoint: endpoint, err: err}
					return
				}
				req, err := http.NewRequestWithContext(ctx, http.MethodPut, requestURL, bytes.NewReader(payload))
				if err != nil {
					results <- result{endpoint: endpoint, err: err}
					return
				}
				req.Header.Set("Content-Type", "application/json")
				resp, err := client.Do(req)
				if err != nil {
					results <- result{endpoint: endpoint, err: err}
					return
				}
				_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
				_ = resp.Body.Close()
				if resp.StatusCode < 200 || resp.StatusCode >= 300 {
					results <- result{endpoint: endpoint, err: fmt.Errorf("HTTP %d", resp.StatusCode)}
					return
				}
				results <- result{endpoint: endpoint}
			}()
		}
		wg.Wait()
		close(results)

		failures := make([]string, 0)
		for item := range results {
			if item.err != nil {
				failures = append(failures, fmt.Sprintf("%s: %v", item.endpoint, item.err))
			}
		}
		if len(failures) > 0 {
			return fmt.Errorf("promo runtime sync failed: %s", strings.Join(failures, "; "))
		}
		return nil
	}
}

func buildADVPromoSyncURL(endpoint string) (string, error) {
	endpoint = strings.TrimSpace(endpoint)
	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		endpoint = "http://" + endpoint
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	if u.Host == "" {
		return "", fmt.Errorf("ADV endpoint has no host")
	}
	u.Path = promoSyncPath
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}
