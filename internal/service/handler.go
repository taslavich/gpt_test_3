package service

import (
	"encoding/json"
	"log"
	"net/http"
)

type Handler struct {
	reportService *ReportService
}

func NewHandler(reportService *ReportService) *Handler {
	return &Handler{reportService: reportService}
}

func (h *Handler) WmAPI(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	feed := q.Get("feed")
	groupBy := q.Get("group_by")
	if groupBy == "" {
		groupBy = q.Get("group")
	}
	dateStart := q.Get("date_start")
	if dateStart == "" {
		dateStart = q.Get("start_date")
	}
	dateEnd := q.Get("date_end")
	if dateEnd == "" {
		dateEnd = q.Get("end_date")
	}

	response, statusCode, err := h.reportService.BuildReport(r.Context(), feed, groupBy, dateStart, dateEnd)
	if err != nil {
		log.Printf("wm_api error: %v", err)
		writeJSON(w, statusCode, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, response)
}

func writeJSON(w http.ResponseWriter, statusCode int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("write json failed: %v", err)
	}
}
