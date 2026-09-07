package percenter

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestShardedCachePreservesTTLAndCampaignValidation(t *testing.T) {
	policy := Policy{ADVCacheTTL: 5 * time.Second}.Normalize()
	store := NewStateStore(nil, policy)
	now := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	state := BaselineStateForCampaign("00abcdef", "c1", 1, 0.2, 4, TypeModelSmart, ProfitModelImpression, now)
	store.putCache(state, now)

	if _, ok := store.cachedForCampaign(state.SegmentHash, 1, 0.2, 4, TypeModelSmart, ProfitModelImpression, now.Add(time.Second)); !ok {
		t.Fatal("fresh cache entry was not found")
	}
	if _, ok := store.cachedForCampaign(state.SegmentHash, 1, 0.2, 5, TypeModelSmart, ProfitModelImpression, now.Add(time.Second)); ok {
		t.Fatal("cache returned state for wrong campaign version")
	}
	store.putCache(state, now)
	if _, ok := store.cachedForCampaign(state.SegmentHash, 1, 0.2, 4, TypeModelSmart, ProfitModelImpression, now.Add(6*time.Second)); ok {
		t.Fatal("expired cache entry was returned")
	}
	if cacheShardIndex("00abcdef") == cacheShardIndex("ffabcdef") {
		t.Fatal("hex hashes expected to distribute across cache shards")
	}
}

func TestShardedCacheConcurrentAccess(t *testing.T) {
	policy := Policy{ADVCacheTTL: 5 * time.Second}.Normalize()
	store := NewStateStore(nil, policy)
	now := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	const workers = 32
	const perWorker = 256
	var wg sync.WaitGroup
	wg.Add(workers)
	for worker := 0; worker < workers; worker++ {
		worker := worker
		go func() {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				hash := fmt.Sprintf("%02x%062x", worker, i)
				state := BaselineStateForCampaign(hash, "campaign", 1, 0.2, 1, TypeModelSmart, ProfitModelImpression, now)
				store.putCache(state, now)
				if _, ok := store.cachedForCampaign(hash, 1, 0.2, 1, TypeModelSmart, ProfitModelImpression, now.Add(time.Second)); !ok {
					t.Errorf("cache miss for worker=%d item=%d", worker, i)
					return
				}
			}
		}()
	}
	wg.Wait()
}
