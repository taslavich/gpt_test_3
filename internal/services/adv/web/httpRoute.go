package web

import (
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/ggicci/httpin"
	"github.com/ggicci/httpin/integration"
	"github.com/go-chi/chi/v5"
	"github.com/unrolled/render"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
	sppAdapterWeb "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/web"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/types"
)

var rnr = render.New(render.Options{
	StreamingJSON: true,
	UnEscapeHTML:  true,
})

const (
	GetSspGeoDspPercentsMapUrl      = "/filter/ssp_geo_dsp_percents_map"
	PutSspGeoDspPercentsMapUrl      = "/filter/ssp_geo_dsp_percents_map"
	GetDebugSspGeoDspPercentsMapUrl = "/filter/debug_ssp_geo_dsp_percents_map"
)

type getSspGeoDspPercentsRequestV25 struct {
	Typic string `in:"query=typic" required:"true"`
}

type putSspGeoDspPercentsRequestV25 struct {
	Typic string                                                     `in:"query=typic" required:"true"`
	Mapa  map[string]map[string]map[string]*types.PercentAndBidfloor `in:"body=json"`
}

func InitHttpRoutes(
	httpRouter *chi.Mux,
	percentFilenameAdult string,
	percentFilenameMainstream string,
	percentMapAdult *map[string]map[string]map[string]*types.PercentAndBidfloor,
	percentMapMainstream *map[string]map[string]map[string]*types.PercentAndBidfloor,
) {
	integration.UseGochiURLParam("path", chi.URLParam)

	httpRouter.With(
		httpin.NewInput(getSspGeoDspPercentsRequestV25{}),
	).Get(GetSspGeoDspPercentsMapUrl, func(w http.ResponseWriter, r *http.Request) {
		getSspGeoPercentsMap(w, r, percentFilenameAdult, percentFilenameMainstream)
	})

	httpRouter.With(
		httpin.NewInput(getSspGeoDspPercentsRequestV25{}),
	).Get(GetDebugSspGeoDspPercentsMapUrl, func(w http.ResponseWriter, r *http.Request) {
		getSspGeoPercentsMapDebug(w, r, percentMapAdult, percentMapMainstream)
	})

	httpRouter.With(
		httpin.NewInput(putSspGeoDspPercentsRequestV25{}),
	).Put(PutSspGeoDspPercentsMapUrl, func(w http.ResponseWriter, r *http.Request) {
		putSspGeoPercentsMap(w, r, percentFilenameAdult, percentFilenameMainstream, percentMapAdult, percentMapMainstream)
	})
}

func putSspGeoPercentsMap(
	w http.ResponseWriter,
	r *http.Request,
	percentFilenameAdult string,
	percentFilenameMainstream string,
	percentMapAdult *map[string]map[string]map[string]*types.PercentAndBidfloor,
	percentMapMainstream *map[string]map[string]map[string]*types.PercentAndBidfloor,
) {
	var err error
	input := r.Context().Value(httpin.Input).(*putSspGeoDspPercentsRequestV25)
	switch input.Typic {
	case sppAdapterWeb.ADULT:
		*percentMapAdult, err = utils.RewriteSspGeoDspFileNextVer[*types.PercentAndBidfloor](input.Mapa, percentFilenameAdult)
	case sppAdapterWeb.MAINSTREAM:
		*percentMapMainstream, err = utils.RewriteSspGeoDspFileNextVer[*types.PercentAndBidfloor](input.Mapa, percentFilenameMainstream)
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

func getSspGeoPercentsMap(w http.ResponseWriter, r *http.Request, percentFilenameAdult string, percentFilenameMainstream string) {
	var filename string
	input := r.Context().Value(httpin.Input).(*getSspGeoDspPercentsRequestV25)
	switch input.Typic {
	case sppAdapterWeb.ADULT:
		filename = percentFilenameAdult
	case sppAdapterWeb.MAINSTREAM:
		filename = percentFilenameMainstream
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
	percentMapAdult *map[string]map[string]map[string]*types.PercentAndBidfloor,
	percentMapMainstream *map[string]map[string]map[string]*types.PercentAndBidfloor,
) {
	var mapa map[string]map[string]map[string]*types.PercentAndBidfloor
	input := r.Context().Value(httpin.Input).(*getSspGeoDspPercentsRequestV25)
	switch input.Typic {
	case sppAdapterWeb.ADULT:
		mapa = *percentMapAdult
	case sppAdapterWeb.MAINSTREAM:
		mapa = *percentMapMainstream
	default:
		http.Error(w, "Invalid Typic value", http.StatusBadRequest)
		return
	}
	if err := rnr.JSON(w, http.StatusOK, mapa); err != nil {
		log.Printf("Cannot make HTTP response back: %v\n", err)
	}
}
