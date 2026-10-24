package sppAdapterWeb

import (
	"context"
	"errors"
	"github.com/redis/go-redis/v9"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/pubuuid"
	"net/http"
	"os"
	"time"
)

func trustedPublisherUUID(ctx context.Context, r *http.Request, in *postBidRequest_V2_5, format string, clients []*redis.Client) (string, error) {
	feedAllowed := pubuuid.FeedAllowed(os.Getenv("PUB_UUID_FEEDS"), in.Feed)
	id := r.Header.Get(pubuuid.Header)
	// Ordinary feeds ignore these headers completely and always generate their own UUID.
	if !feedAllowed {
		return "", nil
	}
	// A configured pub feed must be signed; knowing its ordinary feed token is insufficient.
	if format != "POP" || in.Payload == nil {
		return "", errors.New("POP required")
	}
	signature := r.Header.Get(pubuuid.SignatureHeader)
	if e := pubuuid.Validate(os.Getenv("PUB_INTERNAL_SECRET"), id, in.Feed, r.Header.Get(pubuuid.TimeHeader), signature, in.Payload.BidRequest, time.Now()); e != nil {
		return "", e
	}
	if e := pubuuid.Claim(ctx, clients, id, signature); e != nil {
		return "", e
	}
	return id, nil
}

// UUID lifecycle only: original error exits (e.g. a failed Orchestrator RPC)
// have no ORTB commit. For an admitted UUID, finish through the ORIGINAL writer
// unless a ready-set write has already been attempted. An unknown SADD outcome
// is never retried here: the loader may already have consumed it.
type publisherCommitKey struct{}
type publisherCommitState struct{ attempted bool }

func publisherCommitAttempt(ctx context.Context) {
	if state, ok := ctx.Value(publisherCommitKey{}).(*publisherCommitState); ok {
		state.attempted = true
	}
}

type publisherResponse struct {
	http.ResponseWriter
	status int
}

func (w *publisherResponse) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}
func (w *publisherResponse) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = 200
	}
	return w.ResponseWriter.Write(b)
}
