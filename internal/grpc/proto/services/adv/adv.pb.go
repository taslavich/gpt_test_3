// Code generated for this repository. DO NOT EDIT.
// source: services/adv.proto

package advGrpc

import (
	ortb_V2_5 "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
	unsafe "unsafe"
)

const (
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type DoAuctionRequest struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	BidRequest    *ortb_V2_5.BidRequest  `protobuf:"bytes,1,opt,name=bidRequest,proto3" json:"bidRequest,omitempty"`
	Format        string                 `protobuf:"bytes,2,opt,name=format,proto3" json:"format,omitempty"`
	TrafficType   string                 `protobuf:"bytes,3,opt,name=traffic_type,json=trafficType,proto3" json:"traffic_type,omitempty"`
	SspDomain     string                 `protobuf:"bytes,4,opt,name=ssp_domain,json=sspDomain,proto3" json:"ssp_domain,omitempty"`
	ImpIdUuid     map[string]string      `protobuf:"bytes,5,rep,name=impIdUuid,proto3" json:"impIdUuid,omitempty" protobuf_key:"bytes,1,opt,name=key" protobuf_val:"bytes,2,opt,name=value"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *DoAuctionRequest) Reset() {
	*x = DoAuctionRequest{}
	mi := &file_services_adv_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *DoAuctionRequest) String() string { return protoimpl.X.MessageStringOf(x) }

func (*DoAuctionRequest) ProtoMessage() {}

func (x *DoAuctionRequest) ProtoReflect() protoreflect.Message {
	mi := &file_services_adv_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

func (*DoAuctionRequest) Descriptor() ([]byte, []int) {
	return file_services_adv_proto_rawDescGZIP(), []int{0}
}

func (x *DoAuctionRequest) GetBidRequest() *ortb_V2_5.BidRequest {
	if x != nil {
		return x.BidRequest
	}
	return nil
}

func (x *DoAuctionRequest) GetFormat() string {
	if x != nil {
		return x.Format
	}
	return ""
}

func (x *DoAuctionRequest) GetTrafficType() string {
	if x != nil {
		return x.TrafficType
	}
	return ""
}

func (x *DoAuctionRequest) GetSspDomain() string {
	if x != nil {
		return x.SspDomain
	}
	return ""
}

func (x *DoAuctionRequest) GetImpIdUuid() map[string]string {
	if x != nil {
		return x.ImpIdUuid
	}
	return nil
}

type DoAuctionResponse struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Selected      bool                   `protobuf:"varint,1,opt,name=selected,proto3" json:"selected,omitempty"`
	CampaignId    string                 `protobuf:"bytes,2,opt,name=campaign_id,json=campaignId,proto3" json:"campaign_id,omitempty"`
	CreativeId    string                 `protobuf:"bytes,3,opt,name=creative_id,json=creativeId,proto3" json:"creative_id,omitempty"`
	Adm           string                 `protobuf:"bytes,4,opt,name=adm,proto3" json:"adm,omitempty"`
	AuctionPrice  float64                `protobuf:"fixed64,5,opt,name=auction_price,json=auctionPrice,proto3" json:"auction_price,omitempty"`
	Code          int32                  `protobuf:"varint,6,opt,name=code,proto3" json:"code,omitempty"`
	BidResponse   *ortb_V2_5.BidResponse `protobuf:"bytes,7,opt,name=bid_response,json=bidResponse,proto3" json:"bid_response,omitempty"`
	WinnerUserIds map[string]string      `protobuf:"bytes,8,rep,name=winnerUserIds,proto3" json:"winnerUserIds,omitempty" protobuf_key:"bytes,1,opt,name=key" protobuf_val:"bytes,2,opt,name=value"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *DoAuctionResponse) Reset() {
	*x = DoAuctionResponse{}
	mi := &file_services_adv_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *DoAuctionResponse) String() string { return protoimpl.X.MessageStringOf(x) }

func (*DoAuctionResponse) ProtoMessage() {}

