package service

import (
	"fmt"

	"wm-api/internal/config"
)

type FeedResolver struct {
	feeds map[string]string
}

func NewFeedResolver(cfg *config.WmAPIConfig) *FeedResolver {
	feeds := make(map[string]string)
	merge := func(items config.MapStringToString) {
		for uuid, domain := range items {
			feeds[uuid] = domain
		}
	}

	merge(cfg.SspPopAdlFeeds)
	merge(cfg.SspPopMcFeeds)
	merge(cfg.SspBanAdlFeeds)
	merge(cfg.SspBanMcFeeds)
	merge(cfg.SspNatAdlFeeds)
	merge(cfg.SspNatMcFeeds)
	merge(cfg.SspIppAdlFeeds)
	merge(cfg.SspIppMcFeeds)

	return &FeedResolver{feeds: feeds}
}

func (r *FeedResolver) Resolve(feedUUID string) (string, error) {
	domain, ok := r.feeds[feedUUID]
	if !ok || domain == "" {
		return "", fmt.Errorf("unknown feed uuid: %s", feedUUID)
	}
	return domain, nil
}
