package dspRouterWeb

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/ggicci/httpin"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/filter"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
	sppAdapterWeb "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/web"
)

func requestedGeoFormat(value string) string {
	if strings.TrimSpace(value) == "" {
		return constants.POP
	}
	return normalizeDSPFormat(value)
}

func getSspGeoLinksMapDebug(
	w http.ResponseWriter,
	r *http.Request,
	routes *FormatRoutesV25,
) {
	if routes == nil {
		http.Error(w, "Format routes are not configured", http.StatusInternalServerError)
		return
	}
	input := r.Context().Value(httpin.Input).(*getSspGeoDspLinksRequest_V2_5)
	_, mapa := routes.selectConfig(requestedGeoFormat(input.Format), input.Typic)
	if mapa == nil {
		http.Error(w, "Invalid format/typic value", http.StatusBadRequest)
		return
	}
	if err := rnr.JSON(w, http.StatusOK, *mapa); err != nil {
		log.Printf("Cannot make HTTP response back: %v\n", err)
	}
}

func getSspGeoLinksMap(
	w http.ResponseWriter,
	r *http.Request,
	routes *FormatRoutesV25,
) {
	if routes == nil {
		http.Error(w, "Format routes are not configured", http.StatusInternalServerError)
		return
	}
	input := r.Context().Value(httpin.Input).(*getSspGeoDspLinksRequest_V2_5)
	filename, _ := routes.selectConfig(requestedGeoFormat(input.Format), input.Typic)
	if strings.TrimSpace(filename) == "" {
		http.Error(w, "Invalid format/typic value", http.StatusBadRequest)
		return
	}

	data, err := os.ReadFile(filename)
	if err != nil {
		http.Error(w, "Cannot ReadFile", http.StatusInternalServerError)
		return
	}

	var mapa GeoDspLinkMap
	if err := json.Unmarshal(data, &mapa); err != nil {
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
	routes *FormatRoutesV25,
) {
	if routes == nil {
		http.Error(w, "Format routes are not configured", http.StatusInternalServerError)
		return
	}
	input := r.Context().Value(httpin.Input).(*putSspGeoDspLinksRequest_V2_5)
	filename, mapa := routes.selectConfig(requestedGeoFormat(input.Format), input.Typic)
	if strings.TrimSpace(filename) == "" || mapa == nil {
		http.Error(w, "Invalid format/typic value", http.StatusBadRequest)
		return
	}

	updated, err := utils.RewriteSspGeoDspFileNextVer[bool](
		map[string]map[string]map[string]bool(input.Mapa),
		filename,
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	*mapa = GeoDspLinkMap(updated)

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
