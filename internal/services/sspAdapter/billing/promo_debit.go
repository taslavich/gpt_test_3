package billing

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const cabinetPromoSpendPath = "/api/internal/percenter/promo-spend"

type PromoState struct {
	Remaining  float64 `json:"remaining"`
	Revision   int64   `json:"revision"`
	Generation int64   `json:"generation"`
}

type PromoDebitFunc func(context.Context, string, string, string, int64, float64) (PromoState, error)

type cabinetPromoSpendRequest struct {
	EventID         string  `json:"event_id"`
	UserID          string  `json:"user_id"`
	CampaignID      string  `json:"campaign_id"`
	PromoGeneration int64   `json:"promo_generation"`
	SpendDelta      float64 `json:"spend_delta"`
}

type cabinetPromoSpendEnvelope struct {
	Success  bool       `json:"success"`
	ErrorMsg string     `json:"errorMsg"`
	Data     PromoState `json:"data"`
}

// NewHTTPPromoDebit calls the cabinet backend, which owns the authoritative
// users.promo_spend_remaining/promo_revision PostgreSQL state. The same stable
// billing event_id is sent on every retry, so the cabinet can apply the debit
// idempotently while ORTB keeps the durable write-ahead intent.
func NewHTTPPromoDebit(baseURL, internalSecret string) PromoDebitFunc {
	baseURL = strings.TrimSpace(baseURL)
	internalSecret = strings.TrimSpace(internalSecret)
	client := &http.Client{Timeout: 3 * time.Second}

	return func(ctx context.Context, eventID, userID, campaignID string, promoGeneration int64, spendDelta float64) (PromoState, error) {
		if baseURL == "" {
			return PromoState{}, fmt.Errorf("cabinet promo debit has empty base URL")
		}
		if internalSecret == "" {
			return PromoState{}, fmt.Errorf("cabinet promo debit has empty internal secret")
		}
		if strings.TrimSpace(eventID) == "" || strings.TrimSpace(userID) == "" || strings.TrimSpace(campaignID) == "" {
			return PromoState{}, fmt.Errorf("cabinet promo debit has empty event/user/campaign id")
		}
		if promoGeneration < 0 {
			return PromoState{}, fmt.Errorf("cabinet promo debit has invalid promo_generation %d", promoGeneration)
		}
		if spendDelta <= 0 || math.IsNaN(spendDelta) || math.IsInf(spendDelta, 0) {
			return PromoState{}, fmt.Errorf("cabinet promo debit has invalid spend_delta %v", spendDelta)
		}

		requestURL, err := buildCabinetPromoSpendURL(baseURL)
		if err != nil {
			return PromoState{}, err
		}
		payload, err := json.Marshal(cabinetPromoSpendRequest{
			EventID: eventID, UserID: userID, CampaignID: campaignID, PromoGeneration: promoGeneration, SpendDelta: spendDelta,
		})
		if err != nil {
			return PromoState{}, fmt.Errorf("marshal cabinet promo spend: %w", err)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, bytes.NewReader(payload))
		if err != nil {
			return PromoState{}, fmt.Errorf("build cabinet promo spend request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Bot-Secret", internalSecret)

		resp, err := client.Do(req)
		if err != nil {
			return PromoState{}, fmt.Errorf("cabinet promo spend request: %w", err)
		}
		defer resp.Body.Close()
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		if readErr != nil {
			return PromoState{}, fmt.Errorf("read cabinet promo spend response: %w", readErr)
		}
		var envelope cabinetPromoSpendEnvelope
		if err := json.Unmarshal(body, &envelope); err != nil {
			return PromoState{}, fmt.Errorf("decode cabinet promo spend response (HTTP %d): %w", resp.StatusCode, err)
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 || !envelope.Success {
			if strings.TrimSpace(envelope.ErrorMsg) == "" {
				envelope.ErrorMsg = http.StatusText(resp.StatusCode)
			}
			return PromoState{}, fmt.Errorf("cabinet promo spend failed: HTTP %d: %s", resp.StatusCode, envelope.ErrorMsg)
		}
		if envelope.Data.Remaining < 0 || math.IsNaN(envelope.Data.Remaining) || math.IsInf(envelope.Data.Remaining, 0) || envelope.Data.Revision < 0 || envelope.Data.Generation < 0 {
			return PromoState{}, fmt.Errorf("cabinet promo spend returned invalid state: remaining=%v revision=%d generation=%d", envelope.Data.Remaining, envelope.Data.Revision, envelope.Data.Generation)
		}
		return envelope.Data, nil
	}
}

func buildCabinetPromoSpendURL(baseURL string) (string, error) {
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		baseURL = "http://" + baseURL
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("parse cabinet backend URL: %w", err)
	}
	if u.Host == "" {
		return "", fmt.Errorf("cabinet backend URL has no host")
	}
	u.Path = cabinetPromoSpendPath
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}
