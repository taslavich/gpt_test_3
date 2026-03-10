package dspRouterWeb

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/ggicci/httpin"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/filter"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
	sppAdapterWeb "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/web"
)

func getSspGeoLinksMapDebug(
	w http.ResponseWriter,
	r *http.Request,
	linkMap_adult *map[string]map[string]map[string]bool,
	linkMap_mainstream *map[string]map[string]map[string]bool,
) {
	var mapa map[string]map[string]map[string]bool
	input := r.Context().Value(httpin.Input).(*getSspGeoDspLinksRequest_V2_5)

	switch input.Typic {
	case sppAdapterWeb.ADULT:
		mapa = *linkMap_adult
	case sppAdapterWeb.MAINSTREAM:
		mapa = *linkMap_mainstream
	default:
		http.Error(w, "Invalid Typic value", http.StatusBadRequest)
		return
	}

	if err := rnr.JSON(w, http.StatusOK, mapa); err != nil {
		log.Printf("Cannot make HTTP response back: %v\n", err)
	}
}

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
	filtersAdlFilename string,
	filtersMcFilename string,
	filtersAdl *filter.FiltersBox,
	filtersMc *filter.FiltersBox,
) {
	var err error

	input := r.Context().Value(httpin.Input).(*putDspFiltersMapRequest)

	switch input.Typic {
	case sppAdapterWeb.ADULT:
		filtersAdl.Allowers, err = filter.RewriteDspFiltersFileNextVer(input.FiltersJsonBox, filtersAdlFilename)
	case sppAdapterWeb.MAINSTREAM:
		filtersMc.Allowers, err = filter.RewriteDspFiltersFileNextVer(input.FiltersJsonBox, filtersMcFilename)
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

func getDspFiltersMap(
	w http.ResponseWriter,
	r *http.Request,
	filtersAdl string,
	filtersMc string,
) {
	var filename string

	input := r.Context().Value(httpin.Input).(*getDspFiltersMapRequest_V2_5)

	switch input.Typic {
	case sppAdapterWeb.ADULT:
		filename = filtersAdl
	case sppAdapterWeb.MAINSTREAM:
		filename = filtersMc
	default:
		http.Error(w, "Invalid Typic value", http.StatusBadRequest)
		return
	}

	data, err := os.ReadFile(filename)
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

func getDspFiltersMapDebug(
	w http.ResponseWriter,
	r *http.Request,
	filtersAdl *filter.FiltersBox,
	filtersMc *filter.FiltersBox,
) {
	var filters *filter.FiltersBox

	input := r.Context().Value(httpin.Input).(*getDspFiltersMapRequest_V2_5)

	switch input.Typic {
	case sppAdapterWeb.ADULT:
		filters = filtersAdl
	case sppAdapterWeb.MAINSTREAM:
		filters = filtersMc
	default:
		http.Error(w, "Invalid Typic value", http.StatusBadRequest)
		return
	}

	if err := rnr.JSON(w, http.StatusOK, filters.Allowers); err != nil {
		log.Printf("Cannot make HTTP response back: %v\n", err)
	}
}

func getDspChangersMap(
	w http.ResponseWriter,
	r *http.Request,
	changersAdl string,
	changersMc string,
) {
	var filename string

	input := r.Context().Value(httpin.Input).(*getDspChangersMapRequest_V2_5)

	switch input.Typic {
	case sppAdapterWeb.ADULT:
		filename = changersAdl
	case sppAdapterWeb.MAINSTREAM:
		filename = changersMc
	default:
		http.Error(w, "Invalid Typic value", http.StatusBadRequest)
		return
	}

	data, err := os.ReadFile(filename)
	if err != nil {
		http.Error(w, "Cannot ReadFile", http.StatusInternalServerError)
		return
	}

	var mapa filter.ChangerType
	err = json.Unmarshal(data, &mapa)
	if err != nil {
		http.Error(w, "Cannot Unmarshal", http.StatusInternalServerError)
		return
	}

	if err := rnr.JSON(w, http.StatusOK, mapa); err != nil {
		log.Printf("Cannot make HTTP response back: %v\n", err)
	}
}

func putDspChangersMap(
	w http.ResponseWriter,
	r *http.Request,
	changersAdlFilename string,
	changersMcFilename string,
	changersAdl *filter.ChangersBoxChanger,
	changersMc *filter.ChangersBoxChanger,
) {
	var err error

	input := r.Context().Value(httpin.Input).(*putDspChangersMapRequest)

	switch input.Typic {
	case sppAdapterWeb.ADULT:
		changersAdl.Changers, err = filter.RewriteChangersFile(input.ChangerType, changersAdlFilename)
	case sppAdapterWeb.MAINSTREAM:
		fmt.Println("In Put")
		fmt.Println(changersMcFilename)
		changersMc.Changers, err = filter.RewriteChangersFile(input.ChangerType, changersMcFilename)
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

func getDspChangersMapDebug(
	w http.ResponseWriter,
	r *http.Request,
	changersAdl *filter.ChangersBoxChanger,
	changersMc *filter.ChangersBoxChanger,
) {
	var changers *filter.ChangersBoxChanger

	input := r.Context().Value(httpin.Input).(*getDspChangersMapRequest_V2_5)

	switch input.Typic {
	case sppAdapterWeb.ADULT:
		changers = changersAdl
	case sppAdapterWeb.MAINSTREAM:
		changers = changersMc
	default:
		http.Error(w, "Invalid Typic value", http.StatusBadRequest)
		return
	}

	if err := rnr.JSON(w, http.StatusOK, changers.Changers); err != nil {
		log.Printf("Cannot make HTTP response back: %v\n", err)
	}
}

//-------------------------------------------------------------------------

func getDspFiltersCidMap(
	w http.ResponseWriter,
	r *http.Request,
	filtersCidAdl string,
	filtersCidMc string,
) {
	var filename string

	input := r.Context().Value(httpin.Input).(*getDspFiltersCidMapRequest_V2_5)

	switch input.Typic {
	case sppAdapterWeb.ADULT:
		filename = filtersCidAdl
	case sppAdapterWeb.MAINSTREAM:
		filename = filtersCidMc
	default:
		http.Error(w, "Invalid Typic value", http.StatusBadRequest)
		return
	}

	data, err := os.ReadFile(filename)
	if err != nil {
		http.Error(w, "Cannot ReadFile", http.StatusInternalServerError)
		return
	}

	var mapa filter.FilterCidJsonBoxType
	err = json.Unmarshal(data, &mapa)
	if err != nil {
		http.Error(w, "Cannot Unmarshal", http.StatusInternalServerError)
		return
	}

	if err := rnr.JSON(w, http.StatusOK, mapa); err != nil {
		log.Printf("Cannot make HTTP response back: %v\n", err)
	}
}

func getDspFiltersCidMapDebug(
	w http.ResponseWriter,
	r *http.Request,
	filtersCidAdl *filter.FilterCidBoxType,
	filtersCidMc *filter.FilterCidBoxType,
) {
	var filters *filter.FilterCidBoxType

	input := r.Context().Value(httpin.Input).(*getDspFiltersCidMapRequest_V2_5)

	switch input.Typic {
	case sppAdapterWeb.ADULT:
		filters = filtersCidAdl
	case sppAdapterWeb.MAINSTREAM:
		filters = filtersCidMc
	default:
		http.Error(w, "Invalid Typic value", http.StatusBadRequest)
		return
	}

	if err := rnr.JSON(w, http.StatusOK, filters); err != nil {
		log.Printf("Cannot make HTTP response back: %v\n", err)
	}
}

func putDspFiltersCidMap(
	w http.ResponseWriter,
	r *http.Request,
	filtersCidAdlFilename string,
	filtersCidMcFilename string,
	filtersCidAdl *filter.FilterCidBoxType,
	filtersCidMc *filter.FilterCidBoxType,
) {
	var err error

	input := r.Context().Value(httpin.Input).(*putDspFiltersCidMapRequest)

	switch input.Typic {
	case sppAdapterWeb.ADULT:
		*filtersCidAdl, err = filter.RewriteDspFiltersCidFileNextVer(input.FilterCidJsonBoxType, filtersCidAdlFilename)
	case sppAdapterWeb.MAINSTREAM:
		*filtersCidMc, err = filter.RewriteDspFiltersCidFileNextVer(input.FilterCidJsonBoxType, filtersCidMcFilename)
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
