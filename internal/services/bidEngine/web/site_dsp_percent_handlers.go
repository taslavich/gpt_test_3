package bidEngineWeb

import (
	"log"
	"net/http"

	"github.com/ggicci/httpin"
	bidEngine "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/bidEngine/service"
)

func getSiteIDDspPercentsMap(w http.ResponseWriter, store *bidEngine.Store) {
	raw, err := store.ReadRaw()
	if err != nil {
		http.Error(w, "Cannot read site_id/DSP percent map", http.StatusInternalServerError)
		return
	}
	if err := rnr.JSON(w, http.StatusOK, raw); err != nil {
		log.Printf("Cannot make HTTP response back: %v\n", err)
	}
}

func getSiteIDDspPercentsMapDebug(w http.ResponseWriter, store *bidEngine.Store) {
	if err := rnr.JSON(w, http.StatusOK, store.Snapshot()); err != nil {
		log.Printf("Cannot make HTTP response back: %v\n", err)
	}
}

func putSiteIDDspPercentsMap(w http.ResponseWriter, r *http.Request, store *bidEngine.Store) {
	input := r.Context().Value(httpin.Input).(*putSiteIDDspPercentsMapRequest)
	if err := store.Update(input.Mapa); err != nil {
		status := http.StatusInternalServerError
		if bidEngine.IsValidationError(err) {
			status = http.StatusBadRequest
		}
		http.Error(w, err.Error(), status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNoContent)
}
