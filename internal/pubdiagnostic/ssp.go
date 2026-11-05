// Package pubdiagnostic defines the internal Publisher/SSP diagnostic contract.
// Only fixed reason identifiers cross the HTTP boundary; never raw error text.
package pubdiagnostic

const Header = "X-TwinBid-SSP-Diagnostic"

var sspReasons = map[string]string{
	"ssp_panic":                    "SSP Adapter crashed while processing this request (panic recovered)",
	"ssp_request_invalid":          "SSP Adapter could not read the OpenRTB request",
	"ssp_dependencies":             "SSP Adapter dependencies are not initialized",
	"ssp_feed_unknown":             "SSP Adapter did not find this feed in the route configuration",
	"ssp_uuid_rejected":            "SSP Adapter could not validate or claim the signed Publisher request UUID",
	"ssp_device_missing":           "OpenRTB request is missing the device object",
	"ssp_ip_missing":               "OpenRTB request is missing both IPv4 and IPv6",
	"ssp_ip_limited":               "SSP Adapter rejected the visitor IP under the request-rate policy",
	"ssp_ua_missing":               "OpenRTB request is missing the User-Agent",
	"ssp_ua_short":                 "SSP Adapter rejected a User-Agent shorter than 20 characters",
	"ssp_site_missing":             "OpenRTB request is missing the site object",
	"ssp_site_id_missing":          "OpenRTB request is missing the site ID",
	"ssp_ip_check_failed":          "SSP Adapter failed to check the IP reputation",
	"ssp_ip_rejected":              "SSP Adapter rejected the IP during the IP reputation check",
	"ssp_ip_invalid":               "Geo lookup rejected the format of the visitor IP",
	"ssp_geo_lookup_failed":        "SSP Adapter Geo lookup failed internally",
	"ssp_impressions_missing":      "OpenRTB request has no impressions",
	"ssp_impression_id_missing":    "OpenRTB impression is missing its ID",
	"ssp_impression_id_duplicate":  "OpenRTB request contains duplicate impression IDs",
	"ssp_video_missing":            "Video impression is missing its video object",
	"ssp_video_mimes_missing":      "Video impression is missing the supported MIME types",
	"ssp_publisher_owner":          "SSP Adapter could not read the Publisher owner from Redis",
	"ssp_orchestrator_timeout":     "SSP Adapter timed out waiting for Orchestrator",
	"ssp_orchestrator_unavailable": "SSP Adapter could not reach Orchestrator",
	"ssp_orchestrator_canceled":    "The call from SSP Adapter to Orchestrator was canceled",
	"ssp_orchestrator_rejected":    "Orchestrator rejected the bid request",
	"ssp_orchestrator_error":       "The call from SSP Adapter to Orchestrator failed",
	"ssp_orchestrator_empty":       "Orchestrator returned no response object",
	"ssp_site_blocked":             "Orchestrator blocked this site ID",
	"ssp_ortb_write_failed":        "SSP Adapter could not write or enqueue OpenRTB statistics in Redis",
}

func SSPDescription(reason string) (string, bool) {
	text, ok := sspReasons[reason]
	return text, ok
}
