package bidEngineWeb

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/ggicci/httpin"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/types"
)

func requestedPercentFormat(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" {
		// Existing control-plane callers did not send format; preserve POP.
		return constants.POP
	}
	return value
}

func putSspGeoPercentsMap(
	w http.ResponseWriter,
	r *http.Request,
	percentRoutes *types.FormatPercentRoutesV25,
) {
	input := r.Context().Value(httpin.Input).(*putSspGeoDspPercentsRequest_V2_5)
	if percentRoutes == nil {
		http.Error(w, "Percent routes are not configured", http.StatusInternalServerError)
		return
	}

	filename, selected := percentRoutes.Select(requestedPercentFormat(input.Format), input.Typic)
	if filename == "" || selected == nil {
		http.Error(w, "Invalid format/typic value", http.StatusBadRequest)
		return
	}

	updated, err := utils.RewriteSspGeoDspFileNextVer[*types.PercentAndBidfloor](input.Mapa, filename)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	*selected = types.GeoDspPercentMap(updated)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNoContent)
}

func getSspGeoPercentsMap(
	w http.ResponseWriter,
	r *http.Request,
	percentRoutes *types.FormatPercentRoutesV25,
) {
	input := r.Context().Value(httpin.Input).(*getSspGeoDspPercentsRequest_V2_5)
	if percentRoutes == nil {
		http.Error(w, "Percent routes are not configured", http.StatusInternalServerError)
		return
	}

	filename, selected := percentRoutes.Select(requestedPercentFormat(input.Format), input.Typic)
	if filename == "" || selected == nil {
		http.Error(w, "Invalid format/typic value", http.StatusBadRequest)
		return
	}

	data, err := os.ReadFile(filename)
	if err != nil {
		http.Error(w, "Cannot ReadFile", http.StatusInternalServerError)
		return
	}

	var mapa types.GeoDspPercentMap
	if err := json.Unmarshal(data, &mapa); err != nil {
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
	percentRoutes *types.FormatPercentRoutesV25,
) {
	input := r.Context().Value(httpin.Input).(*getSspGeoDspPercentsRequest_V2_5)
	if percentRoutes == nil {
		http.Error(w, "Percent routes are not configured", http.StatusInternalServerError)
		return
	}
	_, selected := percentRoutes.Select(requestedPercentFormat(input.Format), input.Typic)
	if selected == nil {
		http.Error(w, "Invalid format/typic value", http.StatusBadRequest)
		return
	}

	if err := rnr.JSON(w, http.StatusOK, *selected); err != nil {
		log.Printf("Cannot make HTTP response back: %v\n", err)
	}
}
