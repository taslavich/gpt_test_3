package dspRouterWeb

import (
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/ggicci/httpin"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/filter"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
	sppAdapterWeb "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/web"
)

func getSspGeoLinksMap(
	w http.ResponseWriter,
	r *http.Request,
	linkMap_adult string,
	linkMap_mainstream string,
) {
	var filename string
	input := r.Context().Value(httpin.Input).(*getSspGeoDspLinksRequest_V2_5)

	switch input.Typic {
	case sppAdapterWeb.ADULT:
		filename = linkMap_adult
	case sppAdapterWeb.MAINSTREAM:
		filename = linkMap_mainstream
	default:
		http.Error(w, "Invalid Typic value", http.StatusBadRequest)
		return
	}

	data, err := os.ReadFile(filename)
	if err != nil {
		http.Error(w, "Cannot ReadFile", http.StatusInternalServerError)
		return
	}

	var mapa map[string]map[string]map[string]bool
	err = json.Unmarshal(data, &mapa)
	if err != nil {
		http.Error(w, "Cannot Unmarshal", http.StatusInternalServerError)
		return
	}

	if err := rnr.JSON(w, http.StatusOK, mapa); err != nil {
		log.Printf("Cannot make HTTP response back: %v\n", err)
	}
}

func putSspGeoLinksMap(
	w http.ResponseWriter,
	r *http.Request,
	linkFilename_adult string,
	linkFilename_mainstream string,
	linkMap_adult *map[string]map[string]map[string]bool,
	linkMap_mainstream *map[string]map[string]map[string]bool) {
	var err error

	input := r.Context().Value(httpin.Input).(*putSspGeoDspLinksRequest_V2_5)

	switch input.Typic {
	case sppAdapterWeb.ADULT:
		*linkMap_adult, err = utils.RewriteSspGeoDspFileNextVer[bool](
			input.Mapa,
			linkFilename_adult,
		)
	case sppAdapterWeb.MAINSTREAM:
		*linkMap_mainstream, err = utils.RewriteSspGeoDspFileNextVer[bool](
			input.Mapa,
			linkFilename_mainstream,
		)
	default:
		http.Error(w, "Invalid Typic value", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNoContent)
}

func putDspFiltersMap(
	w http.ResponseWriter,
	r *http.Request,
	filters *filter.FiltersBox,
	filtersFilename string,
) {
	var err error
	input := r.Context().Value(httpin.Input).(*putDspFiltersMapRequest)

	filters.Allowers, err = filter.RewriteDspFiltersFileNextVer(input.FiltersJsonBox, filtersFilename)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNoContent)
}

func getDspFiltersMap(
	w http.ResponseWriter,
	r *http.Request,
	filters string,
) {
	data, err := os.ReadFile(filters)
	if err != nil {
		http.Error(w, "Cannot ReadFile", http.StatusInternalServerError)
		return
	}

	var mapa map[string]*filter.FiltersJson
	err = json.Unmarshal(data, &mapa)
	if err != nil {
		http.Error(w, "Cannot Unmarshal", http.StatusInternalServerError)
		return
	}

	if err := rnr.JSON(w, http.StatusOK, mapa); err != nil {
		log.Printf("Cannot make HTTP response back: %v\n", err)
	}
}
