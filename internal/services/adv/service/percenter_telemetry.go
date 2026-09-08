package auction

import (
	"sync/atomic"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services/percenter"
)

type percenterTelemetryCounters struct {
	stateGetOrInitCalls       atomic.Uint64
	stateGetOrInitSuccess     atomic.Uint64
	requestContextCanceled    atomic.Uint64
	requestDeadlineExceeded   atomic.Uint64
	stateGetOrInitOtherErrors atomic.Uint64

	backgroundRetryStarted      atomic.Uint64
	backgroundRetrySuccess      atomic.Uint64
	backgroundRetryFailed       atomic.Uint64
	backgroundRetryDeduplicated atomic.Uint64
	backgroundRetryPoolFull     atomic.Uint64

	telemetryPublishFailures  atomic.Uint64
	telemetryPublishRecovered atomic.Uint64
}

func (s *AuctionService) PercenterTelemetryTotals() percenter.TelemetryTotals {
	if s == nil {
		return percenter.TelemetryTotals{}
	}
	return percenter.TelemetryTotals{
		StateGetOrInitCalls:         s.percenterTelemetry.stateGetOrInitCalls.Load(),
		StateGetOrInitSuccess:       s.percenterTelemetry.stateGetOrInitSuccess.Load(),
		RequestContextCanceled:      s.percenterTelemetry.requestContextCanceled.Load(),
		RequestDeadlineExceeded:     s.percenterTelemetry.requestDeadlineExceeded.Load(),
		StateGetOrInitOtherErrors:   s.percenterTelemetry.stateGetOrInitOtherErrors.Load(),
		BackgroundRetryStarted:      s.percenterTelemetry.backgroundRetryStarted.Load(),
		BackgroundRetrySuccess:      s.percenterTelemetry.backgroundRetrySuccess.Load(),
		BackgroundRetryFailed:       s.percenterTelemetry.backgroundRetryFailed.Load(),
		BackgroundRetryDeduplicated: s.percenterTelemetry.backgroundRetryDeduplicated.Load(),
		BackgroundRetryPoolFull:     s.percenterTelemetry.backgroundRetryPoolFull.Load(),
		TelemetryPublishFailures:    s.percenterTelemetry.telemetryPublishFailures.Load(),
		TelemetryPublishRecovered:   s.percenterTelemetry.telemetryPublishRecovered.Load(),
	}
}

func (s *AuctionService) RecordPercenterTelemetryPublishFailure() {
	if s != nil {
		s.percenterTelemetry.telemetryPublishFailures.Add(1)
	}
}

func (s *AuctionService) RecordPercenterTelemetryPublishRecovered() {
	if s != nil {
		s.percenterTelemetry.telemetryPublishRecovered.Add(1)
	}
}
