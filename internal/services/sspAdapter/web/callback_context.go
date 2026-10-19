package sppAdapterWeb

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/callback"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/billing"
)

type callbackDSPFallbackKey struct{}
type callbackKey struct{}
type resolvedCallback struct {
	Record callback.Record
	Winner billing.Winner
	Redis  *redis.Client
	ID     string
}

func resolveCallback(ctx context.Context, id string, dsp *redis.Client, adv *billing.Store, fallback ...*redis.Client) (resolvedCallback, error) {
	// Reserve a bounded slice for the existing second DSP hash only on failure.
	first, cancel := context.WithTimeout(ctx, 1300*time.Millisecond)
	out, err := resolvePrimaryCallback(first, id, dsp, adv)
	cancel()
	if err == nil || !callback.ValidID(id) || len(fallback) == 0 || fallback[0] == nil || fallback[0] == dsp {
		return out, err
	}
	lookup, stop := context.WithTimeout(ctx, 650*time.Millisecond)
	defer stop()
	fields, e := fallback[0].HGetAll(lookup, id).Result()
	if e == nil {
		var r callback.Record
		r, e = callback.Decode(fields)
		if e == nil && r.WinnerType == "dsp" {
			return resolvedCallback{Record: r, Redis: fallback[0], ID: id}, nil
		}
	}
	if errors.Is(err, redis.Nil) && e != nil && !errors.Is(e, redis.Nil) {
		return out, e
	}
	return out, err
}
func resolvePrimaryCallback(ctx context.Context, id string, dsp *redis.Client, adv *billing.Store) (resolvedCallback, error) {
	out := resolvedCallback{ID: id}
	if !callback.ValidID(id) {
		return out, errors.New("invalid callback UUID")
	}
	lookup, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var dspErr error
	if dsp != nil {
		first, firstCancel := context.WithTimeout(lookup, 650*time.Millisecond)
		fields, e := dsp.HGetAll(first, id).Result()
		firstCancel()
		if e == nil {
			r, decodeErr := callback.Decode(fields)
			if decodeErr == nil {
				if r.WinnerType != "dsp" {
					return out, errors.New("DSP callback type mismatch")
				}
				out.Record = r
				out.Redis = dsp
				return out, nil
			}
			if !errors.Is(decodeErr, redis.Nil) {
				return out, decodeErr
			}
		} else {
			dspErr = e
		}
	} else {
		dspErr = errors.New("DSP winner Redis unavailable")
	}
	// UUIDs are globally unique and only a finalized ADV has callback_context_v1.
	// A DSP Redis outage must not hide an intact winner on the separate ADV Redis.
	if adv == nil {
		if dspErr != nil {
			return out, dspErr
		}
		return out, redis.Nil
	}
	winner, e := adv.ReadWinner(lookup, id, "")
	if e != nil {
		if dspErr != nil {
			return out, fmt.Errorf("DSP and ADV lookup failed: %v; %v", dspErr, e)
		}
		return out, e
	}
	if winner.Callback.Format == "" {
		if dspErr != nil {
			return out, dspErr
		}
		return out, redis.Nil
	}
	out.Record = winner.Callback
	out.Winner = winner
	out.Redis = adv.CallbackRedis()
	return out, nil
}
func callbackInput(w http.ResponseWriter, r *http.Request, id string, dsp *redis.Client, adv *billing.Store) (*http.Request, resolvedCallback, bool) {
	if !callback.ValidID(id) {
		http.Error(w, "invalid callback UUID", 400)
		return r, resolvedCallback{}, false
	}
	// Extra legacy query fields can be present during deployment but are ignored.
	if ids := r.URL.Query()["id"]; len(ids) > 1 {
		http.Error(w, "ambiguous callback UUID", 400)
		return r, resolvedCallback{}, false
	}
	var backup *redis.Client
	if c, ok := r.Context().Value(callbackDSPFallbackKey{}).(*redis.Client); ok {
		backup = c
	}
	resolved, e := resolveCallback(r.Context(), id, dsp, adv, backup)
	if e != nil {
		log.Printf("callback lookup failed uuid=%s: %v", id, e)
		status := 503
		if errors.Is(e, redis.Nil) {
			status = 410
		}
		http.Error(w, "callback context unavailable", status)
		return r, resolved, false
	}
	return r.WithContext(context.WithValue(r.Context(), callbackKey{}, resolved)), resolved, true
}

type publisherDSPGuardKey struct{}
type publisherBurlKey struct{}
type publisherBurlFunc func(context.Context, resolvedCallback)
