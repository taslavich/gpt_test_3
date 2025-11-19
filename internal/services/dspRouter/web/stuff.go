package dspRouterWeb

import (
	"context"
	"log"
	"os"

	dspRouterGrpc "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/dspRouter"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
	sppAdapterWeb "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/web"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (s *Server) GetSspGeoLinksMap(ctx context.Context, req *dspRouterGrpc.GetSspGeoDspLinksRequest_V2_5) (*dspRouterGrpc.GetSspGeoDspLinksResponse_V2_5, error) {
	var data []byte
	var err error

	switch req.Typic {
	case sppAdapterWeb.ADULT:
		data, err = os.ReadFile(s.linkFilename_adult)
	case sppAdapterWeb.MAINSTREAM:
		data, err = os.ReadFile(s.linkFilename_mainstream)
	default:
		return nil, status.Error(codes.Internal, "Typic is no here! failed to read file")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read file %s: %w", s.linkFilename_adult, err)
	}

	return &dspRouterGrpc.GetSspGeoDspLinksResponse_V2_5{
		JsonData: string(data),
	}, nil
}

func (s *Server) SetSspGeoLinksMap(ctx context.Context, req *dspRouterGrpc.SspGeoDspLinksRequest_V2_5) (*emptypb.Empty, error) {
	var err error

	switch req.Typic {
	case sppAdapterWeb.ADULT:
		s.linkMap_adult, err = utils.RewriteSspGeoDspFile[bool](
			req.JsonData,
			s.linkFilename_adult,
		)
	case sppAdapterWeb.MAINSTREAM:
		s.linkMap_mainstream, err = utils.RewriteSspGeoDspFile[bool](
			req.JsonData,
			s.linkFilename_mainstream,
		)
	default:
		return nil, status.Error(codes.Internal, "Typic is no here! failed to read file")
	}
	if err != nil {
		return nil, err
	}

	log.Printf("Successfully updated Links SSP entries")

	return nil, nil
}

/*func (s *Server) SetDspFiltersMap(context.Context, *dspRouterGrpc.SetDspFiltersRequest_V2_5) (*emptypb.Empty, error) {

}

func (s *Server) GetDspFiltersMap(context.Context, *emptypb.Empty) (*dspRouterGrpc.GetDspFiltersResponse_V2_5, error) {

}*/
