package dspRouterWeb

import pb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/dspRouter"

// Ordinary SSP requests cannot activate a Publisher percentage by setting an
// owner ID in unrelated OpenRTB publisher/user fields.
func trustedPublisherOwner(req *pb.DspRouterRequest_V2_5) string {
	if !req.GetIsPublisher() {
		return ""
	}
	return req.GetPublisherUserId()
}
