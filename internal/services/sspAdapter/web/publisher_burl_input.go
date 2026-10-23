package sppAdapterWeb

import (
	"context"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/billing"
)

type publisherWinnerKey struct{}
type publisherDSPGuardKey struct{}
type publisherBurlKey struct{}
type publisherBurlFunc func(context.Context, publisherBURLInput)

// Same JSON field names as existing persisted Publisher jobs. Old optional
// destination/NURL/SSP fields are ignored; no global callback context is loaded.
type publisherBURLRecord struct {
	Format      string `json:"format"`
	BURL        string `json:"burl"`
	WinnerType  string `json:"winner_type"`
	PublisherID string `json:"publisher_id,omitempty"`
}
type publisherBURLInput struct {
	Record publisherBURLRecord
	Winner billing.Winner
	Redis  *redis.Client
	ID     string
}

func publisherTaskID(id string) bool {
	u, e := uuid.Parse(id)
	return e == nil && u != uuid.Nil && u.String() == id
}
