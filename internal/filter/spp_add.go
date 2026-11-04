package filter

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"strings"

	"github.com/google/uuid"
	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
)

type sppAdd struct {
	Field    string          `json:"field"`
	Value    json.RawMessage `json:"value,omitempty"`
	PoolSize *int            `json:"pool_size,omitempty"`
	RuleID   string          `json:"rule_id,omitempty"`
}

type sppAddRandom struct {
	Field    string `json:"field"`
	PoolSize *int   `json:"pool_size,omitempty"`
	RuleID   string `json:"rule_id,omitempty"`
}

type sppCIDAdd struct {
	value  string
	random bool
	pool   []string
}

const maxSPPCIDPoolSize = 1000

func compileSPPCIDAdd(settings VersionedSPPSettings, scope string) (sppCIDAdd, error) {
	var add sppCIDAdd
	for _, rule := range settings.Add {
		if rule.Field != "bid.cid" || add.value != "" || add.random {
			return add, fmt.Errorf("add: expected a single bid.cid rule, got %q", rule.Field)
		}
		if rule.PoolSize != nil {
			if len(rule.Value) != 0 {
				return add, fmt.Errorf("add: bid.cid requires either value or pool_size")
			}
			var err error
			add, err = compileSPPCIDPool(scope, rule.RuleID, *rule.PoolSize)
			if err != nil {
				return add, fmt.Errorf("add: %w", err)
			}
		} else if rule.RuleID != "" {
			return add, fmt.Errorf("add: rule_id requires pool_size")
		} else if err := json.Unmarshal(rule.Value, &add.value); err != nil || strings.TrimSpace(add.value) == "" {
			return add, fmt.Errorf("add: bid.cid value must be a nonempty string")
		}
	}
	for _, rule := range settings.AddRandom {
		if rule.Field != "bid.cid" || add.random || add.value != "" {
			return add, fmt.Errorf("add_random: expected a single bid.cid rule without add, got %q", rule.Field)
		}
		if rule.PoolSize != nil {
			var err error
			add, err = compileSPPCIDPool(scope, rule.RuleID, *rule.PoolSize)
			if err != nil {
				return add, fmt.Errorf("add_random: %w", err)
			}
		} else if rule.RuleID != "" {
			return add, fmt.Errorf("add_random: rule_id requires pool_size")
		} else {
			add.random = true
		}
	}
	return add, nil
}

func compileSPPCIDPool(scope, ruleID string, size int) (sppCIDAdd, error) {
	if size < 1 || size > maxSPPCIDPoolSize {
		return sppCIDAdd{}, fmt.Errorf("pool_size must be in [1, %d]", maxSPPCIDPoolSize)
	}
	if ruleID != strings.TrimSpace(ruleID) {
		return sppCIDAdd{}, fmt.Errorf("rule_id must not have leading or trailing whitespace")
	}
	add := sppCIDAdd{random: true, pool: make([]string, size)}
	for i := range add.pool {
		// Keep already deployed pools unchanged when rule_id is absent.
		name := fmt.Sprintf("twinbid:spp:bid.cid:%s:%d", scope, i)
		if ruleID != "" {
			name = fmt.Sprintf("twinbid:spp-rule:bid.cid:%q:%q:%d", scope, ruleID, i)
		}
		add.pool[i] = uuid.NewSHA1(uuid.NameSpaceOID, []byte(name)).String()
	}
	return add, nil
}

// addMissingSPPFieldsV25 fills absent CID values for the configured SSP domain.
func (fp *OptimizedFilterProcessor) addMissingSPPFieldsV25(domain string, resp *ortb.BidResponse) {
	if fp == nil || fp.ruleManager == nil || resp == nil {
		return
	}
	rm := fp.ruleManager
	rm.mu.RLock()
	add := rm.sppDomainAdds[domain]
	rm.mu.RUnlock()
	if add.value == "" && !add.random {
		return
	}
	for _, seat := range resp.GetSeatbid() {
		for _, bid := range seat.GetBid() {
			if bid == nil || strings.TrimSpace(bid.GetCid()) != "" {
				continue
			}
			cid := add.value
			if len(add.pool) != 0 {
				cid = add.pool[rand.IntN(len(add.pool))]
			} else if add.random {
				cid = uuid.NewString()
			}
			bid.Cid = &cid
		}
	}
}
