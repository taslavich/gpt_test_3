package auction

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync/atomic"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/types"
)

type PercentMap map[string]map[string]map[string]*types.PercentAndBidfloor
type percentData struct {
	Adult      PercentMap
	Mainstream PercentMap
}
type PercentStore struct {
	value                     atomic.Value
	adultFile, mainstreamFile string
}

func NewPercentStore(adultFile, mainstreamFile string) *PercentStore {
	p := &PercentStore{adultFile: adultFile, mainstreamFile: mainstreamFile}
	p.value.Store(&percentData{Adult: PercentMap{}, Mainstream: PercentMap{}})
	return p
}
func (p *PercentStore) LoadInitial() error {
	d := &percentData{Adult: PercentMap{}, Mainstream: PercentMap{}}
	if p.adultFile != "" {
		m, err := readPercentFile(p.adultFile)
		if err != nil {
			return err
		}
		d.Adult = m
	}
	if p.mainstreamFile != "" {
		m, err := readPercentFile(p.mainstreamFile)
		if err != nil {
			return err
		}
		d.Mainstream = m
	}
	p.value.Store(d)
	return nil
}
func readPercentFile(path string) (PercentMap, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m PercentMap
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	if err := validatePercentMap(m); err != nil {
		return nil, err
	}
	return clonePercentMap(m), nil
}
func validatePercentMap(m PercentMap) error {
	for ssp, countries := range m {
		if strings.TrimSpace(ssp) == "" {
			return fmt.Errorf("empty ssp")
		}
		for country, users := range countries {
			if strings.TrimSpace(country) == "" {
				return fmt.Errorf("empty country")
			}
			for user, v := range users {
				if strings.TrimSpace(user) == "" || v == nil {
					return fmt.Errorf("invalid percent entry")
				}
				if v.Percent < 0 || v.Percent > 1 {
					return fmt.Errorf("percent out of range")
				}
			}
		}
	}
	return nil
}
func clonePercentMap(in PercentMap) PercentMap {
	out := make(PercentMap, len(in))
	for ssp, countries := range in {
		out[ssp] = make(map[string]map[string]*types.PercentAndBidfloor, len(countries))
		for c, users := range countries {
			out[ssp][c] = make(map[string]*types.PercentAndBidfloor, len(users))
			for u, v := range users {
				if v != nil {
					vv := *v
					out[ssp][c][u] = &vv
				}
			}
		}
	}
	return out
}
func (p *PercentStore) Update(traffic string, m PercentMap) error {
	if err := validatePercentMap(m); err != nil {
		return err
	}
	cur := p.value.Load().(*percentData)
	next := &percentData{Adult: clonePercentMap(cur.Adult), Mainstream: clonePercentMap(cur.Mainstream)}
	if strings.EqualFold(traffic, "ADULT") {
		next.Adult = clonePercentMap(m)
	} else {
		next.Mainstream = clonePercentMap(m)
	}
	p.value.Store(next)
	return nil
}
func (p *PercentStore) Get(traffic string) PercentMap {
	if p == nil {
		return PercentMap{}
	}
	cur := p.value.Load().(*percentData)
	if strings.EqualFold(traffic, "ADULT") {
		return clonePercentMap(cur.Adult)
	}
	return clonePercentMap(cur.Mainstream)
}
func (p *PercentStore) Lookup(traffic, ssp, country, user string) float64 {
	m := p.Get(traffic)
	if countries := m[ssp]; countries != nil {
		if users := countries[country]; users != nil {
			if v := users[user]; v != nil {
				return float64(v.Percent)
			}
		}
	}
	return 0
}
