package billing

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/redis/go-redis/v9"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
)

type Winner struct {
	Price                      float64
	UserID, CampaignID, Format string
}
type WinnerStore struct{ client *redis.Client }

func NewWinnerStore(client *redis.Client) *WinnerStore { return &WinnerStore{client: client} }
func (s *WinnerStore) Lookup(ctx context.Context, uuid string) (Winner, bool, error) {
	if s == nil || s.client == nil {
		return Winner{}, false, fmt.Errorf("winner redis client is nil")
	}
	m, err := s.client.HGetAll(ctx, uuid).Result()
	if err != nil {
		return Winner{}, false, err
	}
	if len(m) == 0 {
		return Winner{}, false, nil
	}
	price, err := strconv.ParseFloat(m["price"], 64)
	if err != nil || price <= 0 || math.IsNaN(price) || math.IsInf(price, 0) {
		return Winner{}, false, fmt.Errorf("invalid winner price")
	}
	w := Winner{Price: price, UserID: m["user_id"], CampaignID: m["campaign_id"], Format: strings.ToUpper(m["format"])}
	if w.UserID == "" || w.CampaignID == "" {
		return Winner{}, false, fmt.Errorf("invalid winner ids")
	}
	switch w.Format {
	case constants.IPP, constants.POP, constants.BAN, constants.NAT:
		return w, true, nil
	default:
		return Winner{}, false, fmt.Errorf("unsupported winner format %q", w.Format)
	}
}
