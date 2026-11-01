package web

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	auction "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/adv/service"
)

const PublisherPercentMapURL = "/filter/publisher_percent_map"
const DebugPublisherPercentMapURL = "/filter/debug_publisher_percent_map"

func initPublisherPercentRoutes(router *chi.Mux, store *auction.PublisherPercentStore) {
	router.Get(PublisherPercentMapURL, func(w http.ResponseWriter, _ *http.Request) {
		if store == nil {
			http.Error(w, "Publisher percent map unavailable", http.StatusServiceUnavailable)
			return
		}
		writeJSON(w, http.StatusOK, store.Saved())
	})
	router.Get(DebugPublisherPercentMapURL, func(w http.ResponseWriter, _ *http.Request) {
		if store == nil {
			http.Error(w, "Publisher percent map unavailable", http.StatusServiceUnavailable)
			return
		}
		writeJSON(w, http.StatusOK, store.Memory())
	})
	router.Put(PublisherPercentMapURL, func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			http.Error(w, "Publisher percent map unavailable", http.StatusServiceUnavailable)
			return
		}
		defer r.Body.Close()
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<20))
		var input auction.PublisherPercentMap
		if err := decoder.Decode(&input); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := ensureJSONEOF(decoder); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := store.Update(input); err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, auction.ErrPublisherPercentValidation) {
				status = http.StatusBadRequest
			}
			http.Error(w, err.Error(), status)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
