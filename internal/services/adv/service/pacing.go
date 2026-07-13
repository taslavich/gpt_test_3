package auction

import (
	"context"
	"math"
	"time"
)

func calculateActiveSeconds(now time.Time, intervals []TimeRange, start, end time.Time) int64 {
	from := maxTime(now, start)
	if !from.Before(end) {
		return 0
	}
	if len(intervals) == 0 {
		return int64(end.Sub(from).Seconds())
	}
	var seconds int64
	for _, in := range intervals {
		st := maxTime(from, in.Start)
		en := minTime(end, in.End)
		if st.Before(en) {
			seconds += int64(en.Sub(st).Seconds())
		}
	}
	return seconds
}
func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}
func SlotsLeft(c *Campaign, now time.Time) int {
	sec := calculateActiveSeconds(now, c.ActiveIntervals, c.StartTS, c.EndTS)
	if sec <= 0 {
		return 0
	}
	return int(math.Ceil(float64(sec) / SlotDuration.Seconds()))
}
func SlotTarget(c *Campaign, now time.Time, campaignSpent float64) float64 {
	remaining := c.GoalTotalDollars - campaignSpent
	slots := SlotsLeft(c, now)
	if remaining <= 0 || slots <= 0 {
		return 0
	}
	return remaining / float64(slots)
}
func PacingEligible(ctx context.Context, store *RuntimeStore, c *Campaign, now time.Time, campaignSpent float64) (bool, error) {
	if !c.EvennessBySlotMode {
		return true, nil
	}
	slot := SlotID(now)
	spent, err := store.SlotSpent(ctx, c.ID, slot)
	if err != nil {
		return false, err
	}
	return spent < SlotTarget(c, now, campaignSpent), nil
}
