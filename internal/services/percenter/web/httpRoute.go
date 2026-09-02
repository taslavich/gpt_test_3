package web

import "github.com/go-chi/chi/v5"

const (
	DiagnosticsHealthURL  = "/internal/percenter/health"
	SegmentStateURL       = "/segments/{segmentHash}"
	SegmentMetricsURL     = "/segments/{segmentHash}/metrics"
	SegmentExplainURL     = "/segments/{segmentHash}/explain"
	SegmentEvaluateURL    = "/segments/{segmentHash}/evaluate"
	SegmentRebenchmarkURL = "/segments/{segmentHash}/rebenchmark"
	CampaignSegmentsURL   = "/campaigns/{campaignID}/segments"
	RedisKeyURL           = "/redis/key"
	RedisScanURL          = "/redis/scan"
)

func InitHttpRoutes(httpRouter *chi.Mux, server *Server) {
	httpRouter.Get(DiagnosticsHealthURL, server.handleHealth)
	httpRouter.Get(SegmentStateURL, server.handleSegmentState)
	httpRouter.Get(SegmentMetricsURL, server.handleSegmentMetrics)
	httpRouter.Get(SegmentExplainURL, server.handleSegmentExplain)
	httpRouter.Post(SegmentEvaluateURL, server.handleSegmentEvaluate)
	httpRouter.Post(SegmentRebenchmarkURL, server.handleSegmentRebenchmark)
	httpRouter.Get(CampaignSegmentsURL, server.handleCampaignSegments)
	httpRouter.Get(RedisKeyURL, server.handleRedisKey)
	httpRouter.Get(RedisScanURL, server.handleRedisScan)
}
