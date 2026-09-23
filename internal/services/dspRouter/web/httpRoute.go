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

	GetDspChangersMapUrl = "/filter/dsp_changers_map"
	PutDspChangersMapUrl = "/filter/dsp_changers_map"

	GetDspFiltersCidMapUrl = "/filter/dsp_filters_cid_map"
	PutDspFiltersCidMapUrl = "/filter/dsp_filters_cid_map"
)

const (
	GetDebugSspGeoDspLinksMapUrl = "/filter/debug_ssp_geo_dsp_links_map"
	GetDebugDspFiltersMapUrl     = "/filter/debug_dsp_filters_map"
	GetDebugDspChangersMapUrl    = "/filter/debug_dsp_changers_map"
	GetDebugDspFiltersCidMapUrl  = "/filter/debug_dsp_filters_cid_map"
)

type getSspGeoDspLinksRequest_V2_5 struct {
	Typic  string `in:"query=typic" required:"true"`
	Format string `in:"query=format"`
}

type getDspFiltersMapRequest_V2_5 struct {
	Typic string `in:"query=typic" required:"true"`
}

type putDspFiltersMapRequest struct {
	Typic                 string `in:"query=typic" required:"true"`
	filter.FiltersJsonBox `in:"body=json"`
}

type putSspGeoDspLinksRequest_V2_5 struct {
	Typic  string        `in:"query=typic" required:"true"`
	Format string        `in:"query=format"`
	Mapa   GeoDspLinkMap `in:"body=json"`
}

type getDspChangersMapRequest_V2_5 struct {
	Typic string `in:"query=typic" required:"true"`
}

type putDspChangersMapRequest struct {
	Typic              string `in:"query=typic" required:"true"`
	filter.ChangerType `in:"body=json"`
}

type getDspFiltersCidMapRequest_V2_5 struct {
	Typic string `in:"query=typic" required:"true"`
}

type putDspFiltersCidMapRequest struct {
	Typic                       string `in:"query=typic" required:"true"`
	filter.FilterCidJsonBoxType `in:"body=json"`
}

func InitHttpRoutes(
	httpRouter *chi.Mux,
	formatRoutes *FormatRoutesV25,

	filtersAdlFilename string,
	filtersMcFilename string,

	filtersAdl *filter.FiltersBox,
	filtersMc *filter.FiltersBox,

	changersAdlFilename string,
	changersMcFilename string,

	changersAdl *filter.ChangersBoxChanger,
	changersMc *filter.ChangersBoxChanger,

	filtersCidAdlFilename string,
	filtersCidMcFilename string,

	filtersCidAdl *filter.FilterCidBoxType,
	filtersCidMc *filter.FilterCidBoxType,
) {
	integration.UseGochiURLParam("path", chi.URLParam)

	httpRouter.With(
		httpin.NewInput(getSspGeoDspLinksRequest_V2_5{}),
	).Get(GetSspGeoDspLinksMapUrl, func(w http.ResponseWriter, r *http.Request) {
		getSspGeoLinksMap(w, r, formatRoutes)
	})

	httpRouter.With(
		httpin.NewInput(getSspGeoDspLinksRequest_V2_5{}),
	).Get(GetDebugSspGeoDspLinksMapUrl, func(w http.ResponseWriter, r *http.Request) {
		getSspGeoLinksMapDebug(w, r, formatRoutes)
	})

	httpRouter.With(
		httpin.NewInput(putSspGeoDspLinksRequest_V2_5{}),
	).Put(PutSspGeoDspLinksMapUrl, func(w http.ResponseWriter, r *http.Request) {
		putSspGeoLinksMap(w, r, formatRoutes)
	})

	httpRouter.With(
		httpin.NewInput(getDspFiltersMapRequest_V2_5{}),
	).Get(GetDspFiltersMapUrl, func(w http.ResponseWriter, r *http.Request) {
		getDspFiltersMap(w, r, filtersAdlFilename, filtersMcFilename)
	})

	httpRouter.With(
		httpin.NewInput(getDspFiltersMapRequest_V2_5{}),
	).Get(GetDebugDspFiltersMapUrl, func(w http.ResponseWriter, r *http.Request) {
		getDspFiltersMapDebug(w, r, filtersAdl, filtersMc)
	})

	httpRouter.With(
		httpin.NewInput(putDspFiltersMapRequest{}),
	).Put(PutDspFiltersMapUrl, func(w http.ResponseWriter, r *http.Request) {
		putDspFiltersMap(w, r, filtersAdlFilename, filtersMcFilename, filtersAdl, filtersMc)
	})

	//----------------------------

	httpRouter.With(
		httpin.NewInput(getDspChangersMapRequest_V2_5{}),
	).Get(GetDspChangersMapUrl, func(w http.ResponseWriter, r *http.Request) {
		getDspChangersMap(w, r, changersAdlFilename, changersMcFilename)
	})

	httpRouter.With(
		httpin.NewInput(getDspChangersMapRequest_V2_5{}),
	).Get(GetDebugDspChangersMapUrl, func(w http.ResponseWriter, r *http.Request) {
		getDspChangersMapDebug(w, r, changersAdl, changersMc)
	})

	httpRouter.With(
		httpin.NewInput(putDspChangersMapRequest{}),
	).Put(PutDspChangersMapUrl, func(w http.ResponseWriter, r *http.Request) {
		putDspChangersMap(w, r, changersAdlFilename, changersMcFilename, changersAdl, changersMc)
	})

	//-----------------------------------

	httpRouter.With(
		httpin.NewInput(getDspFiltersCidMapRequest_V2_5{}),
	).Get(GetDspFiltersCidMapUrl, func(w http.ResponseWriter, r *http.Request) {
		getDspFiltersCidMap(w, r, filtersCidAdlFilename, filtersCidMcFilename)
	})

	httpRouter.With(
		httpin.NewInput(getDspFiltersCidMapRequest_V2_5{}),
	).Get(GetDebugDspFiltersCidMapUrl, func(w http.ResponseWriter, r *http.Request) {
		getDspFiltersCidMapDebug(w, r, filtersCidAdl, filtersCidMc)
	})

	httpRouter.With(
		httpin.NewInput(putDspFiltersCidMapRequest{}),
	).Put(PutDspFiltersCidMapUrl, func(w http.ResponseWriter, r *http.Request) {
		putDspFiltersCidMap(w, r, filtersCidAdlFilename, filtersCidMcFilename, filtersCidAdl, filtersCidMc)
	})
}
