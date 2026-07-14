// Code generated for this repository. DO NOT EDIT.
// source: services/dspRouter.proto

package dspRouterGrpc

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

type DspRouterRequest_V2_5 struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	BidRequest    *ortb_V2_5.BidRequest  `protobuf:"bytes,1,opt,name=bidRequest,proto3" json:"bidRequest,omitempty"`
	ImpIdUuid     map[string]string      `protobuf:"bytes,2,rep,name=impIdUuid,proto3" json:"impIdUuid,omitempty" protobuf_key:"bytes,1,opt,name=key" protobuf_val:"bytes,2,opt,name=value"`
	SspDomain     string                 `protobuf:"bytes,3,opt,name=ssp_domain,json=sspDomain,proto3" json:"ssp_domain,omitempty"`
	Logged        bool                   `protobuf:"varint,4,opt,name=logged,proto3" json:"logged,omitempty"`
	Typic         string                 `protobuf:"bytes,5,opt,name=typic,proto3" json:"typic,omitempty"`
	Format        string                 `protobuf:"bytes,6,opt,name=format,proto3" json:"format,omitempty"`
	SspUrl        string                 `protobuf:"bytes,7,opt,name=sspUrl,proto3" json:"sspUrl,omitempty"`
	TrafficType   string                 `protobuf:"bytes,8,opt,name=traffic_type,json=trafficType,proto3" json:"traffic_type,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *DspRouterRequest_V2_5) Reset() {
	*x = DspRouterRequest_V2_5{}
	mi := &file_services_dspRouter_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *DspRouterRequest_V2_5) String() string { return protoimpl.X.MessageStringOf(x) }

func (*DspRouterRequest_V2_5) ProtoMessage() {}

func (x *DspRouterRequest_V2_5) ProtoReflect() protoreflect.Message {
	mi := &file_services_dspRouter_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

func (*DspRouterRequest_V2_5) Descriptor() ([]byte, []int) {
	return file_services_dspRouter_proto_rawDescGZIP(), []int{0}
}

func (x *DspRouterRequest_V2_5) GetBidRequest() *ortb_V2_5.BidRequest {
	if x != nil {
		return x.BidRequest
	}
	return nil
}

func (x *DspRouterRequest_V2_5) GetImpIdUuid() map[string]string {
	if x != nil {
		return x.ImpIdUuid
	}
	return nil
}

func (x *DspRouterRequest_V2_5) GetSspDomain() string {
	if x != nil {
		return x.SspDomain
	}
	return ""
}

func (x *DspRouterRequest_V2_5) GetLogged() bool {
	if x != nil {
		return x.Logged
	}
	return false
}

func (x *DspRouterRequest_V2_5) GetTypic() string {
	if x != nil {
		return x.Typic
	}
	return ""
}

func (x *DspRouterRequest_V2_5) GetFormat() string {
	if x != nil {
		return x.Format
	}
	return ""
}

func (x *DspRouterRequest_V2_5) GetSspUrl() string {
	if x != nil {
		return x.SspUrl
	}
	return ""
}

func (x *DspRouterRequest_V2_5) GetTrafficType() string {
	if x != nil {
		return x.TrafficType
	}
	return ""
}

type DspRouterResponse_V2_5 struct {
	state            protoimpl.MessageState            `protogen:"open.v1"`
	BidRequest       *ortb_V2_5.BidRequest             `protobuf:"bytes,1,opt,name=bidRequest,proto3" json:"bidRequest,omitempty"`
	BidResponses     map[string]*ortb_V2_5.BidResponse `protobuf:"bytes,2,rep,name=bidResponses,proto3" json:"bidResponses,omitempty" protobuf_key:"bytes,1,opt,name=key" protobuf_val:"bytes,2,opt,name=value"`
	SspDomain        string                            `protobuf:"bytes,3,opt,name=ssp_domain,json=sspDomain,proto3" json:"ssp_domain,omitempty"`
	Code             int32                             `protobuf:"varint,4,opt,name=code,proto3" json:"code,omitempty"`
	Rekl             bool                              `protobuf:"varint,5,opt,name=rekl,proto3" json:"rekl,omitempty"`
	ReadyBidResponse *ortb_V2_5.BidResponse            `protobuf:"bytes,6,opt,name=readyBidResponse,proto3" json:"readyBidResponse,omitempty"`
	WinnerUserIds    map[string]string                 `protobuf:"bytes,7,rep,name=winnerUserIds,proto3" json:"winnerUserIds,omitempty" protobuf_key:"bytes,1,opt,name=key" protobuf_val:"bytes,2,opt,name=value"`
	ImpIdUuid        map[string]string                 `protobuf:"bytes,8,rep,name=impIdUuid,proto3" json:"impIdUuid,omitempty" protobuf_key:"bytes,1,opt,name=key" protobuf_val:"bytes,2,opt,name=value"`
	unknownFields    protoimpl.UnknownFields
	sizeCache        protoimpl.SizeCache
}

func (x *DspRouterResponse_V2_5) Reset() {
	*x = DspRouterResponse_V2_5{}
	mi := &file_services_dspRouter_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *DspRouterResponse_V2_5) String() string { return protoimpl.X.MessageStringOf(x) }

func (*DspRouterResponse_V2_5) ProtoMessage() {}

func (x *DspRouterResponse_V2_5) ProtoReflect() protoreflect.Message {
	mi := &file_services_dspRouter_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

func (*DspRouterResponse_V2_5) Descriptor() ([]byte, []int) {
	return file_services_dspRouter_proto_rawDescGZIP(), []int{1}
}

func (x *DspRouterResponse_V2_5) GetBidRequest() *ortb_V2_5.BidRequest {
	if x != nil {
		return x.BidRequest
	}
	return nil
}

func (x *DspRouterResponse_V2_5) GetBidResponses() map[string]*ortb_V2_5.BidResponse {
	if x != nil {
		return x.BidResponses
	}
	return nil
}

func (x *DspRouterResponse_V2_5) GetSspDomain() string {
	if x != nil {
		return x.SspDomain
	}
	return ""
}

func (x *DspRouterResponse_V2_5) GetCode() int32 {
	if x != nil {
		return x.Code
	}
	return 0
}

func (x *DspRouterResponse_V2_5) GetRekl() bool {
	if x != nil {
		return x.Rekl
	}
	return false
}

func (x *DspRouterResponse_V2_5) GetReadyBidResponse() *ortb_V2_5.BidResponse {
	if x != nil {
		return x.ReadyBidResponse
	}
	return nil
}

func (x *DspRouterResponse_V2_5) GetWinnerUserIds() map[string]string {
	if x != nil {
		return x.WinnerUserIds
	}
	return nil
}

func (x *DspRouterResponse_V2_5) GetImpIdUuid() map[string]string {
	if x != nil {
		return x.ImpIdUuid
	}
	return nil
}

var File_services_dspRouter_proto protoreflect.FileDescriptor

const file_services_dspRouter_proto_rawDesc = "\n\x18services/dspRouter.proto\x12\tdspRouter\x1a\x1atypes/ortb_V2_5/ortb.proto\"\x87\x03\n\x15DspRouterRequest_V2_" +
	"5\x125\n\nbidRequest\x18\x01 \x01(\x0b2\x15.ortb_V2_5.BidRequestR\nbidRequest\x12M\n\timpIdUuid\x18\x02 \x03(\x0b2/.dspRouter.Ds" +
	"pRouterRequest_V2_5.ImpIdUuidEntryR\timpIdUuid\x12\x1d\n\nssp_domain\x18\x03 \x01(\tR\tsspDomain\x12\x16\n\x06logged\x18\x04 \x01" +
	"(\x08R\x06logged\x12\x14\n\x05typic\x18\x05 \x01(\tR\x05typic\x12\x16\n\x06format\x18\x06 \x01(\tR\x06format\x12\x16\n\x06sspUrl\x18\x07 \x01(\tR\x06sspUrl\x12!\n\x0ctraffi" +
	"c_type\x18\x08 \x01(\tR\x0btrafficType\x1a<\n\x0eImpIdUuidEntry\x12\x10\n\x03key\x18\x01 \x01(\tR\x03key\x12\x14\n\x05value\x18\x02 \x01(\tR\x05value:\x028\x01J\x04\x08" +
	"\t\x10\nR\x04feed\"\xb8\x05\n\x16DspRouterResponse_V2_5\x125\n\nbidRequest\x18\x01 \x01(\x0b2\x15.ortb_V2_5.BidRequestR\nbidReques" +
	"t\x12W\n\x0cbidResponses\x18\x02 \x03(\x0b23.dspRouter.DspRouterResponse_V2_5.BidResponsesEntryR\x0cbidResponses" +
	"\x12\x1d\n\nssp_domain\x18\x03 \x01(\tR\tsspDomain\x12\x12\n\x04code\x18\x04 \x01(\x05R\x04code\x12\x12\n\x04rekl\x18\x05 \x01(\x08R\x04rekl\x12B\n\x10readyBidRespons" +
	"e\x18\x06 \x01(\x0b2\x16.ortb_V2_5.BidResponseR\x10readyBidResponse\x12Z\n\rwinnerUserIds\x18\x07 \x03(\x0b24.dspRouter.DspRo" +
	"uterResponse_V2_5.WinnerUserIdsEntryR\rwinnerUserIds\x12N\n\timpIdUuid\x18\x08 \x03(\x0b20.dspRouter.DspRout" +
	"erResponse_V2_5.ImpIdUuidEntryR\timpIdUuid\x1aW\n\x11BidResponsesEntry\x12\x10\n\x03key\x18\x01 \x01(\tR\x03key\x12,\n\x05value\x18" +
	"\x02 \x01(\x0b2\x16.ortb_V2_5.BidResponseR\x05value:\x028\x01\x1a@\n\x12WinnerUserIdsEntry\x12\x10\n\x03key\x18\x01 \x01(\tR\x03key\x12\x14\n\x05value\x18" +
	"\x02 \x01(\tR\x05value:\x028\x01\x1a<\n\x0eImpIdUuidEntry\x12\x10\n\x03key\x18\x01 \x01(\tR\x03key\x12\x14\n\x05value\x18\x02 \x01(\tR\x05value:\x028\x012g\n\x10DspRoute" +
	"rService\x12S\n\x0cGetBids_V2_5\x12 .dspRouter.DspRouterRequest_V2_5\x1a!.dspRouter.DspRouterResponse_V" +
	"2_5B_Z]gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/dspRouter;dsp" +
	"RouterGrpcb\x06proto3"

var (
	file_services_dspRouter_proto_rawDescOnce sync.Once
	file_services_dspRouter_proto_rawDescData []byte
)

func file_services_dspRouter_proto_rawDescGZIP() []byte {
	file_services_dspRouter_proto_rawDescOnce.Do(func() {
		file_services_dspRouter_proto_rawDescData = protoimpl.X.CompressGZIP(unsafe.Slice(unsafe.StringData(file_services_dspRouter_proto_rawDesc), len(file_services_dspRouter_proto_rawDesc)))
	})
	return file_services_dspRouter_proto_rawDescData
}

var file_services_dspRouter_proto_msgTypes = make([]protoimpl.MessageInfo, 6)
var file_services_dspRouter_proto_goTypes = []any{
	(*DspRouterRequest_V2_5)(nil),  // dspRouter.DspRouterRequest_V2_5
	(*DspRouterResponse_V2_5)(nil), // dspRouter.DspRouterResponse_V2_5
	nil,                            // dspRouter.DspRouterRequest_V2_5.ImpIdUuidEntry
	nil,                            // dspRouter.DspRouterResponse_V2_5.BidResponsesEntry
	nil,                            // dspRouter.DspRouterResponse_V2_5.WinnerUserIdsEntry
	nil,                            // dspRouter.DspRouterResponse_V2_5.ImpIdUuidEntry
	(*ortb_V2_5.BidRequest)(nil),   // ortb_V2_5.BidRequest
	(*ortb_V2_5.BidResponse)(nil),  // ortb_V2_5.BidResponse
}

var file_services_dspRouter_proto_depIdxs = []int32{
	6,
	2,
	6,
	3,
	7,
	4,
	5,
	7,
	0,
	1,
	9,
	8,
	8,
	8,
	0,
}

func init() { file_services_dspRouter_proto_init() }
func file_services_dspRouter_proto_init() {
	if File_services_dspRouter_proto != nil {
		return
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_services_dspRouter_proto_rawDesc), len(file_services_dspRouter_proto_rawDesc)),
			NumEnums:      0,
			NumMessages:   6,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_services_dspRouter_proto_goTypes,
		DependencyIndexes: file_services_dspRouter_proto_depIdxs,
		MessageInfos:      file_services_dspRouter_proto_msgTypes,
	}.Build()
	File_services_dspRouter_proto = out.File
	file_services_dspRouter_proto_goTypes = nil
	file_services_dspRouter_proto_depIdxs = nil
}
