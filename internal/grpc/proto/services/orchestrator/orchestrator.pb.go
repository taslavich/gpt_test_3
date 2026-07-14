// Code generated for this repository. DO NOT EDIT.
// source: services/orchestrator.proto

package orchestratorGrpc

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

type OrchestratorRequest_V2_5 struct {
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

func (x *OrchestratorRequest_V2_5) Reset() {
	*x = OrchestratorRequest_V2_5{}
	mi := &file_services_orchestrator_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *OrchestratorRequest_V2_5) String() string { return protoimpl.X.MessageStringOf(x) }

func (*OrchestratorRequest_V2_5) ProtoMessage() {}

func (x *OrchestratorRequest_V2_5) ProtoReflect() protoreflect.Message {
	mi := &file_services_orchestrator_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

func (*OrchestratorRequest_V2_5) Descriptor() ([]byte, []int) {
	return file_services_orchestrator_proto_rawDescGZIP(), []int{0}
}

func (x *OrchestratorRequest_V2_5) GetBidRequest() *ortb_V2_5.BidRequest {
	if x != nil {
		return x.BidRequest
	}
	return nil
}

func (x *OrchestratorRequest_V2_5) GetImpIdUuid() map[string]string {
	if x != nil {
		return x.ImpIdUuid
	}
	return nil
}

func (x *OrchestratorRequest_V2_5) GetSspDomain() string {
	if x != nil {
		return x.SspDomain
	}
	return ""
}

func (x *OrchestratorRequest_V2_5) GetLogged() bool {
	if x != nil {
		return x.Logged
	}
	return false
}

func (x *OrchestratorRequest_V2_5) GetTypic() string {
	if x != nil {
		return x.Typic
	}
	return ""
}

func (x *OrchestratorRequest_V2_5) GetFormat() string {
	if x != nil {
		return x.Format
	}
	return ""
}

func (x *OrchestratorRequest_V2_5) GetSspUrl() string {
	if x != nil {
		return x.SspUrl
	}
	return ""
}

func (x *OrchestratorRequest_V2_5) GetTrafficType() string {
	if x != nil {
		return x.TrafficType
	}
	return ""
}

type OrchestratorResponse_V2_5 struct {
	state          protoimpl.MessageState `protogen:"open.v1"`
	BidResponse    *ortb_V2_5.BidResponse `protobuf:"bytes,1,opt,name=bidResponse,proto3" json:"bidResponse,omitempty"`
	GlobalId       string                 `protobuf:"bytes,2,opt,name=globalId,proto3" json:"globalId,omitempty"`
	Code           int32                  `protobuf:"varint,3,opt,name=code,proto3" json:"code,omitempty"`
	FailedImpIds   []string               `protobuf:"bytes,4,rep,name=failedImpIds,proto3" json:"failedImpIds,omitempty"`
	ImpIdUuidClone map[string]string      `protobuf:"bytes,5,rep,name=impIdUuidClone,proto3" json:"impIdUuidClone,omitempty" protobuf_key:"bytes,1,opt,name=key" protobuf_val:"bytes,2,opt,name=value"`
	Rekl           bool                   `protobuf:"varint,6,opt,name=rekl,proto3" json:"rekl,omitempty"`
	WinnerUserIds  map[string]string      `protobuf:"bytes,7,rep,name=winnerUserIds,proto3" json:"winnerUserIds,omitempty" protobuf_key:"bytes,1,opt,name=key" protobuf_val:"bytes,2,opt,name=value"`
	unknownFields  protoimpl.UnknownFields
	sizeCache      protoimpl.SizeCache
}

func (x *OrchestratorResponse_V2_5) Reset() {
	*x = OrchestratorResponse_V2_5{}
	mi := &file_services_orchestrator_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *OrchestratorResponse_V2_5) String() string { return protoimpl.X.MessageStringOf(x) }

func (*OrchestratorResponse_V2_5) ProtoMessage() {}

func (x *OrchestratorResponse_V2_5) ProtoReflect() protoreflect.Message {
	mi := &file_services_orchestrator_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

func (*OrchestratorResponse_V2_5) Descriptor() ([]byte, []int) {
	return file_services_orchestrator_proto_rawDescGZIP(), []int{1}
}

func (x *OrchestratorResponse_V2_5) GetBidResponse() *ortb_V2_5.BidResponse {
	if x != nil {
		return x.BidResponse
	}
	return nil
}

func (x *OrchestratorResponse_V2_5) GetGlobalId() string {
	if x != nil {
		return x.GlobalId
	}
	return ""
}

func (x *OrchestratorResponse_V2_5) GetCode() int32 {
	if x != nil {
		return x.Code
	}
	return 0
}

func (x *OrchestratorResponse_V2_5) GetFailedImpIds() []string {
	if x != nil {
		return x.FailedImpIds
	}
	return nil
}

func (x *OrchestratorResponse_V2_5) GetImpIdUuidClone() map[string]string {
	if x != nil {
		return x.ImpIdUuidClone
	}
	return nil
}

func (x *OrchestratorResponse_V2_5) GetRekl() bool {
	if x != nil {
		return x.Rekl
	}
	return false
}

func (x *OrchestratorResponse_V2_5) GetWinnerUserIds() map[string]string {
	if x != nil {
		return x.WinnerUserIds
	}
	return nil
}

var File_services_orchestrator_proto protoreflect.FileDescriptor

const file_services_orchestrator_proto_rawDesc = "\n\x1bservices/orchestrator.proto\x12\x0corchestrator\x1a\x1atypes/ortb_V2_5/ortb.proto\"\x90\x03\n\x18OrchestratorRe" +
	"quest_V2_5\x125\n\nbidRequest\x18\x01 \x01(\x0b2\x15.ortb_V2_5.BidRequestR\nbidRequest\x12S\n\timpIdUuid\x18\x02 \x03(\x0b25.orc" +
	"hestrator.OrchestratorRequest_V2_5.ImpIdUuidEntryR\timpIdUuid\x12\x1d\n\nssp_domain\x18\x03 \x01(\tR\tsspDomai" +
	"n\x12\x16\n\x06logged\x18\x04 \x01(\x08R\x06logged\x12\x14\n\x05typic\x18\x05 \x01(\tR\x05typic\x12\x16\n\x06format\x18\x06 \x01(\tR\x06format\x12\x16\n\x06sspUrl\x18\x07 \x01(\tR\x06s" +
	"spUrl\x12!\n\x0ctraffic_type\x18\x08 \x01(\tR\x0btrafficType\x1a<\n\x0eImpIdUuidEntry\x12\x10\n\x03key\x18\x01 \x01(\tR\x03key\x12\x14\n\x05value\x18\x02 \x01(" +
	"\tR\x05value:\x028\x01J\x04\x08\t\x10\nR\x04feed\"\x89\x04\n\x19OrchestratorResponse_V2_5\x128\n\x0bbidResponse\x18\x01 \x01(\x0b2\x16.ortb_V2_5.Bi" +
	"dResponseR\x0bbidResponse\x12\x1a\n\x08globalId\x18\x02 \x01(\tR\x08globalId\x12\x12\n\x04code\x18\x03 \x01(\x05R\x04code\x12\"\n\x0cfailedImpIds\x18\x04 \x03" +
	"(\tR\x0cfailedImpIds\x12c\n\x0eimpIdUuidClone\x18\x05 \x03(\x0b2;.orchestrator.OrchestratorResponse_V2_5.ImpIdUui" +
	"dCloneEntryR\x0eimpIdUuidClone\x12\x12\n\x04rekl\x18\x06 \x01(\x08R\x04rekl\x12`\n\rwinnerUserIds\x18\x07 \x03(\x0b2:.orchestrator.Orch" +
	"estratorResponse_V2_5.WinnerUserIdsEntryR\rwinnerUserIds\x1aA\n\x13ImpIdUuidCloneEntry\x12\x10\n\x03key\x18\x01 \x01(" +
	"\tR\x03key\x12\x14\n\x05value\x18\x02 \x01(\tR\x05value:\x028\x01\x1a@\n\x12WinnerUserIdsEntry\x12\x10\n\x03key\x18\x01 \x01(\tR\x03key\x12\x14\n\x05value\x18\x02 \x01(\tR\x05v" +
	"alue:\x028\x012}\n\x13OrchestratorService\x12f\n\x11getWinnerBid_V2_5\x12&.orchestrator.OrchestratorRequest_V2" +
	"_5\x1a'.orchestrator.OrchestratorResponse_V2_5\"\x00BeZcgitlab.com/twinbid-exchange/RTB-exchange/" +
	"internal/grpc/proto/services/orchestrator;orchestratorGrpcb\x06proto3"

var (
	file_services_orchestrator_proto_rawDescOnce sync.Once
	file_services_orchestrator_proto_rawDescData []byte
)

func file_services_orchestrator_proto_rawDescGZIP() []byte {
	file_services_orchestrator_proto_rawDescOnce.Do(func() {
		file_services_orchestrator_proto_rawDescData = protoimpl.X.CompressGZIP(unsafe.Slice(unsafe.StringData(file_services_orchestrator_proto_rawDesc), len(file_services_orchestrator_proto_rawDesc)))
	})
	return file_services_orchestrator_proto_rawDescData
}

var file_services_orchestrator_proto_msgTypes = make([]protoimpl.MessageInfo, 5)
var file_services_orchestrator_proto_goTypes = []any{
	(*OrchestratorRequest_V2_5)(nil),  // orchestrator.OrchestratorRequest_V2_5
	(*OrchestratorResponse_V2_5)(nil), // orchestrator.OrchestratorResponse_V2_5
	nil,                               // orchestrator.OrchestratorRequest_V2_5.ImpIdUuidEntry
	nil,                               // orchestrator.OrchestratorResponse_V2_5.ImpIdUuidCloneEntry
	nil,                               // orchestrator.OrchestratorResponse_V2_5.WinnerUserIdsEntry
	(*ortb_V2_5.BidRequest)(nil),      // ortb_V2_5.BidRequest
	(*ortb_V2_5.BidResponse)(nil),     // ortb_V2_5.BidResponse
}

var file_services_orchestrator_proto_depIdxs = []int32{
	5,
	2,
	6,
	3,
	4,
	0,
	1,
	6,
	5,
	5,
	5,
	0,
}

func init() { file_services_orchestrator_proto_init() }
func file_services_orchestrator_proto_init() {
	if File_services_orchestrator_proto != nil {
		return
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_services_orchestrator_proto_rawDesc), len(file_services_orchestrator_proto_rawDesc)),
			NumEnums:      0,
			NumMessages:   5,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_services_orchestrator_proto_goTypes,
		DependencyIndexes: file_services_orchestrator_proto_depIdxs,
		MessageInfos:      file_services_orchestrator_proto_msgTypes,
	}.Build()
	File_services_orchestrator_proto = out.File
	file_services_orchestrator_proto_goTypes = nil
	file_services_orchestrator_proto_depIdxs = nil
}
