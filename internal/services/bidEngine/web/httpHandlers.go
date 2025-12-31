package bidEngineWeb

import (
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/ggicci/httpin"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
	sppAdapterWeb "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/web"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/types"
)

func putSspGeoPercentsMap(
	w http.ResponseWriter,
	r *http.Request,
	percentFilename_adult string,
	percentFilename_mainstream string,
	percentMap_adult *map[string]map[string]map[string]*types.PercentAndBidfloor,
	percentMap_mainstream *map[string]map[string]map[string]*types.PercentAndBidfloor,
) {
	var err error

	input := r.Context().Value(httpin.Input).(*putSspGeoDspPercentsRequest_V2_5)
	switch input.Typic {
	case sppAdapterWeb.ADULT:
		*percentMap_adult, err = utils.RewriteSspGeoDspFileNextVer[*types.PercentAndBidfloor](
			input.Mapa,
			percentFilename_adult,
		)
	case sppAdapterWeb.MAINSTREAM:
		*percentMap_mainstream, err = utils.RewriteSspGeoDspFileNextVer[*types.PercentAndBidfloor](
			input.Mapa,
			percentFilename_mainstream,
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

func getSspGeoPercentsMap(
	w http.ResponseWriter,
	r *http.Request,
	percentMap_adult string,
	percentMap_mainstream string,
) {
	var filename string

	input := r.Context().Value(httpin.Input).(*getSspGeoDspPercentsRequest_V2_5)

	switch input.Typic {
	case sppAdapterWeb.ADULT:
		filename = percentMap_adult
	case sppAdapterWeb.MAINSTREAM:
		filename = percentMap_mainstream
	default:
		http.Error(w, "Invalid Typic value", http.StatusBadRequest)
		return
	}

	data, err := os.ReadFile(filename)
	if err != nil {
		http.Error(w, "Cannot ReadFile", http.StatusInternalServerError)
		return
	}

	var mapa map[string]map[string]map[string]*types.PercentAndBidfloor
	err = json.Unmarshal(data, &mapa)
	if err != nil {
		http.Error(w, "Cannot Unmarshal", http.StatusInternalServerError)
		return
	}

	if err := rnr.JSON(w, http.StatusOK, mapa); err != nil {
		log.Printf("Cannot make HTTP response back: %v\n", err)
	}
}

func getSspGeoPercentsMapDebug(
	w http.ResponseWriter,
	r *http.Request,
	percentMap_adult *map[string]map[string]map[string]*types.PercentAndBidfloor,
	percentMap_mainstream *map[string]map[string]map[string]*types.PercentAndBidfloor,
) {
	var mapa map[string]map[string]map[string]*types.PercentAndBidfloor
	input := r.Context().Value(httpin.Input).(*getSspGeoDspPercentsRequest_V2_5)

	switch input.Typic {
	case sppAdapterWeb.ADULT:
		mapa = *percentMap_adult
	case sppAdapterWeb.MAINSTREAM:
		mapa = *percentMap_mainstream
	default:
		http.Error(w, "Invalid Typic value", http.StatusBadRequest)
		return
	}

	if err := rnr.JSON(w, http.StatusOK, mapa); err != nil {
		log.Printf("Cannot make HTTP response back: %v\n", err)
	}
}
