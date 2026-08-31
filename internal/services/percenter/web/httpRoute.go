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
	httpRouter.Group(func(r chi.Router) {
		r.Use(server.requireAdmin)
		r.Get(SegmentStateURL, server.handleSegmentState)
		r.Get(SegmentMetricsURL, server.handleSegmentMetrics)
		r.Get(SegmentExplainURL, server.handleSegmentExplain)
		r.Post(SegmentEvaluateURL, server.handleSegmentEvaluate)
		r.Post(SegmentRebenchmarkURL, server.handleSegmentRebenchmark)
		r.Get(CampaignSegmentsURL, server.handleCampaignSegments)
		r.Get(RedisKeyURL, server.handleRedisKey)
		r.Get(RedisScanURL, server.handleRedisScan)
	})
}
