package web

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	auction "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/adv/service"
)

const (
	GetADVPercentMapURL      = "/filter/adv_percent_map"
	PutADVPercentMapURL      = "/filter/adv_percent_map"
	GetDebugADVPercentMapURL = "/filter/debug_adv_percent_map"

	// Legacy routes remain available so existing control clients do not break.
	GetSspGeoDspPercentsMapURL      = "/filter/ssp_geo_dsp_percents_map"
	PutSspGeoDspPercentsMapURL      = "/filter/ssp_geo_dsp_percents_map"
	GetDebugSspGeoDspPercentsMapURL = "/filter/debug_ssp_geo_dsp_percents_map"

	GetQualityMapURL       = "/filter/quality_map"
	PutQualityMapURL       = "/filter/quality_map"
	GetDebugQualityMapURL  = "/filter/debug_quality_map"
	WorkStatusURL          = "/work_status"
	AntiPerekrutRestartURL = "/internal/antiperekrut/restart"
)

// Backward-compatible aliases for callers that used the previous Go constant spelling.
const (
	GetSspGeoDspPercentsMapUrl      = GetSspGeoDspPercentsMapURL
	PutSspGeoDspPercentsMapUrl      = PutSspGeoDspPercentsMapURL
	GetDebugSspGeoDspPercentsMapUrl = GetDebugSspGeoDspPercentsMapURL
)

type AntiPerekrutHTTPConfig struct {
	Manager *auction.AntiPerekrutManager
}

type antiPerekrutRestartRequest struct {
	EventID        string `json:"event_id"`
	SourceService  string `json:"source_service"`
	SourceInstance string `json:"source_instance"`
	Reason         string `json:"reason"`
}

