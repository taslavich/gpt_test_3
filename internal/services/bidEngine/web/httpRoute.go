package bidEngineWeb

import (
	"net/http"

	"github.com/ggicci/httpin"
	"github.com/ggicci/httpin/integration"
	"github.com/go-chi/chi/v5"
	"github.com/unrolled/render"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/types"
)

var rnr = render.New(render.Options{
	StreamingJSON: true,
	UnEscapeHTML:  true,
})

const (
	GetSspGeoDspPercentsMapUrl = "/filter/ssp_geo_dsp_percents_map"
	PutSspGeoDspPercentsMapUrl = "/filter/ssp_geo_dsp_percents_map"
)

const (
	GetDebugSspGeoDspPercentsMapUrl = "/filter/debug_ssp_geo_dsp_percents_map"
)

type getSspGeoDspPercentsRequest_V2_5 struct {
	Typic string `in:"query=typic" required:"true"`
}

type putSspGeoDspPercentsRequest_V2_5 struct {
	Typic string                                                     `in:"query=typic" required:"true"`
	Mapa  map[string]map[string]map[string]*types.PercentAndBidfloor `in:"body=json"`
}

func InitHttpRoutes(
	httpRouter *chi.Mux,
	percentFilename_adult,
	percentFilename_mainstream string,

	percentMap_adult,
	percentMap_mainstream *map[string]map[string]map[string]*types.PercentAndBidfloor,
) {
	integration.UseGochiURLParam("path", chi.URLParam)

	httpRouter.With(
		httpin.NewInput(getSspGeoDspPercentsRequest_V2_5{}),
	).Get(GetSspGeoDspPercentsMapUrl, func(w http.ResponseWriter, r *http.Request) {
		getSspGeoPercentsMap(w, r, percentFilename_adult, percentFilename_mainstream)
	})

	httpRouter.With(
		httpin.NewInput(getSspGeoDspPercentsRequest_V2_5{}),
	).Get(GetDebugSspGeoDspPercentsMapUrl, func(w http.ResponseWriter, r *http.Request) {
		getSspGeoPercentsMapDebug(w, r, percentMap_adult, percentMap_mainstream)
	})

	httpRouter.With(
		httpin.NewInput(putSspGeoDspPercentsRequest_V2_5{}),
	).Put(PutSspGeoDspPercentsMapUrl, func(w http.ResponseWriter, r *http.Request) {
		putSspGeoPercentsMap(w, r, percentFilename_adult, percentFilename_mainstream, percentMap_adult, percentMap_mainstream)
	})
}
