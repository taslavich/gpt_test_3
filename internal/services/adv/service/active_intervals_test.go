package auction

import (
	"testing"
	"time"
)

func TestParseActiveIntervalScheduleInclusiveHourBounds(t *testing.T) {
	windowStart := time.Date(2026, time.July, 6, 0, 0, 0, 0, time.UTC) // Monday
	windowEnd := windowStart.Add(7 * 24 * time.Hour)

	intervals, err := ParseActiveIntervalSchedule([][]string{
		{"mon,4", "mon,4"},
		{"mon,6", "tue,10"},
		{"tue,18", "sun,23"},
	}, windowStart, windowEnd)
	if err != nil {
		t.Fatalf("ParseActiveIntervalSchedule returned error: %v", err)
	}
	if len(intervals) != 3 {
		t.Fatalf("expected 3 intervals, got %d: %#v", len(intervals), intervals)
	}

	want := []TimeRange{
		{
			Start: time.Date(2026, time.July, 6, 4, 0, 0, 0, time.UTC),
			End:   time.Date(2026, time.July, 6, 5, 0, 0, 0, time.UTC),
		},
		{
			Start: time.Date(2026, time.July, 6, 6, 0, 0, 0, time.UTC),
			End:   time.Date(2026, time.July, 7, 11, 0, 0, 0, time.UTC),
		},
		{
			Start: time.Date(2026, time.July, 7, 18, 0, 0, 0, time.UTC),
			End:   time.Date(2026, time.July, 13, 0, 0, 0, 0, time.UTC),
		},
	}

	for i := range want {
		if !intervals[i].Start.Equal(want[i].Start) || !intervals[i].End.Equal(want[i].End) {
			t.Fatalf("interval %d mismatch: got [%s, %s), want [%s, %s)", i, intervals[i].Start, intervals[i].End, want[i].Start, want[i].End)
		}
	}
}
