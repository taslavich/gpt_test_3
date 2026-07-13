package web

import (
	"log"
	"net/http"

	"github.com/ggicci/httpin"
	"github.com/ggicci/httpin/integration"
	"github.com/go-chi/chi/v5"
	"github.com/unrolled/render"
	auction "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/adv/service"
	sppAdapterWeb "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/web"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/types"
)

var rnr = render.New(render.Options{StreamingJSON: true, UnEscapeHTML: true})

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

func InitHttpRoutes(httpRouter *chi.Mux, percentFilenameAdult, percentFilenameMainstream string, store *auction.PercentStore) {
	integration.UseGochiURLParam("path", chi.URLParam)
	httpRouter.With(httpin.NewInput(getSspGeoDspPercentsRequestV25{})).Get(GetSspGeoDspPercentsMapUrl, func(w http.ResponseWriter, r *http.Request) { getSspGeoPercentsMapDebug(w, r, store) })
	httpRouter.With(httpin.NewInput(getSspGeoDspPercentsRequestV25{})).Get(GetDebugSspGeoDspPercentsMapUrl, func(w http.ResponseWriter, r *http.Request) { getSspGeoPercentsMapDebug(w, r, store) })
	httpRouter.With(httpin.NewInput(putSspGeoDspPercentsRequestV25{})).Put(PutSspGeoDspPercentsMapUrl, func(w http.ResponseWriter, r *http.Request) { putSspGeoPercentsMap(w, r, store) })
}

func putSspGeoPercentsMap(w http.ResponseWriter, r *http.Request, store *auction.PercentStore) {
	input := r.Context().Value(httpin.Input).(*putSspGeoDspPercentsRequestV25)
	if input.Typic != sppAdapterWeb.ADULT && input.Typic != sppAdapterWeb.MAINSTREAM {
		http.Error(w, "Invalid Typic value", http.StatusBadRequest)
		return
	}
	if err := store.Update(input.Typic, auction.PercentMap(input.Mapa)); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNoContent)
}

func getSspGeoPercentsMapDebug(w http.ResponseWriter, r *http.Request, store *auction.PercentStore) {
	input := r.Context().Value(httpin.Input).(*getSspGeoDspPercentsRequestV25)
	if input.Typic != sppAdapterWeb.ADULT && input.Typic != sppAdapterWeb.MAINSTREAM {
		http.Error(w, "Invalid Typic value", http.StatusBadRequest)
		return
	}
	if err := rnr.JSON(w, http.StatusOK, store.Get(input.Typic)); err != nil {
		log.Printf("Cannot make HTTP response back: %v\n", err)
	}
}

func InitWorkStatusRoutes(httpRouter *chi.Mux, controller *WorkController) {
	httpRouter.Get("/work_status", func(w http.ResponseWriter, r *http.Request) {
		_ = rnr.JSON(w, http.StatusOK, map[string]bool{"work": controller.Enabled()})
	})
	httpRouter.Put("/work_status", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("work") {
		case "true":
			_ = controller.Set(true)
			w.WriteHeader(http.StatusOK)
		case "false":
			_ = controller.Set(false)
			w.WriteHeader(http.StatusOK)
		default:
			http.Error(w, "invalid work parameter", http.StatusBadRequest)
		}
	})
}
