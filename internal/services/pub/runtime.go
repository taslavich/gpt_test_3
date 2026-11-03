package pub

import (
	"bytes"
	"embed"
	"encoding/json"
	"net/http"
)

const pageHelperPath = "/static/pageHelper.js"

//go:embed pageHelper.js sdk.prod.js runtime.prod.js
var runtimeFiles embed.FS

func (s *Server) script(w http.ResponseWriter, r *http.Request) {
	b, e := runtimeFiles.ReadFile("pageHelper.js")
	if e != nil {
		http.Error(w, "runtime unavailable", 503)
		return
	}
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "public,max-age=60")
	// The direct script may be hosted on a separate domain; API and delivery
	// stay on the configured Publisher origin. The bundled SDK runtime is untouched.
	origin, _ := json.Marshal(s.Config.PublicURL)
	w.Write(bytes.Replace(b, []byte("/* publisher-api-origin */ tag.src"), origin, 1))
}
