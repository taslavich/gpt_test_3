package auction

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
)

type QualitySegment string

const (
	QualitySegmentUsual QualitySegment = "usual"
	QualitySegmentHigh  QualitySegment = "high"
	QualitySegmentUltra QualitySegment = "ultra"
)

type QualityStore struct{ value atomic.Value }
type qualityData struct {
	bySegment map[string][]string
	byDomain  map[string]string
}

func NewQualityStore() *QualityStore {
	q := &QualityStore{}
	q.value.Store(&qualityData{bySegment: map[string][]string{}, byDomain: map[string]string{}})
	return q
}
func LoadQualityStore(path string) (*QualityStore, error) {
	q := NewQualityStore()
	if err := q.LoadFile(path); err != nil {
		return nil, err
	}
	return q, nil
}
func (q *QualityStore) LoadFile(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var raw map[string][]string
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	return q.Replace(raw)
}
func (q *QualityStore) Replace(raw map[string][]string) error {
	data := &qualityData{bySegment: map[string][]string{}, byDomain: map[string]string{}}
	for seg, domains := range raw {
		nseg := strings.ToLower(strings.TrimSpace(seg))
		if nseg != "usual" && nseg != "high" && nseg != "ultra" {
			return fmt.Errorf("invalid quality segment %q", seg)
		}
		for _, d := range domains {
			nd := strings.ToLower(strings.TrimSpace(d))
			if nd == "" {
				continue
			}
			if prev, ok := data.byDomain[nd]; ok && prev != nseg {
				return fmt.Errorf("quality domain %q in both %s and %s", nd, prev, nseg)
			}
			data.byDomain[nd] = nseg
			data.bySegment[nseg] = append(data.bySegment[nseg], nd)
		}
	}
	q.value.Store(data)
	return nil
}
func (q *QualityStore) Segment(domain string) string {
	if q == nil {
		return ""
	}
	data := q.value.Load().(*qualityData)
	return data.byDomain[strings.ToLower(strings.TrimSpace(domain))]
}
func (q *QualityStore) Domains(segment string) []string {
	if q == nil {
		return nil
	}
	data := q.value.Load().(*qualityData)
	return append([]string(nil), data.bySegment[strings.ToLower(strings.TrimSpace(segment))]...)
}
