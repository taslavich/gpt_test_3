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
	GetSspGeoDspPercentsMapURL      = "/filter/ssp_geo_dsp_percents_map"
	PutSspGeoDspPercentsMapURL      = "/filter/ssp_geo_dsp_percents_map"
	GetDebugSspGeoDspPercentsMapURL = "/filter/debug_ssp_geo_dsp_percents_map"
	GetQualityMapURL                = "/filter/quality_map"
	PutQualityMapURL                = "/filter/quality_map"
	GetDebugQualityMapURL           = "/filter/debug_quality_map"
	WorkStatusURL                   = "/work_status"
)

// Backward-compatible aliases for callers that used the previous spelling.
const (
	GetSspGeoDspPercentsMapUrl      = GetSspGeoDspPercentsMapURL
	PutSspGeoDspPercentsMapUrl      = PutSspGeoDspPercentsMapURL
	GetDebugSspGeoDspPercentsMapUrl = GetDebugSspGeoDspPercentsMapURL
)

func InitHttpRoutes(httpRouter *chi.Mux, percentStore *auction.PercentStore, qualityStore *auction.QualityStore, work *WorkController) {
	httpRouter.Get(GetSspGeoDspPercentsMapURL, func(w http.ResponseWriter, r *http.Request) {
		traffic, ok := queryTrafficType(r)
		if !ok || percentStore == nil {
			http.Error(w, "invalid traffic type or percent store", http.StatusBadRequest)
			return
		}
		value, err := percentStore.Saved(traffic)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, value)
	})

	httpRouter.Get(GetDebugSspGeoDspPercentsMapURL, func(w http.ResponseWriter, r *http.Request) {
		traffic, ok := queryTrafficType(r)
		if !ok || percentStore == nil {
			http.Error(w, "invalid traffic type or percent store", http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, percentStore.Memory(traffic))
	})

	httpRouter.Put(PutSspGeoDspPercentsMapURL, func(w http.ResponseWriter, r *http.Request) {
		traffic, ok := queryTrafficType(r)
		if !ok || percentStore == nil {
			http.Error(w, "invalid traffic type or percent store", http.StatusBadRequest)
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
		if err := percentStore.Update(traffic, input); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

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

		var input auction.QualityFeedMap
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

func queryTrafficType(r *http.Request) (string, bool) {
	value := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("typic")))
	return value, value == auction.TrafficAdult || value == auction.TrafficMainstream
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
