package pub

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	c "gitlab.com/twinbid-exchange/RTB-exchange/internal/pubcontract"
)

const directVisitorCookie = "tb_direct_visitor"

func (s *Server) directLink(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet { // Go's GET pattern also matches HEAD.
		w.Header().Set("Allow", "GET")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	mode := r.Header.Get("Sec-Fetch-Mode")
	if (mode != "" && mode != "navigate") ||
		strings.Contains(strings.ToLower(r.Header.Get("Sec-Purpose")), "prefetch") ||
		strings.Contains(strings.ToLower(r.Header.Get("Purpose")), "prefetch") {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	// The existing code contract has no configured site. An absent or invalid
	// Referer cannot be attributed to a site or checked against site blocks.
	host, page, err := c.Page(r.Referer())
	if err != nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	s.Requests.Add(1)
	in := Request{ID: uuid.NewString(), Code: r.PathValue("code"), PageURL: page}
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		s.negative(w, r, in, c.Code{}, "", "", "", "overload", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), s.Config.RTBTimeout+3*time.Second)
	defer cancel()
	epoch, err := s.ready(ctx)
	if err != nil {
		s.negative(w, r, in, c.Code{}, "", "", "", "config_unready", http.StatusServiceUnavailable)
		return
	}
	code, err := s.code(ctx, in.Code, epoch)
	if err != nil || code.Format != "direct_link" {
		http.NotFound(w, r)
		return
	}
	site := c.SiteID(host)
	zone := c.ZoneID(site, code.ID)
	if !code.Active || code.Archived {
		s.negative(w, r, in, code, host, site, zone, "code_paused", http.StatusForbidden)
		return
	}
	// A first-party session cookie supplies the visitor key that JS supplies
	// for Popunder. The direct format has no configurable per-session cap.
	if cookie, err := r.Cookie(directVisitorCookie); err == nil {
		if visitor, err := uuid.Parse(cookie.Value); err == nil && visitor != uuid.Nil {
			in.VisitorID = visitor.String()
		}
	}
	if in.VisitorID == "" {
		in.VisitorID = uuid.NewString()
		http.SetCookie(w, &http.Cookie{
			Name: directVisitorCookie, Value: in.VisitorID, Path: "/go/",
			Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode,
		})
	}
	in.SessionID = in.VisitorID
	s.serveAuction(ctx, w, r, in, code, host, site, zone, epoch, true)
}
