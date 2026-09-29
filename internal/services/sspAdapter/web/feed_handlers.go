package sppAdapterWeb

import (
	"log"
	"net/http"

	"github.com/ggicci/httpin"
)

func getSspFeedsMap(w http.ResponseWriter, r *http.Request, routes *FormatFeedRoutesV25, debug bool) {
	if routes == nil {
		http.Error(w, "SSP feed routes are not configured", http.StatusInternalServerError)
		return
	}
	input := r.Context().Value(httpin.Input).(*getSspFeedsMapRequest)
	store := routes.Select(requestedFeedFormat(input.Format), input.Typic)
	if store == nil {
		http.Error(w, "Invalid format/typic value", http.StatusBadRequest)
		return
	}

	var (
		mapa FeedMap
		err  error
	)
	if debug {
		mapa = store.Snapshot()
	} else {
		mapa, err = store.ReadRaw()
		if err != nil {
			http.Error(w, "Cannot ReadFile", http.StatusInternalServerError)
			return
		}
	}

	if err := rnr.JSON(w, http.StatusOK, mapa); err != nil {
		log.Printf("Cannot make HTTP response back: %v\n", err)
	}
}

func putSspFeedsMap(w http.ResponseWriter, r *http.Request, routes *FormatFeedRoutesV25) {
	if routes == nil {
		http.Error(w, "SSP feed routes are not configured", http.StatusInternalServerError)
		return
	}
	input := r.Context().Value(httpin.Input).(*putSspFeedsMapRequest)
	store := routes.Select(requestedFeedFormat(input.Format), input.Typic)
	if store == nil {
		http.Error(w, "Invalid format/typic value", http.StatusBadRequest)
		return
	}
	if err := store.Update(input.Mapa); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNoContent)
}
