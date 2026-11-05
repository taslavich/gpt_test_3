package pub

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/http"
	"strings"
	"syscall"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/pubdiagnostic"
)

type rtbFailure struct {
	reason, explanation string
	path                string
	status              int
}

func (f rtbFailure) Error() string { return f.explanation }

func (f rtbFailure) withUpstream(upstream string) rtbFailure {
	f.path = alertRTBPath(upstream)
	return f
}

func classifyRTBHTTPFailure(status int, header, body string) rtbFailure {
	reason := header
	explanation, known := pubdiagnostic.SSPDescription(reason)
	if !known {
		// Rolling deploy: only recognize fixed strings from the old SSP. An
		// upstream body can contain private data or HTML and never goes to Telegram.
		body = strings.TrimSpace(body)
		switch {
		case status == 403 && legacyIPRestriction(body):
			reason = "ssp_ip_limited"
		case status == 403 && strings.HasPrefix(body, "User-Agent is too short: "):
			reason = "ssp_ua_short"
		case status == 403 && body == "Busy":
			reason = "ssp_feed_unknown"
		case status == 403 && body == "invalid publisher UUID":
			reason = "ssp_uuid_rejected"
		case status == 503 && body == "Publisher owner unavailable":
			reason = "ssp_publisher_owner"
		default:
			reason = ""
		}
		explanation, known = pubdiagnostic.SSPDescription(reason)
	}
	if !known {
		reason = "upstream_unclassified"
		switch status {
		case http.StatusBadRequest:
			explanation = "RTB rejected the request as invalid; no specific reason supplied"
		case http.StatusUnauthorized, http.StatusForbidden:
			explanation = "RTB refused the request; no specific reason supplied"
		case http.StatusNotFound:
			explanation = "RTB endpoint or resource was not found"
		case http.StatusRequestTimeout:
			explanation = "RTB timed out while receiving the request"
		case http.StatusTooManyRequests:
			explanation = "RTB or its proxy throttled the request"
		case http.StatusBadGateway:
			explanation = "The gateway failed to obtain a valid response from RTB"
		case http.StatusServiceUnavailable:
			explanation = "RTB or its proxy reported temporary unavailability"
		case http.StatusGatewayTimeout:
			explanation = "The gateway timed out waiting for RTB"
		default:
			explanation = "Unexpected RTB response; no specific reason supplied"
		}
	}
	return rtbFailure{reason: reason, explanation: explanation, status: status}
}

func legacyIPRestriction(body string) bool {
	for _, prefix := range []string{"Ip is limited: ", "Ipv6 is limited: "} {
		if ip, ok := strings.CutPrefix(body, prefix); ok && net.ParseIP(ip) != nil {
			return true
		}
	}
	return false
}

func classifyRTBTransportFailure(err error) rtbFailure {
	var netErr net.Error
	var dnsErr *net.DNSError
	var certificateErr *tls.CertificateVerificationError
	var unknownAuthority x509.UnknownAuthorityError
	var tlsRecord tls.RecordHeaderError
	reason, explanation := "transport_failed", "Publisher could not complete the connection to RTB; see the pub log"
	switch {
	case errors.Is(err, context.Canceled):
		reason, explanation = "transport_canceled", "The Publisher request to RTB was canceled"
	case errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()):
		reason, explanation = "transport_timeout", "Publisher timed out waiting for RTB"
	case errors.As(err, &dnsErr):
		reason, explanation = "transport_dns", "Publisher could not resolve the RTB hostname"
	case errors.Is(err, syscall.ECONNREFUSED):
		reason, explanation = "transport_refused", "The RTB connection was refused"
	case errors.Is(err, syscall.ECONNRESET):
		reason, explanation = "transport_reset", "The RTB connection was reset before the response completed"
	case errors.As(err, &certificateErr), errors.As(err, &unknownAuthority), errors.As(err, &tlsRecord):
		reason, explanation = "transport_tls", "Publisher could not establish TLS with RTB"
	}
	return rtbFailure{reason: reason, explanation: explanation}
}
