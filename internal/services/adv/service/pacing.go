package auction

import (
	"context"
	"sort"
	"time"
)

func ActiveSlotsLeft(campaign *Campaign, now time.Time) float64 {
	if campaign == nil {
		return 0
	}
	from := now.UTC()
	if campaign.StartTS.After(from) {
		from = campaign.StartTS
	}
	if !from.Before(campaign.EndTS) {
		return 0
	}
	if len(campaign.ActiveIntervals) == 0 {
		return campaign.EndTS.Sub(from).Seconds() / SlotDuration.Seconds()
	}

	intervals := make([]TimeRange, 0, len(campaign.ActiveIntervals))
	for _, interval := range campaign.ActiveIntervals {
		start := maxTime(interval.Start.UTC(), from)
		end := minTime(interval.End.UTC(), campaign.EndTS.UTC())
		if start.Before(end) {
			intervals = append(intervals, TimeRange{Start: start, End: end})
		}
	}
	if len(intervals) == 0 {
		return 0
	}
	sort.Slice(intervals, func(i, j int) bool { return intervals[i].Start.Before(intervals[j].Start) })
	merged := intervals[:1]
	for _, interval := range intervals[1:] {
		last := &merged[len(merged)-1]
		if !interval.Start.After(last.End) {
			if interval.End.After(last.End) {
				last.End = interval.End
			}
			continue
		}
		merged = append(merged, interval)
	}
	var seconds float64
	for _, interval := range merged {
		seconds += interval.End.Sub(interval.Start).Seconds()
	}
	return seconds / SlotDuration.Seconds()
}

func (s *AuctionService) StartPacingTicker(ctx context.Context, interval time.Duration, onError func(error)) {
	if s == nil || s.runtime == nil {
		return
	}
	if interval <= 0 {
		interval = time.Second
	}
	run := func() {
		if err := s.runtime.UpdatePacing(ctx, s.snapshot.Load(), time.Now()); err != nil && onError != nil {
			onError(err)
		}
	}
	run()
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				run()
			}
		}
	}()
}