func InitHttpRoutes(httpRouter *chi.Mux, percentStore *auction.PercentStore, qualityStore *auction.QualityStore, work *WorkController, antiConfig ...AntiPerekrutHTTPConfig) {
	getPercentMap := func(w http.ResponseWriter, _ *http.Request) {
		if percentStore == nil {
			http.Error(w, "percent store is unavailable", http.StatusServiceUnavailable)
			return
		}
		value, err := percentStore.Saved()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, value)
	}
	getDebugPercentMap := func(w http.ResponseWriter, _ *http.Request) {
		if percentStore == nil {
			http.Error(w, "percent store is unavailable", http.StatusServiceUnavailable)
			return
		}
		writeJSON(w, http.StatusOK, percentStore.Memory())
	}
	putPercentMap := func(w http.ResponseWriter, r *http.Request) {
		if percentStore == nil {
			http.Error(w, "percent store is unavailable", http.StatusServiceUnavailable)
			return
		}
		defer r.Body.Close()
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<20))
		decoder.DisallowUnknownFields()
		var input auction.PercentMap
		if err := decoder.Decode(&input); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := ensureJSONEOF(decoder); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := percentStore.Update(input); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}

	httpRouter.Get(GetADVPercentMapURL, getPercentMap)
	httpRouter.Get(GetDebugADVPercentMapURL, getDebugPercentMap)
	httpRouter.Put(PutADVPercentMapURL, putPercentMap)

	httpRouter.Get(GetSspGeoDspPercentsMapURL, getPercentMap)
	httpRouter.Get(GetDebugSspGeoDspPercentsMapURL, getDebugPercentMap)
	httpRouter.Put(PutSspGeoDspPercentsMapURL, putPercentMap)

	httpRouter.Get(GetQualityMapURL, func(w http.ResponseWriter, r *http.Request) {
		if qualityStore == nil {
			http.Error(w, "quality store is unavailable", http.StatusServiceUnavailable)
			return
		}
		segment, present, valid := queryQualitySegment(r)
		if present && !valid {
			http.Error(w, "quality must be usual, high or ultra", http.StatusBadRequest)
			return
		}
		if !present {
			value, err := qualityStore.SavedAll()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, http.StatusOK, value)
			return
		}
		value, err := qualityStore.Saved(segment)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, value)
	})
	httpRouter.Get(GetDebugQualityMapURL, func(w http.ResponseWriter, r *http.Request) {
		if qualityStore == nil {
			http.Error(w, "quality store is unavailable", http.StatusServiceUnavailable)
			return
		}
		segment, present, valid := queryQualitySegment(r)
		if present && !valid {
			http.Error(w, "quality must be usual, high or ultra", http.StatusBadRequest)
			return
		}
		if !present {
			writeJSON(w, http.StatusOK, qualityStore.MemoryAll())
			return
		}
		writeJSON(w, http.StatusOK, qualityStore.Memory(segment))
	})
	httpRouter.Put(PutQualityMapURL, func(w http.ResponseWriter, r *http.Request) {
		if qualityStore == nil {
			http.Error(w, "quality store is unavailable", http.StatusServiceUnavailable)
			return
		}
		segment, present, valid := queryQualitySegment(r)
		if present && !valid {
			http.Error(w, "quality must be usual, high or ultra", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<20))
		decoder.DisallowUnknownFields()
		if !present {
			var input auction.QualityMaps
			if err := decoder.Decode(&input); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if err := ensureJSONEOF(decoder); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if err := qualityStore.UpdateAll(input); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}

		var input auction.QualityDomainMap
		if err := decoder.Decode(&input); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := ensureJSONEOF(decoder); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := qualityStore.Update(segment, input); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	httpRouter.Get(WorkStatusURL, func(w http.ResponseWriter, _ *http.Request) {
		if work == nil {
			http.Error(w, "work controller is unavailable", http.StatusServiceUnavailable)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"work": work.Enabled()})
	})
	httpRouter.Put(WorkStatusURL, func(w http.ResponseWriter, r *http.Request) {
		raw := strings.TrimSpace(r.URL.Query().Get("work"))
		enabled, err := strconv.ParseBool(raw)
		if err != nil {
			http.Error(w, "work must be true or false", http.StatusBadRequest)
			return
		}
		if work == nil || work.Set(enabled) != nil {
			http.Error(w, "cannot change ADV work status", http.StatusServiceUnavailable)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"work": enabled})
	})

	if len(antiConfig) > 0 && antiConfig[0].Manager != nil {
		cfg := antiConfig[0]
		httpRouter.Post(AntiPerekrutRestartURL, func(w http.ResponseWriter, r *http.Request) {
			defer r.Body.Close()
			decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
			decoder.DisallowUnknownFields()
			var input antiPerekrutRestartRequest
			if err := decoder.Decode(&input); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if err := ensureJSONEOF(decoder); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if strings.TrimSpace(input.Reason) != "startup" {
				http.Error(w, "reason must be startup", http.StatusBadRequest)
				return
			}
			switch strings.TrimSpace(input.SourceService) {
			case "adm-adapter", "spp-adapter", "orchestrator", "router", "bid-engine", "adv":
			default:
				http.Error(w, "unsupported source_service", http.StatusBadRequest)
				return
			}
			if strings.TrimSpace(input.SourceInstance) == "" {
				http.Error(w, "source_instance is required", http.StatusBadRequest)
				return
			}
			generation, err := cfg.Manager.RegisterStartupEvent(r.Context(), input.EventID, input.SourceService, input.SourceInstance)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, http.StatusOK, map[string]int64{"generation": generation})
		})
	}
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); errors.Is(err, io.EOF) {
		return nil
	} else if err != nil {
		return err
	}
	return errors.New("request contains more than one JSON value")
}

func queryQualitySegment(r *http.Request) (segment string, present bool, valid bool) {
	raw := strings.TrimSpace(r.URL.Query().Get("quality"))
	if raw == "" {
		return "", false, false
	}
	value := strings.ToLower(raw)
	return value, true, value == "usual" || value == "high" || value == "ultra"
}

func writeJSON(w http.ResponseWriter, statusCode int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("write ADV HTTP response: %v", err)
	}
}
