package services

import (
	"testing"
	"time"
)

func TestRedisWriteErrorsPerTick(t *testing.T) {
	tests := []struct {
		name            string
		thresholdPerSec uint64
		tickerInterval  time.Duration
		want            uint64
	}{
		{
			name:            "one second keeps threshold",
			thresholdPerSec: 200,
			tickerInterval:  time.Second,
			want:            200,
		},
		{
			name:            "five seconds multiplies threshold",
			thresholdPerSec: 200,
			tickerInterval:  5 * time.Second,
			want:            1000,
		},
		{
			name:            "fractional interval rounds up",
			thresholdPerSec: 201,
			tickerInterval:  1500 * time.Millisecond,
			want:            302,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := redisWriteErrorsPerTick(tt.thresholdPerSec, tt.tickerInterval)
			if got != tt.want {
				t.Fatalf("redisWriteErrorsPerTick(%d, %s) = %d, want %d", tt.thresholdPerSec, tt.tickerInterval, got, tt.want)
			}
		})
	}
}
