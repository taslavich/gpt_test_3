package pub

import (
	"embed"
	"net/http"
)

//go:embed adManager.js sdk.prod.js runtime.prod.js
var runtimeFiles embed.FS

func (s *Server) script(w http.ResponseWriter, r *http.Request) {
	b, e := runtimeFiles.ReadFile("adManager.js")
	if e != nil {
		http.Error(w, "runtime unavailable", 503)
		return
	}
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "public,max-age=60")
	w.Write(b)
}
