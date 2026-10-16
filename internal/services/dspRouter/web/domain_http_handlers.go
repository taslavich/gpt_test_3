package dspRouterWeb

import (
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/ggicci/httpin"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/filter"
	sppAdapterWeb "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/web"
)

func domainMapForTypic(typic string, adult, mainstream string) (string, bool) {
	switch typic {
	case sppAdapterWeb.ADULT:
		return adult, true
	case sppAdapterWeb.MAINSTREAM:
		return mainstream, true
	default:
		return "", false
	}
}

func domainChangersForTypic(typic string, adult, mainstream *filter.DomainChangersBoxChanger) (*filter.DomainChangersBoxChanger, bool) {
	switch typic {
	case sppAdapterWeb.ADULT:
		return adult, adult != nil
	case sppAdapterWeb.MAINSTREAM:
		return mainstream, mainstream != nil
	default:
		return nil, false
	}
}

func getDspDomainsMap(w http.ResponseWriter, r *http.Request, adult, mainstream string) {
	input := r.Context().Value(httpin.Input).(*getDspChangersMapRequest_V2_5)
	filename, ok := domainMapForTypic(input.Typic, adult, mainstream)
	if !ok {
		http.Error(w, "Invalid Typic value", http.StatusBadRequest)
		return
	}
	data, err := os.ReadFile(filename)
	if err != nil {
		http.Error(w, "Cannot ReadFile", http.StatusInternalServerError)
		return
	}
	var mapa filter.DomainChangerType
	if err := json.Unmarshal(data, &mapa); err != nil {
		http.Error(w, "Cannot Unmarshal", http.StatusInternalServerError)
		return
	}
	if err := rnr.JSON(w, http.StatusOK, mapa); err != nil {
		log.Printf("Cannot make HTTP response back: %v", err)
	}
}

func getDspDomainsMapDebug(w http.ResponseWriter, r *http.Request, adult, mainstream *filter.DomainChangersBoxChanger) {
	input := r.Context().Value(httpin.Input).(*getDspChangersMapRequest_V2_5)
	box, ok := domainChangersForTypic(input.Typic, adult, mainstream)
	if !ok {
		http.Error(w, "Invalid Typic value", http.StatusBadRequest)
		return
	}
	if err := rnr.JSON(w, http.StatusOK, box.Snapshot()); err != nil {
		log.Printf("Cannot make HTTP response back: %v", err)
	}
}

func putDspDomainsMap(w http.ResponseWriter, r *http.Request, adultFilename, mainstreamFilename string, adult, mainstream *filter.DomainChangersBoxChanger) {
	input := r.Context().Value(httpin.Input).(*putDspDomainsMapRequest)
	filename, valid := domainMapForTypic(input.Typic, adultFilename, mainstreamFilename)
	box, ok := domainChangersForTypic(input.Typic, adult, mainstream)
	if !valid || !ok {
		http.Error(w, "Invalid Typic value", http.StatusBadRequest)
		return
	}
	if err := box.Rewrite(input.DomainChangerType, filename); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNoContent)
}
