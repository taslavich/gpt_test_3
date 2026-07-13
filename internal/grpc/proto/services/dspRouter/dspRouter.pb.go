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

const file_services_dspRouter_proto_rawDesc = "\x0a\x18\x73\x65\x72\x76\x69\x63\x65\x73\x2f\x64\x73\x70\x52\x6f\x75\x74\x65\x72\x2e\x70\x72\x6f" +
	"\x74\x6f\x12\x09\x64\x73\x70\x52\x6f\x75\x74\x65\x72\x1a\x1a\x74\x79\x70\x65\x73\x2f\x6f\x72\x74" +
	"\x62\x5f\x56\x32\x5f\x35\x2f\x6f\x72\x74\x62\x2e\x70\x72\x6f\x74\x6f\x22\xfb\x02\x0a\x15\x44\x73" +
	"\x70\x52\x6f\x75\x74\x65\x72\x52\x65\x71\x75\x65\x73\x74\x5f\x56\x32\x5f\x35\x12\x35\x0a\x0a\x62" +
	"\x69\x64\x52\x65\x71\x75\x65\x73\x74\x18\x01\x20\x01\x28\x0b\x32\x15\x2e\x6f\x72\x74\x62\x5f\x56" +
	"\x32\x5f\x35\x2e\x42\x69\x64\x52\x65\x71\x75\x65\x73\x74\x52\x0a\x62\x69\x64\x52\x65\x71\x75\x65" +
	"\x73\x74\x12\x4d\x0a\x09\x69\x6d\x70\x49\x64\x55\x75\x69\x64\x18\x02\x20\x03\x28\x0b\x32\x2f\x2e" +
	"\x64\x73\x70\x52\x6f\x75\x74\x65\x72\x2e\x44\x73\x70\x52\x6f\x75\x74\x65\x72\x52\x65\x71\x75\x65" +
	"\x73\x74\x5f\x56\x32\x5f\x35\x2e\x49\x6d\x70\x49\x64\x55\x75\x69\x64\x45\x6e\x74\x72\x79\x52\x09" +
	"\x69\x6d\x70\x49\x64\x55\x75\x69\x64\x12\x1d\x0a\x0a\x73\x73\x70\x5f\x64\x6f\x6d\x61\x69\x6e\x18" +
	"\x03\x20\x01\x28\x09\x52\x09\x73\x73\x70\x44\x6f\x6d\x61\x69\x6e\x12\x16\x0a\x06\x6c\x6f\x67\x67" +
	"\x65\x64\x18\x04\x20\x01\x28\x08\x52\x06\x6c\x6f\x67\x67\x65\x64\x12\x14\x0a\x05\x74\x79\x70\x69" +
	"\x63\x18\x05\x20\x01\x28\x09\x52\x05\x74\x79\x70\x69\x63\x12\x16\x0a\x06\x66\x6f\x72\x6d\x61\x74" +
	"\x18\x06\x20\x01\x28\x09\x52\x06\x66\x6f\x72\x6d\x61\x74\x12\x16\x0a\x06\x73\x73\x70\x55\x72\x6c" +
	"\x18\x07\x20\x01\x28\x09\x52\x06\x73\x73\x70\x55\x72\x6c\x12\x21\x0a\x0c\x74\x72\x61\x66\x66\x69" +
	"\x63\x5f\x74\x79\x70\x65\x18\x08\x20\x01\x28\x09\x52\x0b\x74\x72\x61\x66\x66\x69\x63\x54\x79\x70" +
	"\x65\x1a\x3c\x0a\x0e\x49\x6d\x70\x49\x64\x55\x75\x69\x64\x45\x6e\x74\x72\x79\x12\x10\x0a\x03\x6b" +
	"\x65\x79\x18\x01\x20\x01\x28\x09\x52\x03\x6b\x65\x79\x12\x14\x0a\x05\x76\x61\x6c\x75\x65\x18\x02" +
	"\x20\x01\x28\x09\x52\x05\x76\x61\x6c\x75\x65\x3a\x02\x38\x01\x22\xb8\x05\x0a\x16\x44\x73\x70\x52" +
	"\x6f\x75\x74\x65\x72\x52\x65\x73\x70\x6f\x6e\x73\x65\x5f\x56\x32\x5f\x35\x12\x35\x0a\x0a\x62\x69" +
	"\x64\x52\x65\x71\x75\x65\x73\x74\x18\x01\x20\x01\x28\x0b\x32\x15\x2e\x6f\x72\x74\x62\x5f\x56\x32" +
	"\x5f\x35\x2e\x42\x69\x64\x52\x65\x71\x75\x65\x73\x74\x52\x0a\x62\x69\x64\x52\x65\x71\x75\x65\x73" +
	"\x74\x12\x57\x0a\x0c\x62\x69\x64\x52\x65\x73\x70\x6f\x6e\x73\x65\x73\x18\x02\x20\x03\x28\x0b\x32" +
	"\x33\x2e\x64\x73\x70\x52\x6f\x75\x74\x65\x72\x2e\x44\x73\x70\x52\x6f\x75\x74\x65\x72\x52\x65\x73" +
	"\x70\x6f\x6e\x73\x65\x5f\x56\x32\x5f\x35\x2e\x42\x69\x64\x52\x65\x73\x70\x6f\x6e\x73\x65\x73\x45" +
	"\x6e\x74\x72\x79\x52\x0c\x62\x69\x64\x52\x65\x73\x70\x6f\x6e\x73\x65\x73\x12\x1d\x0a\x0a\x73\x73" +
	"\x70\x5f\x64\x6f\x6d\x61\x69\x6e\x18\x03\x20\x01\x28\x09\x52\x09\x73\x73\x70\x44\x6f\x6d\x61\x69" +
	"\x6e\x12\x12\x0a\x04\x63\x6f\x64\x65\x18\x04\x20\x01\x28\x05\x52\x04\x63\x6f\x64\x65\x12\x12\x0a" +
	"\x04\x72\x65\x6b\x6c\x18\x05\x20\x01\x28\x08\x52\x04\x72\x65\x6b\x6c\x12\x42\x0a\x10\x72\x65\x61" +
	"\x64\x79\x42\x69\x64\x52\x65\x73\x70\x6f\x6e\x73\x65\x18\x06\x20\x01\x28\x0b\x32\x16\x2e\x6f\x72" +
	"\x74\x62\x5f\x56\x32\x5f\x35\x2e\x42\x69\x64\x52\x65\x73\x70\x6f\x6e\x73\x65\x52\x10\x72\x65\x61" +
	"\x64\x79\x42\x69\x64\x52\x65\x73\x70\x6f\x6e\x73\x65\x12\x5a\x0a\x0d\x77\x69\x6e\x6e\x65\x72\x55" +
	"\x73\x65\x72\x49\x64\x73\x18\x07\x20\x03\x28\x0b\x32\x34\x2e\x64\x73\x70\x52\x6f\x75\x74\x65\x72" +
	"\x2e\x44\x73\x70\x52\x6f\x75\x74\x65\x72\x52\x65\x73\x70\x6f\x6e\x73\x65\x5f\x56\x32\x5f\x35\x2e" +
	"\x57\x69\x6e\x6e\x65\x72\x55\x73\x65\x72\x49\x64\x73\x45\x6e\x74\x72\x79\x52\x0d\x77\x69\x6e\x6e" +
	"\x65\x72\x55\x73\x65\x72\x49\x64\x73\x12\x4e\x0a\x09\x69\x6d\x70\x49\x64\x55\x75\x69\x64\x18\x08" +
	"\x20\x03\x28\x0b\x32\x30\x2e\x64\x73\x70\x52\x6f\x75\x74\x65\x72\x2e\x44\x73\x70\x52\x6f\x75\x74" +
	"\x65\x72\x52\x65\x73\x70\x6f\x6e\x73\x65\x5f\x56\x32\x5f\x35\x2e\x49\x6d\x70\x49\x64\x55\x75\x69" +
	"\x64\x45\x6e\x74\x72\x79\x52\x09\x69\x6d\x70\x49\x64\x55\x75\x69\x64\x1a\x57\x0a\x11\x42\x69\x64" +
	"\x52\x65\x73\x70\x6f\x6e\x73\x65\x73\x45\x6e\x74\x72\x79\x12\x10\x0a\x03\x6b\x65\x79\x18\x01\x20" +
	"\x01\x28\x09\x52\x03\x6b\x65\x79\x12\x2c\x0a\x05\x76\x61\x6c\x75\x65\x18\x02\x20\x01\x28\x0b\x32" +
	"\x16\x2e\x6f\x72\x74\x62\x5f\x56\x32\x5f\x35\x2e\x42\x69\x64\x52\x65\x73\x70\x6f\x6e\x73\x65\x52" +
	"\x05\x76\x61\x6c\x75\x65\x3a\x02\x38\x01\x1a\x40\x0a\x12\x57\x69\x6e\x6e\x65\x72\x55\x73\x65\x72" +
	"\x49\x64\x73\x45\x6e\x74\x72\x79\x12\x10\x0a\x03\x6b\x65\x79\x18\x01\x20\x01\x28\x09\x52\x03\x6b" +
	"\x65\x79\x12\x14\x0a\x05\x76\x61\x6c\x75\x65\x18\x02\x20\x01\x28\x09\x52\x05\x76\x61\x6c\x75\x65" +
	"\x3a\x02\x38\x01\x1a\x3c\x0a\x0e\x49\x6d\x70\x49\x64\x55\x75\x69\x64\x45\x6e\x74\x72\x79\x12\x10" +
	"\x0a\x03\x6b\x65\x79\x18\x01\x20\x01\x28\x09\x52\x03\x6b\x65\x79\x12\x14\x0a\x05\x76\x61\x6c\x75" +
	"\x65\x18\x02\x20\x01\x28\x09\x52\x05\x76\x61\x6c\x75\x65\x3a\x02\x38\x01\x32\x67\x0a\x10\x44\x73" +
	"\x70\x52\x6f\x75\x74\x65\x72\x53\x65\x72\x76\x69\x63\x65\x12\x53\x0a\x0c\x47\x65\x74\x42\x69\x64" +
	"\x73\x5f\x56\x32\x5f\x35\x12\x20\x2e\x64\x73\x70\x52\x6f\x75\x74\x65\x72\x2e\x44\x73\x70\x52\x6f" +
	"\x75\x74\x65\x72\x52\x65\x71\x75\x65\x73\x74\x5f\x56\x32\x5f\x35\x1a\x21\x2e\x64\x73\x70\x52\x6f" +
	"\x75\x74\x65\x72\x2e\x44\x73\x70\x52\x6f\x75\x74\x65\x72\x52\x65\x73\x70\x6f\x6e\x73\x65\x5f\x56" +
	"\x32\x5f\x35\x42\x5f\x5a\x5d\x67\x69\x74\x6c\x61\x62\x2e\x63\x6f\x6d\x2f\x74\x77\x69\x6e\x62\x69" +
	"\x64\x2d\x65\x78\x63\x68\x61\x6e\x67\x65\x2f\x52\x54\x42\x2d\x65\x78\x63\x68\x61\x6e\x67\x65\x2f" +
	"\x69\x6e\x74\x65\x72\x6e\x61\x6c\x2f\x67\x72\x70\x63\x2f\x70\x72\x6f\x74\x6f\x2f\x73\x65\x72\x76" +
	"\x69\x63\x65\x73\x2f\x64\x73\x70\x52\x6f\x75\x74\x65\x72\x3b\x64\x73\x70\x52\x6f\x75\x74\x65\x72" +
	"\x47\x72\x70\x63\x62\x06\x70\x72\x6f\x74\x6f\x33"

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
