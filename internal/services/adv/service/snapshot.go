package auction

import "sync/atomic"

type Snapshot struct {
	Campaigns map[string]*Campaign
	UserGoals map[string]float64
}

type snapshotStore struct{ value atomic.Value }

func newSnapshotStore() *snapshotStore {
	s := &snapshotStore{}
	s.Store(&Snapshot{Campaigns: map[string]*Campaign{}, UserGoals: map[string]float64{}})
	return s
}
func (s *snapshotStore) Load() *Snapshot {
	if v := s.value.Load(); v != nil {
		return v.(*Snapshot)
	}
	return &Snapshot{Campaigns: map[string]*Campaign{}, UserGoals: map[string]float64{}}
}
func (s *snapshotStore) Store(snapshot *Snapshot) {
	if snapshot == nil {
		snapshot = &Snapshot{Campaigns: map[string]*Campaign{}, UserGoals: map[string]float64{}}
	}
	s.value.Store(snapshot)
}

func BuildSnapshot(campaigns map[string]*Campaign, userGoals map[string]float64) *Snapshot {
	out := &Snapshot{Campaigns: make(map[string]*Campaign, len(campaigns)), UserGoals: make(map[string]float64, len(userGoals))}
	for k, v := range userGoals {
		out.UserGoals[k] = v
	}
	for id, c := range campaigns {
		if c == nil || !c.IsValid() {
			continue
		}
		clone := c.Clone()
		out.Campaigns[id] = clone
	}
	return out
}
