package dspRouterWeb

import (
	"net/http"

	"github.com/ggicci/httpin"
	"github.com/ggicci/httpin/integration"
	"github.com/go-chi/chi/v5"
	"github.com/unrolled/render"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/filter"
)

var rnr = render.New(render.Options{
	StreamingJSON: true,
	UnEscapeHTML:  true,
})

const (
	GetSspGeoDspLinksMapUrl = "/filter/ssp_geo_dsp_links_map"
	PutSspGeoDspLinksMapUrl = "/filter/ssp_geo_dsp_links_map"

	GetDspFiltersMapUrl = "/filter/dsp_filters_map"
	PutDspFiltersMapUrl = "/filter/dsp_filters_map"
)

type getSspGeoDspLinksRequest_V2_5 struct {
	Typic string `in:"query=typic" required:"true"`
}

type putDspFiltersMapRequest struct {
	filter.FiltersJsonBox `in:"body=json"`
}

type putSspGeoDspLinksRequest_V2_5 struct {
	Typic string                                `in:"query=typic" required:"true"`
	Mapa  map[string]map[string]map[string]bool `in:"body=json"`
}

func InitHttpRoutes(
	httpRouter *chi.Mux,
	linkFilename_adult string,
	linkFilename_mainstream string,

	linkMap_adult,
	linkMap_mainstream *map[string]map[string]map[string]bool,

	filters *filter.FiltersBox,
	filtersFilename string,
) {
	integration.UseGochiURLParam("path", chi.URLParam)

	httpRouter.With(
		httpin.NewInput(getSspGeoDspLinksRequest_V2_5{}),
	).Get(GetSspGeoDspLinksMapUrl, func(w http.ResponseWriter, r *http.Request) {
		getSspGeoLinksMap(w, r, linkFilename_adult, linkFilename_mainstream)
	})

	httpRouter.With(
		httpin.NewInput(putSspGeoDspLinksRequest_V2_5{}),
	).Put(PutSspGeoDspLinksMapUrl, func(w http.ResponseWriter, r *http.Request) {
		putSspGeoLinksMap(w, r, linkFilename_adult, linkFilename_mainstream, linkMap_adult, linkMap_mainstream)
	})

	httpRouter.Get(GetDspFiltersMapUrl, func(w http.ResponseWriter, r *http.Request) {
		getDspFiltersMap(w, r, filtersFilename)
	})

	httpRouter.With(
		httpin.NewInput(putDspFiltersMapRequest{}),
	).Put(PutDspFiltersMapUrl, func(w http.ResponseWriter, r *http.Request) {
		putDspFiltersMap(w, r, filters, filtersFilename)
	})
}