func (x *DoAuctionResponse) ProtoReflect() protoreflect.Message {
	mi := &file_services_adv_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

func (*DoAuctionResponse) Descriptor() ([]byte, []int) {
	return file_services_adv_proto_rawDescGZIP(), []int{1}
}

func (x *DoAuctionResponse) GetSelected() bool {
	if x != nil {
		return x.Selected
	}
	return false
}

func (x *DoAuctionResponse) GetCampaignId() string {
	if x != nil {
		return x.CampaignId
	}
	return ""
}

func (x *DoAuctionResponse) GetCreativeId() string {
	if x != nil {
		return x.CreativeId
	}
	return ""
}

func (x *DoAuctionResponse) GetAdm() string {
	if x != nil {
		return x.Adm
	}
	return ""
}

func (x *DoAuctionResponse) GetAuctionPrice() float64 {
	if x != nil {
		return x.AuctionPrice
	}
	return 0
}

func (x *DoAuctionResponse) GetCode() int32 {
	if x != nil {
		return x.Code
	}
	return 0
}

func (x *DoAuctionResponse) GetBidResponse() *ortb_V2_5.BidResponse {
	if x != nil {
		return x.BidResponse
	}
	return nil
}

func (x *DoAuctionResponse) GetWinnerUserIds() map[string]string {
	if x != nil {
		return x.WinnerUserIds
	}
	return nil
}

var File_services_adv_proto protoreflect.FileDescriptor

const file_services_adv_proto_rawDesc = "\x0a\x12\x73\x65\x72\x76\x69\x63\x65\x73\x2f\x61\x64\x76\x2e\x70\x72\x6f\x74\x6f\x12\x03\x61\x64" +
	"\x76\x1a\x1a\x74\x79\x70\x65\x73\x2f\x6f\x72\x74\x62\x5f\x56\x32\x5f\x35\x2f\x6f\x72\x74\x62\x2e" +
	"\x70\x72\x6f\x74\x6f\x22\xa5\x02\x0a\x10\x44\x6f\x41\x75\x63\x74\x69\x6f\x6e\x52\x65\x71\x75\x65" +
	"\x73\x74\x12\x35\x0a\x0a\x62\x69\x64\x52\x65\x71\x75\x65\x73\x74\x18\x01\x20\x01\x28\x0b\x32\x15" +
	"\x2e\x6f\x72\x74\x62\x5f\x56\x32\x5f\x35\x2e\x42\x69\x64\x52\x65\x71\x75\x65\x73\x74\x52\x0a\x62" +
	"\x69\x64\x52\x65\x71\x75\x65\x73\x74\x12\x16\x0a\x06\x66\x6f\x72\x6d\x61\x74\x18\x02\x20\x01\x28" +
	"\x09\x52\x06\x66\x6f\x72\x6d\x61\x74\x12\x21\x0a\x0c\x74\x72\x61\x66\x66\x69\x63\x5f\x74\x79\x70" +
	"\x65\x18\x03\x20\x01\x28\x09\x52\x0b\x74\x72\x61\x66\x66\x69\x63\x54\x79\x70\x65\x12\x1d\x0a\x0a" +
	"\x73\x73\x70\x5f\x64\x6f\x6d\x61\x69\x6e\x18\x04\x20\x01\x28\x09\x52\x09\x73\x73\x70\x44\x6f\x6d" +
	"\x61\x69\x6e\x12\x42\x0a\x09\x69\x6d\x70\x49\x64\x55\x75\x69\x64\x18\x05\x20\x03\x28\x0b\x32\x24" +
	"\x2e\x61\x64\x76\x2e\x44\x6f\x41\x75\x63\x74\x69\x6f\x6e\x52\x65\x71\x75\x65\x73\x74\x2e\x49\x6d" +
	"\x70\x49\x64\x55\x75\x69\x64\x45\x6e\x74\x72\x79\x52\x09\x69\x6d\x70\x49\x64\x55\x75\x69\x64\x1a" +
	"\x3c\x0a\x0e\x49\x6d\x70\x49\x64\x55\x75\x69\x64\x45\x6e\x74\x72\x79\x12\x10\x0a\x03\x6b\x65\x79" +
	"\x18\x01\x20\x01\x28\x09\x52\x03\x6b\x65\x79\x12\x14\x0a\x05\x76\x61\x6c\x75\x65\x18\x02\x20\x01" +
	"\x28\x09\x52\x05\x76\x61\x6c\x75\x65\x3a\x02\x38\x01\x22\x8a\x03\x0a\x11\x44\x6f\x41\x75\x63\x74" +
	"\x69\x6f\x6e\x52\x65\x73\x70\x6f\x6e\x73\x65\x12\x1a\x0a\x08\x73\x65\x6c\x65\x63\x74\x65\x64\x18" +
	"\x01\x20\x01\x28\x08\x52\x08\x73\x65\x6c\x65\x63\x74\x65\x64\x12\x1f\x0a\x0b\x63\x61\x6d\x70\x61" +
	"\x69\x67\x6e\x5f\x69\x64\x18\x02\x20\x01\x28\x09\x52\x0a\x63\x61\x6d\x70\x61\x69\x67\x6e\x49\x64" +
	"\x12\x1f\x0a\x0b\x63\x72\x65\x61\x74\x69\x76\x65\x5f\x69\x64\x18\x03\x20\x01\x28\x09\x52\x0a\x63" +
	"\x72\x65\x61\x74\x69\x76\x65\x49\x64\x12\x10\x0a\x03\x61\x64\x6d\x18\x04\x20\x01\x28\x09\x52\x03" +
	"\x61\x64\x6d\x12\x23\x0a\x0d\x61\x75\x63\x74\x69\x6f\x6e\x5f\x70\x72\x69\x63\x65\x18\x05\x20\x01" +
	"\x28\x01\x52\x0c\x61\x75\x63\x74\x69\x6f\x6e\x50\x72\x69\x63\x65\x12\x12\x0a\x04\x63\x6f\x64\x65" +
	"\x18\x06\x20\x01\x28\x05\x52\x04\x63\x6f\x64\x65\x12\x39\x0a\x0c\x62\x69\x64\x5f\x72\x65\x73\x70" +
	"\x6f\x6e\x73\x65\x18\x07\x20\x01\x28\x0b\x32\x16\x2e\x6f\x72\x74\x62\x5f\x56\x32\x5f\x35\x2e\x42" +
	"\x69\x64\x52\x65\x73\x70\x6f\x6e\x73\x65\x52\x0b\x62\x69\x64\x52\x65\x73\x70\x6f\x6e\x73\x65\x12" +
	"\x4f\x0a\x0d\x77\x69\x6e\x6e\x65\x72\x55\x73\x65\x72\x49\x64\x73\x18\x08\x20\x03\x28\x0b\x32\x29" +
	"\x2e\x61\x64\x76\x2e\x44\x6f\x41\x75\x63\x74\x69\x6f\x6e\x52\x65\x73\x70\x6f\x6e\x73\x65\x2e\x57" +
	"\x69\x6e\x6e\x65\x72\x55\x73\x65\x72\x49\x64\x73\x45\x6e\x74\x72\x79\x52\x0d\x77\x69\x6e\x6e\x65" +
	"\x72\x55\x73\x65\x72\x49\x64\x73\x1a\x40\x0a\x12\x57\x69\x6e\x6e\x65\x72\x55\x73\x65\x72\x49\x64" +
	"\x73\x45\x6e\x74\x72\x79\x12\x10\x0a\x03\x6b\x65\x79\x18\x01\x20\x01\x28\x09\x52\x03\x6b\x65\x79" +
	"\x12\x14\x0a\x05\x76\x61\x6c\x75\x65\x18\x02\x20\x01\x28\x09\x52\x05\x76\x61\x6c\x75\x65\x3a\x02" +
	"\x38\x01\x32\x4a\x0a\x0a\x41\x64\x76\x53\x65\x72\x76\x69\x63\x65\x12\x3c\x0a\x09\x44\x6f\x41\x75" +
	"\x63\x74\x69\x6f\x6e\x12\x15\x2e\x61\x64\x76\x2e\x44\x6f\x41\x75\x63\x74\x69\x6f\x6e\x52\x65\x71" +
	"\x75\x65\x73\x74\x1a\x16\x2e\x61\x64\x76\x2e\x44\x6f\x41\x75\x63\x74\x69\x6f\x6e\x52\x65\x73\x70" +
	"\x6f\x6e\x73\x65\x22\x00\x42\x53\x5a\x51\x67\x69\x74\x6c\x61\x62\x2e\x63\x6f\x6d\x2f\x74\x77\x69" +
	"\x6e\x62\x69\x64\x2d\x65\x78\x63\x68\x61\x6e\x67\x65\x2f\x52\x54\x42\x2d\x65\x78\x63\x68\x61\x6e" +
	"\x67\x65\x2f\x69\x6e\x74\x65\x72\x6e\x61\x6c\x2f\x67\x72\x70\x63\x2f\x70\x72\x6f\x74\x6f\x2f\x73" +
	"\x65\x72\x76\x69\x63\x65\x73\x2f\x61\x64\x76\x3b\x61\x64\x76\x47\x72\x70\x63\x62\x06\x70\x72\x6f" +
	"\x74\x6f\x33"

var (
	file_services_adv_proto_rawDescOnce sync.Once
	file_services_adv_proto_rawDescData []byte
)

func file_services_adv_proto_rawDescGZIP() []byte {
	file_services_adv_proto_rawDescOnce.Do(func() {
		file_services_adv_proto_rawDescData = protoimpl.X.CompressGZIP(unsafe.Slice(unsafe.StringData(file_services_adv_proto_rawDesc), len(file_services_adv_proto_rawDesc)))
	})
	return file_services_adv_proto_rawDescData
}

var file_services_adv_proto_msgTypes = make([]protoimpl.MessageInfo, 4)
var file_services_adv_proto_goTypes = []any{
	(*DoAuctionRequest)(nil),      // adv.DoAuctionRequest
	(*DoAuctionResponse)(nil),     // adv.DoAuctionResponse
	nil,                           // adv.DoAuctionRequest.ImpIdUuidEntry
	nil,                           // adv.DoAuctionResponse.WinnerUserIdsEntry
	(*ortb_V2_5.BidRequest)(nil),  // ortb_V2_5.BidRequest
	(*ortb_V2_5.BidResponse)(nil), // ortb_V2_5.BidResponse
}

var file_services_adv_proto_depIdxs = []int32{
	4,
	2,
	5,
	3,
	0,
	1,
	5,
	4,
	4,
	4,
	0,
}

func init() { file_services_adv_proto_init() }
func file_services_adv_proto_init() {
	if File_services_adv_proto != nil {
		return
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_services_adv_proto_rawDesc), len(file_services_adv_proto_rawDesc)),
			NumEnums:      0,
			NumMessages:   4,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_services_adv_proto_goTypes,
		DependencyIndexes: file_services_adv_proto_depIdxs,
		MessageInfos:      file_services_adv_proto_msgTypes,
	}.Build()
	File_services_adv_proto = out.File
	file_services_adv_proto_goTypes = nil
	file_services_adv_proto_depIdxs = nil
}
