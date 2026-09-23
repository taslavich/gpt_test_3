package bidEngineWeb

import (
	"net/http"

	"github.com/ggicci/httpin"
	"github.com/ggicci/httpin/integration"
	"github.com/go-chi/chi/v5"
	"github.com/unrolled/render"
	bidEngine "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/bidEngine/service"
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

	GetSiteIDDspPercentsMapUrl      = "/filter/site_id_dsp_percents_map"
	PutSiteIDDspPercentsMapUrl      = "/filter/site_id_dsp_percents_map"
	GetDebugSiteIDDspPercentsMapUrl = "/filter/debug_site_id_dsp_percents_map"
)

type getSspGeoDspPercentsRequest_V2_5 struct {
	Typic  string `in:"query=typic" required:"true"`
	Format string `in:"query=format"`
}

type putSspGeoDspPercentsRequest_V2_5 struct {
	Typic  string                 `in:"query=typic" required:"true"`
	Format string                 `in:"query=format"`
	Mapa   types.GeoDspPercentMap `in:"body=json"`
}

type putSiteIDDspPercentsMapRequest struct {
	Mapa bidEngine.Map `in:"body=json"`
}

func InitHttpRoutes(
	httpRouter *chi.Mux,
	percentRoutes *types.FormatPercentRoutesV25,
	sitePercentStores ...*bidEngine.Store,
) {
	integration.UseGochiURLParam("path", chi.URLParam)

	var sitePercentStore *bidEngine.Store
	if len(sitePercentStores) > 0 {
		sitePercentStore = sitePercentStores[0]
	}

	httpRouter.With(
		httpin.NewInput(getSspGeoDspPercentsRequest_V2_5{}),
	).Get(GetSspGeoDspPercentsMapUrl, func(w http.ResponseWriter, r *http.Request) {
		getSspGeoPercentsMap(w, r, percentRoutes)
	})

	httpRouter.With(
		httpin.NewInput(getSspGeoDspPercentsRequest_V2_5{}),
	).Get(GetDebugSspGeoDspPercentsMapUrl, func(w http.ResponseWriter, r *http.Request) {
		getSspGeoPercentsMapDebug(w, r, percentRoutes)
	})

	httpRouter.With(
		httpin.NewInput(putSspGeoDspPercentsRequest_V2_5{}),
	).Put(PutSspGeoDspPercentsMapUrl, func(w http.ResponseWriter, r *http.Request) {
		putSspGeoPercentsMap(w, r, percentRoutes)
	})

	if sitePercentStore != nil {
		httpRouter.Get(GetSiteIDDspPercentsMapUrl, func(w http.ResponseWriter, r *http.Request) {
			getSiteIDDspPercentsMap(w, sitePercentStore)
		})
		httpRouter.Get(GetDebugSiteIDDspPercentsMapUrl, func(w http.ResponseWriter, r *http.Request) {
			getSiteIDDspPercentsMapDebug(w, sitePercentStore)
		})
		httpRouter.With(
			httpin.NewInput(putSiteIDDspPercentsMapRequest{}),
		).Put(PutSiteIDDspPercentsMapUrl, func(w http.ResponseWriter, r *http.Request) {
			putSiteIDDspPercentsMap(w, r, sitePercentStore)
		})
	}
}
