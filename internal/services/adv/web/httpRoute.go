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

	httpRouter.Get(GetQualityMapURL, func(w http.ResponseWriter, _ *http.Request) {
		if qualityStore == nil {
			http.Error(w, "quality store is unavailable", http.StatusServiceUnavailable)
			return
		}
		value, err := qualityStore.Saved()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, value)
	})
	httpRouter.Get(GetDebugQualityMapURL, func(w http.ResponseWriter, _ *http.Request) {
		if qualityStore == nil {
			http.Error(w, "quality store is unavailable", http.StatusServiceUnavailable)
			return
		}
		writeJSON(w, http.StatusOK, qualityStore.Memory())
	})
	httpRouter.Put(PutQualityMapURL, func(w http.ResponseWriter, r *http.Request) {
		if qualityStore == nil {
			http.Error(w, "quality store is unavailable", http.StatusServiceUnavailable)
			return
		}
		defer r.Body.Close()
		data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 16<<20))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := qualityStore.UpdateJSON(data); err != nil {
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

func writeJSON(w http.ResponseWriter, statusCode int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("write ADV HTTP response: %v", err)
	}
}
