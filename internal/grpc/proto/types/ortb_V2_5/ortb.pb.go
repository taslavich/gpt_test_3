// Code generated for this repository. DO NOT EDIT.
// source: types/ortb_V2_5/ortb.proto

package ortb_V2_5

import (
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

type BidRequest struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Id            *string                `protobuf:"bytes,1,opt,name=id,proto3,oneof" json:"id,omitempty"`
	At            *int32                 `protobuf:"varint,2,opt,name=at,proto3,oneof" json:"at,omitempty"`
	Imp           []*Imp                 `protobuf:"bytes,3,rep,name=imp,proto3" json:"imp,omitempty"`
	Device        *Device                `protobuf:"bytes,4,opt,name=device,proto3,oneof" json:"device,omitempty"`
	Site          *Site                  `protobuf:"bytes,5,opt,name=site,proto3,oneof" json:"site,omitempty"`
	User          *User                  `protobuf:"bytes,6,opt,name=user,proto3,oneof" json:"user,omitempty"`
	Tmax          *int32                 `protobuf:"varint,7,opt,name=tmax,proto3,oneof" json:"tmax,omitempty"`
	Cur           []string               `protobuf:"bytes,8,rep,name=cur,proto3" json:"cur,omitempty"`
	Bcat          []string               `protobuf:"bytes,9,rep,name=bcat,proto3" json:"bcat,omitempty"`
	Test          *int32                 `protobuf:"varint,10,opt,name=test,proto3,oneof" json:"test,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *BidRequest) Reset() {
	*x = BidRequest{}
	mi := &file_types_ortb_V2_5_ortb_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *BidRequest) String() string { return protoimpl.X.MessageStringOf(x) }

func (*BidRequest) ProtoMessage() {}

func (x *BidRequest) ProtoReflect() protoreflect.Message {
	mi := &file_types_ortb_V2_5_ortb_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

func (*BidRequest) Descriptor() ([]byte, []int) {
	return file_types_ortb_V2_5_ortb_proto_rawDescGZIP(), []int{0}
}

func (x *BidRequest) GetId() string {
	if x != nil && x.Id != nil {
		return *x.Id
	}
	return ""
}

func (x *BidRequest) GetAt() int32 {
	if x != nil && x.At != nil {
		return *x.At
	}
	return 0
}

func (x *BidRequest) GetImp() []*Imp {
	if x != nil {
		return x.Imp
	}
	return nil
}

func (x *BidRequest) GetDevice() *Device {
	if x != nil {
		return x.Device
	}
	return nil
}

func (x *BidRequest) GetSite() *Site {
	if x != nil {
		return x.Site
	}
	return nil
}

func (x *BidRequest) GetUser() *User {
	if x != nil {
		return x.User
	}
	return nil
}

func (x *BidRequest) GetTmax() int32 {
	if x != nil && x.Tmax != nil {
		return *x.Tmax
	}
	return 0
}

func (x *BidRequest) GetCur() []string {
	if x != nil {
		return x.Cur
	}
	return nil
}

func (x *BidRequest) GetBcat() []string {
	if x != nil {
		return x.Bcat
	}
	return nil
}

func (x *BidRequest) GetTest() int32 {
	if x != nil && x.Test != nil {
		return *x.Test
	}
	return 0
}

type Imp struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Id            *string                `protobuf:"bytes,1,opt,name=id,proto3,oneof" json:"id,omitempty"`
	Bidfloor      *float32               `protobuf:"fixed32,2,opt,name=bidfloor,proto3,oneof" json:"bidfloor,omitempty"`
	Tagid         *string                `protobuf:"bytes,3,opt,name=tagid,proto3,oneof" json:"tagid,omitempty"`
	Secure        *int32                 `protobuf:"varint,4,opt,name=secure,proto3,oneof" json:"secure,omitempty"`
	Instl         *int32                 `protobuf:"varint,5,opt,name=instl,proto3,oneof" json:"instl,omitempty"`
	Bidfloorcur   *string                `protobuf:"bytes,6,opt,name=bidfloorcur,proto3,oneof" json:"bidfloorcur,omitempty"`
	Ext           *Imp_Ext               `protobuf:"bytes,7,opt,name=ext,proto3,oneof" json:"ext,omitempty"`
	Banner        *Banner                `protobuf:"bytes,8,opt,name=banner,proto3,oneof" json:"banner,omitempty"`
	Native        *Native                `protobuf:"bytes,9,opt,name=native,proto3,oneof" json:"native,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *Imp) Reset() {
	*x = Imp{}
	mi := &file_types_ortb_V2_5_ortb_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Imp) String() string { return protoimpl.X.MessageStringOf(x) }

func (*Imp) ProtoMessage() {}

func (x *Imp) ProtoReflect() protoreflect.Message {
	mi := &file_types_ortb_V2_5_ortb_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

func (*Imp) Descriptor() ([]byte, []int) {
	return file_types_ortb_V2_5_ortb_proto_rawDescGZIP(), []int{1}
}

func (x *Imp) GetId() string {
	if x != nil && x.Id != nil {
		return *x.Id
	}
	return ""
}

func (x *Imp) GetBidfloor() float32 {
	if x != nil && x.Bidfloor != nil {
		return *x.Bidfloor
	}
	return 0
}

func (x *Imp) GetTagid() string {
	if x != nil && x.Tagid != nil {
		return *x.Tagid
	}
	return ""
}

func (x *Imp) GetSecure() int32 {
	if x != nil && x.Secure != nil {
		return *x.Secure
	}
	return 0
}

func (x *Imp) GetInstl() int32 {
	if x != nil && x.Instl != nil {
		return *x.Instl
	}
	return 0
}

func (x *Imp) GetBidfloorcur() string {
	if x != nil && x.Bidfloorcur != nil {
		return *x.Bidfloorcur
	}
	return ""
}

func (x *Imp) GetExt() *Imp_Ext {
	if x != nil {
		return x.Ext
	}
	return nil
}

func (x *Imp) GetBanner() *Banner {
	if x != nil {
		return x.Banner
	}
	return nil
}

func (x *Imp) GetNative() *Native {
	if x != nil {
		return x.Native
	}
	return nil
}

type Banner struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	W             *int32                 `protobuf:"varint,1,opt,name=w,proto3,oneof" json:"w,omitempty"`
	H             *int32                 `protobuf:"varint,2,opt,name=h,proto3,oneof" json:"h,omitempty"`
	Pos           *int32                 `protobuf:"varint,3,opt,name=pos,proto3,oneof" json:"pos,omitempty"`
	Mimes         []string               `protobuf:"bytes,4,rep,name=mimes,proto3" json:"mimes,omitempty"`
	Api           *string                `protobuf:"bytes,5,opt,name=api,proto3,oneof" json:"api,omitempty"`
	Topframe      *int32                 `protobuf:"varint,6,opt,name=topframe,proto3,oneof" json:"topframe,omitempty"`
	Btype         []int32                `protobuf:"varint,7,rep,name=btype,proto3" json:"btype,omitempty"`
	Battr         []int32                `protobuf:"varint,8,rep,name=battr,proto3" json:"battr,omitempty"`
	Ext           []string               `protobuf:"bytes,9,rep,name=ext,proto3" json:"ext,omitempty"`
	Format        []*Format              `protobuf:"bytes,10,rep,name=format,proto3" json:"format,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *Banner) Reset() {
	*x = Banner{}
	mi := &file_types_ortb_V2_5_ortb_proto_msgTypes[2]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Banner) String() string { return protoimpl.X.MessageStringOf(x) }

func (*Banner) ProtoMessage() {}

func (x *Banner) ProtoReflect() protoreflect.Message {
	mi := &file_types_ortb_V2_5_ortb_proto_msgTypes[2]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

func (*Banner) Descriptor() ([]byte, []int) {
	return file_types_ortb_V2_5_ortb_proto_rawDescGZIP(), []int{2}
}

func (x *Banner) GetW() int32 {
	if x != nil && x.W != nil {
		return *x.W
	}
	return 0
}

func (x *Banner) GetH() int32 {
	if x != nil && x.H != nil {
		return *x.H
	}
	return 0
}

func (x *Banner) GetPos() int32 {
	if x != nil && x.Pos != nil {
		return *x.Pos
	}
	return 0
}

func (x *Banner) GetMimes() []string {
	if x != nil {
		return x.Mimes
	}
	return nil
}

func (x *Banner) GetApi() string {
	if x != nil && x.Api != nil {
		return *x.Api
	}
	return ""
}

func (x *Banner) GetTopframe() int32 {
	if x != nil && x.Topframe != nil {
		return *x.Topframe
	}
	return 0
}

func (x *Banner) GetBtype() []int32 {
	if x != nil {
		return x.Btype
	}
	return nil
}

func (x *Banner) GetBattr() []int32 {
	if x != nil {
		return x.Battr
	}
	return nil
}

func (x *Banner) GetExt() []string {
	if x != nil {
		return x.Ext
	}
	return nil
}

func (x *Banner) GetFormat() []*Format {
	if x != nil {
		return x.Format
	}
	return nil
}

type Format struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	W             *int32                 `protobuf:"varint,1,opt,name=w,proto3,oneof" json:"w,omitempty"`
	H             *int32                 `protobuf:"varint,2,opt,name=h,proto3,oneof" json:"h,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *Format) Reset() {
	*x = Format{}
	mi := &file_types_ortb_V2_5_ortb_proto_msgTypes[3]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Format) String() string { return protoimpl.X.MessageStringOf(x) }

func (*Format) ProtoMessage() {}

func (x *Format) ProtoReflect() protoreflect.Message {
	mi := &file_types_ortb_V2_5_ortb_proto_msgTypes[3]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

func (*Format) Descriptor() ([]byte, []int) {
	return file_types_ortb_V2_5_ortb_proto_rawDescGZIP(), []int{3}
}

func (x *Format) GetW() int32 {
	if x != nil && x.W != nil {
		return *x.W
	}
	return 0
}

func (x *Format) GetH() int32 {
	if x != nil && x.H != nil {
		return *x.H
	}
	return 0
}

type Native struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Request       *string                `protobuf:"bytes,1,opt,name=request,proto3,oneof" json:"request,omitempty"`
	Ver           *string                `protobuf:"bytes,2,opt,name=ver,proto3,oneof" json:"ver,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *Native) Reset() {
	*x = Native{}
	mi := &file_types_ortb_V2_5_ortb_proto_msgTypes[4]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Native) String() string { return protoimpl.X.MessageStringOf(x) }

func (*Native) ProtoMessage() {}

func (x *Native) ProtoReflect() protoreflect.Message {
	mi := &file_types_ortb_V2_5_ortb_proto_msgTypes[4]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

func (*Native) Descriptor() ([]byte, []int) {
	return file_types_ortb_V2_5_ortb_proto_rawDescGZIP(), []int{4}
}

func (x *Native) GetRequest() string {
	if x != nil && x.Request != nil {
		return *x.Request
	}
	return ""
}

func (x *Native) GetVer() string {
	if x != nil && x.Ver != nil {
		return *x.Ver
	}
	return ""
}

type Imp_Ext struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Subid         *string                `protobuf:"bytes,1,opt,name=subid,proto3,oneof" json:"subid,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *Imp_Ext) Reset() {
	*x = Imp_Ext{}
	mi := &file_types_ortb_V2_5_ortb_proto_msgTypes[5]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Imp_Ext) String() string { return protoimpl.X.MessageStringOf(x) }

func (*Imp_Ext) ProtoMessage() {}

func (x *Imp_Ext) ProtoReflect() protoreflect.Message {
	mi := &file_types_ortb_V2_5_ortb_proto_msgTypes[5]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

func (*Imp_Ext) Descriptor() ([]byte, []int) {
	return file_types_ortb_V2_5_ortb_proto_rawDescGZIP(), []int{5}
}

func (x *Imp_Ext) GetSubid() string {
	if x != nil && x.Subid != nil {
		return *x.Subid
	}
	return ""
}

type Device struct {
	state          protoimpl.MessageState `protogen:"open.v1"`
	Ip             *string                `protobuf:"bytes,1,opt,name=ip,proto3,oneof" json:"ip,omitempty"`
	Geo            *Geo                   `protobuf:"bytes,2,opt,name=geo,proto3,oneof" json:"geo,omitempty"`
	Ua             *string                `protobuf:"bytes,3,opt,name=ua,proto3,oneof" json:"ua,omitempty"`
	Js             *int32                 `protobuf:"varint,4,opt,name=js,proto3,oneof" json:"js,omitempty"`
	Language       *string                `protobuf:"bytes,5,opt,name=language,proto3,oneof" json:"language,omitempty"`
	DeviceExt      *Ext                   `protobuf:"bytes,6,opt,name=device_ext,json=deviceExt,proto3,oneof" json:"device_ext,omitempty"`
	Ipv6           *string                `protobuf:"bytes,7,opt,name=ipv6,proto3,oneof" json:"ipv6,omitempty"`
	Connectiontype *int32                 `protobuf:"varint,8,opt,name=connectiontype,proto3,oneof" json:"connectiontype,omitempty"`
	Carrier        *string                `protobuf:"bytes,9,opt,name=carrier,proto3,oneof" json:"carrier,omitempty"`
	Os             *string                `protobuf:"bytes,10,opt,name=os,proto3,oneof" json:"os,omitempty"`
	DeviceType     *int32                 `protobuf:"varint,11,opt,name=deviceType,proto3,oneof" json:"deviceType,omitempty"`
	W              *int32                 `protobuf:"varint,12,opt,name=w,proto3,oneof" json:"w,omitempty"`
	H              *int32                 `protobuf:"varint,13,opt,name=h,proto3,oneof" json:"h,omitempty"`
	unknownFields  protoimpl.UnknownFields
	sizeCache      protoimpl.SizeCache
}

func (x *Device) Reset() {
	*x = Device{}
	mi := &file_types_ortb_V2_5_ortb_proto_msgTypes[6]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Device) String() string { return protoimpl.X.MessageStringOf(x) }

func (*Device) ProtoMessage() {}

func (x *Device) ProtoReflect() protoreflect.Message {
	mi := &file_types_ortb_V2_5_ortb_proto_msgTypes[6]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

func (*Device) Descriptor() ([]byte, []int) {
	return file_types_ortb_V2_5_ortb_proto_rawDescGZIP(), []int{6}
}

func (x *Device) GetIp() string {
	if x != nil && x.Ip != nil {
		return *x.Ip
	}
	return ""
}

func (x *Device) GetGeo() *Geo {
	if x != nil {
		return x.Geo
	}
	return nil
}

func (x *Device) GetUa() string {
	if x != nil && x.Ua != nil {
		return *x.Ua
	}
	return ""
}

func (x *Device) GetJs() int32 {
	if x != nil && x.Js != nil {
		return *x.Js
	}
	return 0
}

func (x *Device) GetLanguage() string {
	if x != nil && x.Language != nil {
		return *x.Language
	}
	return ""
}

func (x *Device) GetDeviceExt() *Ext {
	if x != nil {
		return x.DeviceExt
	}
	return nil
}

func (x *Device) GetIpv6() string {
	if x != nil && x.Ipv6 != nil {
		return *x.Ipv6
	}
	return ""
}

func (x *Device) GetConnectiontype() int32 {
	if x != nil && x.Connectiontype != nil {
		return *x.Connectiontype
	}
	return 0
}

func (x *Device) GetCarrier() string {
	if x != nil && x.Carrier != nil {
		return *x.Carrier
	}
	return ""
}

func (x *Device) GetOs() string {
	if x != nil && x.Os != nil {
		return *x.Os
	}
	return ""
}

func (x *Device) GetDeviceType() int32 {
	if x != nil && x.DeviceType != nil {
		return *x.DeviceType
	}
	return 0
}

func (x *Device) GetW() int32 {
	if x != nil && x.W != nil {
		return *x.W
	}
	return 0
}

func (x *Device) GetH() int32 {
	if x != nil && x.H != nil {
		return *x.H
	}
	return 0
}

type Geo struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Country       *string                `protobuf:"bytes,1,opt,name=country,proto3,oneof" json:"country,omitempty"`
	Lat           *float32               `protobuf:"fixed32,2,opt,name=lat,proto3,oneof" json:"lat,omitempty"`
	Lon           *float32               `protobuf:"fixed32,3,opt,name=lon,proto3,oneof" json:"lon,omitempty"`
	Region        *string                `protobuf:"bytes,4,opt,name=region,proto3,oneof" json:"region,omitempty"`
	City          *string                `protobuf:"bytes,5,opt,name=city,proto3,oneof" json:"city,omitempty"`
	Zip           *string                `protobuf:"bytes,6,opt,name=zip,proto3,oneof" json:"zip,omitempty"`
	Type          *int32                 `protobuf:"varint,7,opt,name=type,proto3,oneof" json:"type,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *Geo) Reset() {
	*x = Geo{}
	mi := &file_types_ortb_V2_5_ortb_proto_msgTypes[7]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Geo) String() string { return protoimpl.X.MessageStringOf(x) }

func (*Geo) ProtoMessage() {}

func (x *Geo) ProtoReflect() protoreflect.Message {
	mi := &file_types_ortb_V2_5_ortb_proto_msgTypes[7]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

func (*Geo) Descriptor() ([]byte, []int) {
	return file_types_ortb_V2_5_ortb_proto_rawDescGZIP(), []int{7}
}

func (x *Geo) GetCountry() string {
	if x != nil && x.Country != nil {
		return *x.Country
	}
	return ""
}

func (x *Geo) GetLat() float32 {
	if x != nil && x.Lat != nil {
		return *x.Lat
	}
	return 0
}

func (x *Geo) GetLon() float32 {
	if x != nil && x.Lon != nil {
		return *x.Lon
	}
	return 0
}

func (x *Geo) GetRegion() string {
	if x != nil && x.Region != nil {
		return *x.Region
	}
	return ""
}

func (x *Geo) GetCity() string {
	if x != nil && x.City != nil {
		return *x.City
	}
	return ""
}

func (x *Geo) GetZip() string {
	if x != nil && x.Zip != nil {
		return *x.Zip
	}
	return ""
}

func (x *Geo) GetType() int32 {
	if x != nil && x.Type != nil {
		return *x.Type
	}
	return 0
}

type Site struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Id            *string                `protobuf:"bytes,1,opt,name=id,proto3,oneof" json:"id,omitempty"`
	Name          *string                `protobuf:"bytes,2,opt,name=name,proto3,oneof" json:"name,omitempty"`
	Page          *string                `protobuf:"bytes,3,opt,name=page,proto3,oneof" json:"page,omitempty"`
	Domain        *string                `protobuf:"bytes,4,opt,name=domain,proto3,oneof" json:"domain,omitempty"`
	Ref           *string                `protobuf:"bytes,5,opt,name=ref,proto3,oneof" json:"ref,omitempty"`
	Cat           []string               `protobuf:"bytes,6,rep,name=cat,proto3" json:"cat,omitempty"`
	Publisher     *Publisher             `protobuf:"bytes,7,opt,name=publisher,proto3,oneof" json:"publisher,omitempty"`
	Keywords      *string                `protobuf:"bytes,8,opt,name=keywords,proto3,oneof" json:"keywords,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *Site) Reset() {
	*x = Site{}
	mi := &file_types_ortb_V2_5_ortb_proto_msgTypes[8]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Site) String() string { return protoimpl.X.MessageStringOf(x) }

func (*Site) ProtoMessage() {}

func (x *Site) ProtoReflect() protoreflect.Message {
	mi := &file_types_ortb_V2_5_ortb_proto_msgTypes[8]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

func (*Site) Descriptor() ([]byte, []int) {
	return file_types_ortb_V2_5_ortb_proto_rawDescGZIP(), []int{8}
}

func (x *Site) GetId() string {
	if x != nil && x.Id != nil {
		return *x.Id
	}
	return ""
}

func (x *Site) GetName() string {
	if x != nil && x.Name != nil {
		return *x.Name
	}
	return ""
}

func (x *Site) GetPage() string {
	if x != nil && x.Page != nil {
		return *x.Page
	}
	return ""
}

func (x *Site) GetDomain() string {
	if x != nil && x.Domain != nil {
		return *x.Domain
	}
	return ""
}

func (x *Site) GetRef() string {
	if x != nil && x.Ref != nil {
		return *x.Ref
	}
	return ""
}

func (x *Site) GetCat() []string {
	if x != nil {
		return x.Cat
	}
	return nil
}

func (x *Site) GetPublisher() *Publisher {
	if x != nil {
		return x.Publisher
	}
	return nil
}

func (x *Site) GetKeywords() string {
	if x != nil && x.Keywords != nil {
		return *x.Keywords
	}
	return ""
}

type Publisher struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Id            *string                `protobuf:"bytes,1,opt,name=id,proto3,oneof" json:"id,omitempty"`
	Name          *string                `protobuf:"bytes,2,opt,name=name,proto3,oneof" json:"name,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *Publisher) Reset() {
	*x = Publisher{}
	mi := &file_types_ortb_V2_5_ortb_proto_msgTypes[9]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Publisher) String() string { return protoimpl.X.MessageStringOf(x) }

func (*Publisher) ProtoMessage() {}

func (x *Publisher) ProtoReflect() protoreflect.Message {
	mi := &file_types_ortb_V2_5_ortb_proto_msgTypes[9]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

func (*Publisher) Descriptor() ([]byte, []int) {
	return file_types_ortb_V2_5_ortb_proto_rawDescGZIP(), []int{9}
}

func (x *Publisher) GetId() string {
	if x != nil && x.Id != nil {
		return *x.Id
	}
	return ""
}

func (x *Publisher) GetName() string {
	if x != nil && x.Name != nil {
		return *x.Name
	}
	return ""
}

type User struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Id            *string                `protobuf:"bytes,1,opt,name=id,proto3,oneof" json:"id,omitempty"`
	Keywords      *string                `protobuf:"bytes,2,opt,name=keywords,proto3,oneof" json:"keywords,omitempty"`
	Buyeruid      *string                `protobuf:"bytes,3,opt,name=buyeruid,proto3,oneof" json:"buyeruid,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *User) Reset() {
	*x = User{}
	mi := &file_types_ortb_V2_5_ortb_proto_msgTypes[10]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *User) String() string { return protoimpl.X.MessageStringOf(x) }

func (*User) ProtoMessage() {}

func (x *User) ProtoReflect() protoreflect.Message {
	mi := &file_types_ortb_V2_5_ortb_proto_msgTypes[10]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

func (*User) Descriptor() ([]byte, []int) {
	return file_types_ortb_V2_5_ortb_proto_rawDescGZIP(), []int{10}
}

func (x *User) GetId() string {
	if x != nil && x.Id != nil {
		return *x.Id
	}
	return ""
}

func (x *User) GetKeywords() string {
	if x != nil && x.Keywords != nil {
		return *x.Keywords
	}
	return ""
}

func (x *User) GetBuyeruid() string {
	if x != nil && x.Buyeruid != nil {
		return *x.Buyeruid
	}
	return ""
}

type SeatBid struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Bid           []*Bid                 `protobuf:"bytes,1,rep,name=bid,proto3" json:"bid,omitempty"`
	Seat          *string                `protobuf:"bytes,2,opt,name=seat,proto3,oneof" json:"seat,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *SeatBid) Reset() {
	*x = SeatBid{}
	mi := &file_types_ortb_V2_5_ortb_proto_msgTypes[11]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *SeatBid) String() string { return protoimpl.X.MessageStringOf(x) }

func (*SeatBid) ProtoMessage() {}

func (x *SeatBid) ProtoReflect() protoreflect.Message {
	mi := &file_types_ortb_V2_5_ortb_proto_msgTypes[11]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

func (*SeatBid) Descriptor() ([]byte, []int) {
	return file_types_ortb_V2_5_ortb_proto_rawDescGZIP(), []int{11}
}

func (x *SeatBid) GetBid() []*Bid {
	if x != nil {
		return x.Bid
	}
	return nil
}

func (x *SeatBid) GetSeat() string {
	if x != nil && x.Seat != nil {
		return *x.Seat
	}
	return ""
}

type Bid struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Id            *string                `protobuf:"bytes,1,opt,name=id,proto3,oneof" json:"id,omitempty"`
	Impid         *string                `protobuf:"bytes,2,opt,name=impid,proto3,oneof" json:"impid,omitempty"`
	Price         *float32               `protobuf:"fixed32,3,opt,name=price,proto3,oneof" json:"price,omitempty"`
	Adid          *string                `protobuf:"bytes,4,opt,name=adid,proto3,oneof" json:"adid,omitempty"`
	Nurl          *string                `protobuf:"bytes,5,opt,name=nurl,proto3,oneof" json:"nurl,omitempty"`
	Burl          *string                `protobuf:"bytes,6,opt,name=burl,proto3,oneof" json:"burl,omitempty"`
	Adm           *string                `protobuf:"bytes,7,opt,name=adm,proto3,oneof" json:"adm,omitempty"`
	Adomain       []string               `protobuf:"bytes,8,rep,name=adomain,proto3" json:"adomain,omitempty"`
	Bundle        *string                `protobuf:"bytes,9,opt,name=bundle,proto3,oneof" json:"bundle,omitempty"`
	Iurl          *string                `protobuf:"bytes,10,opt,name=iurl,proto3,oneof" json:"iurl,omitempty"`
	Cid           *string                `protobuf:"bytes,11,opt,name=cid,proto3,oneof" json:"cid,omitempty"`
	Crid          *string                `protobuf:"bytes,12,opt,name=crid,proto3,oneof" json:"crid,omitempty"`
	Attr          []int32                `protobuf:"varint,13,rep,name=attr,proto3" json:"attr,omitempty"`
	Dealid        *string                `protobuf:"bytes,14,opt,name=dealid,proto3,oneof" json:"dealid,omitempty"`
	W             *int32                 `protobuf:"varint,15,opt,name=w,proto3,oneof" json:"w,omitempty"`
	H             *int32                 `protobuf:"varint,16,opt,name=h,proto3,oneof" json:"h,omitempty"`
	Ext           *BidExt                `protobuf:"bytes,17,opt,name=ext,proto3,oneof" json:"ext,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *Bid) Reset() {
	*x = Bid{}
	mi := &file_types_ortb_V2_5_ortb_proto_msgTypes[12]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Bid) String() string { return protoimpl.X.MessageStringOf(x) }

func (*Bid) ProtoMessage() {}

func (x *Bid) ProtoReflect() protoreflect.Message {
	mi := &file_types_ortb_V2_5_ortb_proto_msgTypes[12]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

func (*Bid) Descriptor() ([]byte, []int) {
	return file_types_ortb_V2_5_ortb_proto_rawDescGZIP(), []int{12}
}

func (x *Bid) GetId() string {
	if x != nil && x.Id != nil {
		return *x.Id
	}
	return ""
}

func (x *Bid) GetImpid() string {
	if x != nil && x.Impid != nil {
		return *x.Impid
	}
	return ""
}

func (x *Bid) GetPrice() float32 {
	if x != nil && x.Price != nil {
		return *x.Price
	}
	return 0
}

func (x *Bid) GetAdid() string {
	if x != nil && x.Adid != nil {
		return *x.Adid
	}
	return ""
}

func (x *Bid) GetNurl() string {
	if x != nil && x.Nurl != nil {
		return *x.Nurl
	}
	return ""
}

func (x *Bid) GetBurl() string {
	if x != nil && x.Burl != nil {
		return *x.Burl
	}
	return ""
}

func (x *Bid) GetAdm() string {
	if x != nil && x.Adm != nil {
		return *x.Adm
	}
	return ""
}

func (x *Bid) GetAdomain() []string {
	if x != nil {
		return x.Adomain
	}
	return nil
}

func (x *Bid) GetBundle() string {
	if x != nil && x.Bundle != nil {
		return *x.Bundle
	}
	return ""
}

func (x *Bid) GetIurl() string {
	if x != nil && x.Iurl != nil {
		return *x.Iurl
	}
	return ""
}

func (x *Bid) GetCid() string {
	if x != nil && x.Cid != nil {
		return *x.Cid
	}
	return ""
}

func (x *Bid) GetCrid() string {
	if x != nil && x.Crid != nil {
		return *x.Crid
	}
	return ""
}

func (x *Bid) GetAttr() []int32 {
	if x != nil {
		return x.Attr
	}
	return nil
}

func (x *Bid) GetDealid() string {
	if x != nil && x.Dealid != nil {
		return *x.Dealid
	}
	return ""
}

func (x *Bid) GetW() int32 {
	if x != nil && x.W != nil {
		return *x.W
	}
	return 0
}

func (x *Bid) GetH() int32 {
	if x != nil && x.H != nil {
		return *x.H
	}
	return 0
}

func (x *Bid) GetExt() *BidExt {
	if x != nil {
		return x.Ext
	}
	return nil
}

type BidExt struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Btype         *int32                 `protobuf:"varint,1,opt,name=btype,proto3,oneof" json:"btype,omitempty"`
	VerticalId    *int32                 `protobuf:"varint,2,opt,name=vertical_id,json=verticalId,proto3,oneof" json:"vertical_id,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *BidExt) Reset() {
	*x = BidExt{}
	mi := &file_types_ortb_V2_5_ortb_proto_msgTypes[13]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *BidExt) String() string { return protoimpl.X.MessageStringOf(x) }

func (*BidExt) ProtoMessage() {}

func (x *BidExt) ProtoReflect() protoreflect.Message {
	mi := &file_types_ortb_V2_5_ortb_proto_msgTypes[13]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

func (*BidExt) Descriptor() ([]byte, []int) {
	return file_types_ortb_V2_5_ortb_proto_rawDescGZIP(), []int{13}
}

func (x *BidExt) GetBtype() int32 {
	if x != nil && x.Btype != nil {
		return *x.Btype
	}
	return 0
}

func (x *BidExt) GetVerticalId() int32 {
	if x != nil && x.VerticalId != nil {
		return *x.VerticalId
	}
	return 0
}

type BidResponse struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Id            *string                `protobuf:"bytes,1,opt,name=id,proto3,oneof" json:"id,omitempty"`
	Seatbid       []*SeatBid             `protobuf:"bytes,2,rep,name=seatbid,proto3" json:"seatbid,omitempty"`
	Bidid         *string                `protobuf:"bytes,3,opt,name=bidid,proto3,oneof" json:"bidid,omitempty"`
	Cur           *string                `protobuf:"bytes,4,opt,name=cur,proto3,oneof" json:"cur,omitempty"`
	Nbr           *int32                 `protobuf:"varint,5,opt,name=nbr,proto3,oneof" json:"nbr,omitempty"`
	Ext           *Ext                   `protobuf:"bytes,6,opt,name=ext,proto3,oneof" json:"ext,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *BidResponse) Reset() {
	*x = BidResponse{}
	mi := &file_types_ortb_V2_5_ortb_proto_msgTypes[14]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *BidResponse) String() string { return protoimpl.X.MessageStringOf(x) }

func (*BidResponse) ProtoMessage() {}

func (x *BidResponse) ProtoReflect() protoreflect.Message {
	mi := &file_types_ortb_V2_5_ortb_proto_msgTypes[14]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

func (*BidResponse) Descriptor() ([]byte, []int) {
	return file_types_ortb_V2_5_ortb_proto_rawDescGZIP(), []int{14}
}

func (x *BidResponse) GetId() string {
	if x != nil && x.Id != nil {
		return *x.Id
	}
	return ""
}

func (x *BidResponse) GetSeatbid() []*SeatBid {
	if x != nil {
		return x.Seatbid
	}
	return nil
}

func (x *BidResponse) GetBidid() string {
	if x != nil && x.Bidid != nil {
		return *x.Bidid
	}
	return ""
}

func (x *BidResponse) GetCur() string {
	if x != nil && x.Cur != nil {
		return *x.Cur
	}
	return ""
}

func (x *BidResponse) GetNbr() int32 {
	if x != nil && x.Nbr != nil {
		return *x.Nbr
	}
	return 0
}

func (x *BidResponse) GetExt() *Ext {
	if x != nil {
		return x.Ext
	}
	return nil
}

type Ext struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Values        map[string]string      `protobuf:"bytes,1,rep,name=values,proto3" json:"values,omitempty" protobuf_key:"bytes,1,opt,name=key" protobuf_val:"bytes,2,opt,name=value"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *Ext) Reset() {
	*x = Ext{}
	mi := &file_types_ortb_V2_5_ortb_proto_msgTypes[15]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Ext) String() string { return protoimpl.X.MessageStringOf(x) }

func (*Ext) ProtoMessage() {}

func (x *Ext) ProtoReflect() protoreflect.Message {
	mi := &file_types_ortb_V2_5_ortb_proto_msgTypes[15]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

func (*Ext) Descriptor() ([]byte, []int) {
	return file_types_ortb_V2_5_ortb_proto_rawDescGZIP(), []int{15}
}

func (x *Ext) GetValues() map[string]string {
	if x != nil {
		return x.Values
	}
	return nil
}

var File_types_ortb_V2_5_ortb_proto protoreflect.FileDescriptor

const file_types_ortb_V2_5_ortb_proto_rawDesc = "\x0a\x1a\x74\x79\x70\x65\x73\x2f\x6f\x72\x74\x62\x5f\x56\x32\x5f\x35\x2f\x6f\x72\x74\x62\x2e\x70" +
	"\x72\x6f\x74\x6f\x12\x09\x6f\x72\x74\x62\x5f\x56\x32\x5f\x35\x22\xf1\x02\x0a\x0a\x42\x69\x64\x52" +
	"\x65\x71\x75\x65\x73\x74\x12\x13\x0a\x02\x69\x64\x18\x01\x20\x01\x28\x09\x48\x00\x52\x02\x69\x64" +
	"\x88\x01\x01\x12\x13\x0a\x02\x61\x74\x18\x02\x20\x01\x28\x05\x48\x01\x52\x02\x61\x74\x88\x01\x01" +
	"\x12\x20\x0a\x03\x69\x6d\x70\x18\x03\x20\x03\x28\x0b\x32\x0e\x2e\x6f\x72\x74\x62\x5f\x56\x32\x5f" +
	"\x35\x2e\x49\x6d\x70\x52\x03\x69\x6d\x70\x12\x2e\x0a\x06\x64\x65\x76\x69\x63\x65\x18\x04\x20\x01" +
	"\x28\x0b\x32\x11\x2e\x6f\x72\x74\x62\x5f\x56\x32\x5f\x35\x2e\x44\x65\x76\x69\x63\x65\x48\x02\x52" +
	"\x06\x64\x65\x76\x69\x63\x65\x88\x01\x01\x12\x28\x0a\x04\x73\x69\x74\x65\x18\x05\x20\x01\x28\x0b" +
	"\x32\x0f\x2e\x6f\x72\x74\x62\x5f\x56\x32\x5f\x35\x2e\x53\x69\x74\x65\x48\x03\x52\x04\x73\x69\x74" +
	"\x65\x88\x01\x01\x12\x28\x0a\x04\x75\x73\x65\x72\x18\x06\x20\x01\x28\x0b\x32\x0f\x2e\x6f\x72\x74" +
	"\x62\x5f\x56\x32\x5f\x35\x2e\x55\x73\x65\x72\x48\x04\x52\x04\x75\x73\x65\x72\x88\x01\x01\x12\x17" +
	"\x0a\x04\x74\x6d\x61\x78\x18\x07\x20\x01\x28\x05\x48\x05\x52\x04\x74\x6d\x61\x78\x88\x01\x01\x12" +
	"\x10\x0a\x03\x63\x75\x72\x18\x08\x20\x03\x28\x09\x52\x03\x63\x75\x72\x12\x12\x0a\x04\x62\x63\x61" +
	"\x74\x18\x09\x20\x03\x28\x09\x52\x04\x62\x63\x61\x74\x12\x17\x0a\x04\x74\x65\x73\x74\x18\x0a\x20" +
	"\x01\x28\x05\x48\x06\x52\x04\x74\x65\x73\x74\x88\x01\x01\x42\x05\x0a\x03\x5f\x69\x64\x42\x05\x0a" +
	"\x03\x5f\x61\x74\x42\x09\x0a\x07\x5f\x64\x65\x76\x69\x63\x65\x42\x07\x0a\x05\x5f\x73\x69\x74\x65" +
	"\x42\x07\x0a\x05\x5f\x75\x73\x65\x72\x42\x07\x0a\x05\x5f\x74\x6d\x61\x78\x42\x07\x0a\x05\x5f\x74" +
	"\x65\x73\x74\x22\xa1\x03\x0a\x03\x49\x6d\x70\x12\x13\x0a\x02\x69\x64\x18\x01\x20\x01\x28\x09\x48" +
	"\x00\x52\x02\x69\x64\x88\x01\x01\x12\x1f\x0a\x08\x62\x69\x64\x66\x6c\x6f\x6f\x72\x18\x02\x20\x01" +
	"\x28\x02\x48\x01\x52\x08\x62\x69\x64\x66\x6c\x6f\x6f\x72\x88\x01\x01\x12\x19\x0a\x05\x74\x61\x67" +
	"\x69\x64\x18\x03\x20\x01\x28\x09\x48\x02\x52\x05\x74\x61\x67\x69\x64\x88\x01\x01\x12\x1b\x0a\x06" +
	"\x73\x65\x63\x75\x72\x65\x18\x04\x20\x01\x28\x05\x48\x03\x52\x06\x73\x65\x63\x75\x72\x65\x88\x01" +
	"\x01\x12\x19\x0a\x05\x69\x6e\x73\x74\x6c\x18\x05\x20\x01\x28\x05\x48\x04\x52\x05\x69\x6e\x73\x74" +
	"\x6c\x88\x01\x01\x12\x25\x0a\x0b\x62\x69\x64\x66\x6c\x6f\x6f\x72\x63\x75\x72\x18\x06\x20\x01\x28" +
	"\x09\x48\x05\x52\x0b\x62\x69\x64\x66\x6c\x6f\x6f\x72\x63\x75\x72\x88\x01\x01\x12\x29\x0a\x03\x65" +
	"\x78\x74\x18\x07\x20\x01\x28\x0b\x32\x12\x2e\x6f\x72\x74\x62\x5f\x56\x32\x5f\x35\x2e\x49\x6d\x70" +
	"\x5f\x45\x78\x74\x48\x06\x52\x03\x65\x78\x74\x88\x01\x01\x12\x2e\x0a\x06\x62\x61\x6e\x6e\x65\x72" +
	"\x18\x08\x20\x01\x28\x0b\x32\x11\x2e\x6f\x72\x74\x62\x5f\x56\x32\x5f\x35\x2e\x42\x61\x6e\x6e\x65" +
	"\x72\x48\x07\x52\x06\x62\x61\x6e\x6e\x65\x72\x88\x01\x01\x12\x2e\x0a\x06\x6e\x61\x74\x69\x76\x65" +
	"\x18\x09\x20\x01\x28\x0b\x32\x11\x2e\x6f\x72\x74\x62\x5f\x56\x32\x5f\x35\x2e\x4e\x61\x74\x69\x76" +
	"\x65\x48\x08\x52\x06\x6e\x61\x74\x69\x76\x65\x88\x01\x01\x42\x05\x0a\x03\x5f\x69\x64\x42\x0b\x0a" +
	"\x09\x5f\x62\x69\x64\x66\x6c\x6f\x6f\x72\x42\x08\x0a\x06\x5f\x74\x61\x67\x69\x64\x42\x09\x0a\x07" +
	"\x5f\x73\x65\x63\x75\x72\x65\x42\x08\x0a\x06\x5f\x69\x6e\x73\x74\x6c\x42\x0e\x0a\x0c\x5f\x62\x69" +
	"\x64\x66\x6c\x6f\x6f\x72\x63\x75\x72\x42\x06\x0a\x04\x5f\x65\x78\x74\x42\x09\x0a\x07\x5f\x62\x61" +
	"\x6e\x6e\x65\x72\x42\x09\x0a\x07\x5f\x6e\x61\x74\x69\x76\x65\x22\xa5\x02\x0a\x06\x42\x61\x6e\x6e" +
	"\x65\x72\x12\x11\x0a\x01\x77\x18\x01\x20\x01\x28\x05\x48\x00\x52\x01\x77\x88\x01\x01\x12\x11\x0a" +
	"\x01\x68\x18\x02\x20\x01\x28\x05\x48\x01\x52\x01\x68\x88\x01\x01\x12\x15\x0a\x03\x70\x6f\x73\x18" +
	"\x03\x20\x01\x28\x05\x48\x02\x52\x03\x70\x6f\x73\x88\x01\x01\x12\x14\x0a\x05\x6d\x69\x6d\x65\x73" +
	"\x18\x04\x20\x03\x28\x09\x52\x05\x6d\x69\x6d\x65\x73\x12\x15\x0a\x03\x61\x70\x69\x18\x05\x20\x01" +
	"\x28\x09\x48\x03\x52\x03\x61\x70\x69\x88\x01\x01\x12\x1f\x0a\x08\x74\x6f\x70\x66\x72\x61\x6d\x65" +
	"\x18\x06\x20\x01\x28\x05\x48\x04\x52\x08\x74\x6f\x70\x66\x72\x61\x6d\x65\x88\x01\x01\x12\x14\x0a" +
	"\x05\x62\x74\x79\x70\x65\x18\x07\x20\x03\x28\x05\x52\x05\x62\x74\x79\x70\x65\x12\x14\x0a\x05\x62" +
	"\x61\x74\x74\x72\x18\x08\x20\x03\x28\x05\x52\x05\x62\x61\x74\x74\x72\x12\x10\x0a\x03\x65\x78\x74" +
	"\x18\x09\x20\x03\x28\x09\x52\x03\x65\x78\x74\x12\x29\x0a\x06\x66\x6f\x72\x6d\x61\x74\x18\x0a\x20" +
	"\x03\x28\x0b\x32\x11\x2e\x6f\x72\x74\x62\x5f\x56\x32\x5f\x35\x2e\x46\x6f\x72\x6d\x61\x74\x52\x06" +
	"\x66\x6f\x72\x6d\x61\x74\x42\x04\x0a\x02\x5f\x77\x42\x04\x0a\x02\x5f\x68\x42\x06\x0a\x04\x5f\x70" +
	"\x6f\x73\x42\x06\x0a\x04\x5f\x61\x70\x69\x42\x0b\x0a\x09\x5f\x74\x6f\x70\x66\x72\x61\x6d\x65\x22" +
	"\x3a\x0a\x06\x46\x6f\x72\x6d\x61\x74\x12\x11\x0a\x01\x77\x18\x01\x20\x01\x28\x05\x48\x00\x52\x01" +
	"\x77\x88\x01\x01\x12\x11\x0a\x01\x68\x18\x02\x20\x01\x28\x05\x48\x01\x52\x01\x68\x88\x01\x01\x42" +
	"\x04\x0a\x02\x5f\x77\x42\x04\x0a\x02\x5f\x68\x22\x52\x0a\x06\x4e\x61\x74\x69\x76\x65\x12\x1d\x0a" +
	"\x07\x72\x65\x71\x75\x65\x73\x74\x18\x01\x20\x01\x28\x09\x48\x00\x52\x07\x72\x65\x71\x75\x65\x73" +
	"\x74\x88\x01\x01\x12\x15\x0a\x03\x76\x65\x72\x18\x02\x20\x01\x28\x09\x48\x01\x52\x03\x76\x65\x72" +
	"\x88\x01\x01\x42\x0a\x0a\x08\x5f\x72\x65\x71\x75\x65\x73\x74\x42\x06\x0a\x04\x5f\x76\x65\x72\x22" +
	"\x2e\x0a\x07\x49\x6d\x70\x5f\x45\x78\x74\x12\x19\x0a\x05\x73\x75\x62\x69\x64\x18\x01\x20\x01\x28" +
	"\x09\x48\x00\x52\x05\x73\x75\x62\x69\x64\x88\x01\x01\x42\x08\x0a\x06\x5f\x73\x75\x62\x69\x64\x22" +
	"\x8b\x04\x0a\x06\x44\x65\x76\x69\x63\x65\x12\x13\x0a\x02\x69\x70\x18\x01\x20\x01\x28\x09\x48\x00" +
	"\x52\x02\x69\x70\x88\x01\x01\x12\x25\x0a\x03\x67\x65\x6f\x18\x02\x20\x01\x28\x0b\x32\x0e\x2e\x6f" +
	"\x72\x74\x62\x5f\x56\x32\x5f\x35\x2e\x47\x65\x6f\x48\x01\x52\x03\x67\x65\x6f\x88\x01\x01\x12\x13" +
	"\x0a\x02\x75\x61\x18\x03\x20\x01\x28\x09\x48\x02\x52\x02\x75\x61\x88\x01\x01\x12\x13\x0a\x02\x6a" +
	"\x73\x18\x04\x20\x01\x28\x05\x48\x03\x52\x02\x6a\x73\x88\x01\x01\x12\x1f\x0a\x08\x6c\x61\x6e\x67" +
	"\x75\x61\x67\x65\x18\x05\x20\x01\x28\x09\x48\x04\x52\x08\x6c\x61\x6e\x67\x75\x61\x67\x65\x88\x01" +
	"\x01\x12\x32\x0a\x0a\x64\x65\x76\x69\x63\x65\x5f\x65\x78\x74\x18\x06\x20\x01\x28\x0b\x32\x0e\x2e" +
	"\x6f\x72\x74\x62\x5f\x56\x32\x5f\x35\x2e\x45\x78\x74\x48\x05\x52\x09\x64\x65\x76\x69\x63\x65\x45" +
	"\x78\x74\x88\x01\x01\x12\x17\x0a\x04\x69\x70\x76\x36\x18\x07\x20\x01\x28\x09\x48\x06\x52\x04\x69" +
	"\x70\x76\x36\x88\x01\x01\x12\x2b\x0a\x0e\x63\x6f\x6e\x6e\x65\x63\x74\x69\x6f\x6e\x74\x79\x70\x65" +
	"\x18\x08\x20\x01\x28\x05\x48\x07\x52\x0e\x63\x6f\x6e\x6e\x65\x63\x74\x69\x6f\x6e\x74\x79\x70\x65" +
	"\x88\x01\x01\x12\x1d\x0a\x07\x63\x61\x72\x72\x69\x65\x72\x18\x09\x20\x01\x28\x09\x48\x08\x52\x07" +
	"\x63\x61\x72\x72\x69\x65\x72\x88\x01\x01\x12\x13\x0a\x02\x6f\x73\x18\x0a\x20\x01\x28\x09\x48\x09" +
	"\x52\x02\x6f\x73\x88\x01\x01\x12\x23\x0a\x0a\x64\x65\x76\x69\x63\x65\x54\x79\x70\x65\x18\x0b\x20" +
	"\x01\x28\x05\x48\x0a\x52\x0a\x64\x65\x76\x69\x63\x65\x54\x79\x70\x65\x88\x01\x01\x12\x11\x0a\x01" +
	"\x77\x18\x0c\x20\x01\x28\x05\x48\x0b\x52\x01\x77\x88\x01\x01\x12\x11\x0a\x01\x68\x18\x0d\x20\x01" +
	"\x28\x05\x48\x0c\x52\x01\x68\x88\x01\x01\x42\x05\x0a\x03\x5f\x69\x70\x42\x06\x0a\x04\x5f\x67\x65" +
	"\x6f\x42\x05\x0a\x03\x5f\x75\x61\x42\x05\x0a\x03\x5f\x6a\x73\x42\x0b\x0a\x09\x5f\x6c\x61\x6e\x67" +
	"\x75\x61\x67\x65\x42\x0d\x0a\x0b\x5f\x64\x65\x76\x69\x63\x65\x5f\x65\x78\x74\x42\x07\x0a\x05\x5f" +
	"\x69\x70\x76\x36\x42\x11\x0a\x0f\x5f\x63\x6f\x6e\x6e\x65\x63\x74\x69\x6f\x6e\x74\x79\x70\x65\x42" +
	"\x0a\x0a\x08\x5f\x63\x61\x72\x72\x69\x65\x72\x42\x05\x0a\x03\x5f\x6f\x73\x42\x0d\x0a\x0b\x5f\x64" +
	"\x65\x76\x69\x63\x65\x54\x79\x70\x65\x42\x04\x0a\x02\x5f\x77\x42\x04\x0a\x02\x5f\x68\x22\xf9\x01" +
	"\x0a\x03\x47\x65\x6f\x12\x1d\x0a\x07\x63\x6f\x75\x6e\x74\x72\x79\x18\x01\x20\x01\x28\x09\x48\x00" +
	"\x52\x07\x63\x6f\x75\x6e\x74\x72\x79\x88\x01\x01\x12\x15\x0a\x03\x6c\x61\x74\x18\x02\x20\x01\x28" +
	"\x02\x48\x01\x52\x03\x6c\x61\x74\x88\x01\x01\x12\x15\x0a\x03\x6c\x6f\x6e\x18\x03\x20\x01\x28\x02" +
	"\x48\x02\x52\x03\x6c\x6f\x6e\x88\x01\x01\x12\x1b\x0a\x06\x72\x65\x67\x69\x6f\x6e\x18\x04\x20\x01" +
	"\x28\x09\x48\x03\x52\x06\x72\x65\x67\x69\x6f\x6e\x88\x01\x01\x12\x17\x0a\x04\x63\x69\x74\x79\x18" +
	"\x05\x20\x01\x28\x09\x48\x04\x52\x04\x63\x69\x74\x79\x88\x01\x01\x12\x15\x0a\x03\x7a\x69\x70\x18" +
	"\x06\x20\x01\x28\x09\x48\x05\x52\x03\x7a\x69\x70\x88\x01\x01\x12\x17\x0a\x04\x74\x79\x70\x65\x18" +
	"\x07\x20\x01\x28\x05\x48\x06\x52\x04\x74\x79\x70\x65\x88\x01\x01\x42\x0a\x0a\x08\x5f\x63\x6f\x75" +
	"\x6e\x74\x72\x79\x42\x06\x0a\x04\x5f\x6c\x61\x74\x42\x06\x0a\x04\x5f\x6c\x6f\x6e\x42\x09\x0a\x07" +
	"\x5f\x72\x65\x67\x69\x6f\x6e\x42\x07\x0a\x05\x5f\x63\x69\x74\x79\x42\x06\x0a\x04\x5f\x7a\x69\x70" +
	"\x42\x07\x0a\x05\x5f\x74\x79\x70\x65\x22\xb4\x02\x0a\x04\x53\x69\x74\x65\x12\x13\x0a\x02\x69\x64" +
	"\x18\x01\x20\x01\x28\x09\x48\x00\x52\x02\x69\x64\x88\x01\x01\x12\x17\x0a\x04\x6e\x61\x6d\x65\x18" +
	"\x02\x20\x01\x28\x09\x48\x01\x52\x04\x6e\x61\x6d\x65\x88\x01\x01\x12\x17\x0a\x04\x70\x61\x67\x65" +
	"\x18\x03\x20\x01\x28\x09\x48\x02\x52\x04\x70\x61\x67\x65\x88\x01\x01\x12\x1b\x0a\x06\x64\x6f\x6d" +
	"\x61\x69\x6e\x18\x04\x20\x01\x28\x09\x48\x03\x52\x06\x64\x6f\x6d\x61\x69\x6e\x88\x01\x01\x12\x15" +
	"\x0a\x03\x72\x65\x66\x18\x05\x20\x01\x28\x09\x48\x04\x52\x03\x72\x65\x66\x88\x01\x01\x12\x10\x0a" +
	"\x03\x63\x61\x74\x18\x06\x20\x03\x28\x09\x52\x03\x63\x61\x74\x12\x37\x0a\x09\x70\x75\x62\x6c\x69" +
	"\x73\x68\x65\x72\x18\x07\x20\x01\x28\x0b\x32\x14\x2e\x6f\x72\x74\x62\x5f\x56\x32\x5f\x35\x2e\x50" +
	"\x75\x62\x6c\x69\x73\x68\x65\x72\x48\x05\x52\x09\x70\x75\x62\x6c\x69\x73\x68\x65\x72\x88\x01\x01" +
	"\x12\x1f\x0a\x08\x6b\x65\x79\x77\x6f\x72\x64\x73\x18\x08\x20\x01\x28\x09\x48\x06\x52\x08\x6b\x65" +
	"\x79\x77\x6f\x72\x64\x73\x88\x01\x01\x42\x05\x0a\x03\x5f\x69\x64\x42\x07\x0a\x05\x5f\x6e\x61\x6d" +
	"\x65\x42\x07\x0a\x05\x5f\x70\x61\x67\x65\x42\x09\x0a\x07\x5f\x64\x6f\x6d\x61\x69\x6e\x42\x06\x0a" +
	"\x04\x5f\x72\x65\x66\x42\x0c\x0a\x0a\x5f\x70\x75\x62\x6c\x69\x73\x68\x65\x72\x42\x0b\x0a\x09\x5f" +
	"\x6b\x65\x79\x77\x6f\x72\x64\x73\x22\x49\x0a\x09\x50\x75\x62\x6c\x69\x73\x68\x65\x72\x12\x13\x0a" +
	"\x02\x69\x64\x18\x01\x20\x01\x28\x09\x48\x00\x52\x02\x69\x64\x88\x01\x01\x12\x17\x0a\x04\x6e\x61" +
	"\x6d\x65\x18\x02\x20\x01\x28\x09\x48\x01\x52\x04\x6e\x61\x6d\x65\x88\x01\x01\x42\x05\x0a\x03\x5f" +
	"\x69\x64\x42\x07\x0a\x05\x5f\x6e\x61\x6d\x65\x22\x7e\x0a\x04\x55\x73\x65\x72\x12\x13\x0a\x02\x69" +
	"\x64\x18\x01\x20\x01\x28\x09\x48\x00\x52\x02\x69\x64\x88\x01\x01\x12\x1f\x0a\x08\x6b\x65\x79\x77" +
	"\x6f\x72\x64\x73\x18\x02\x20\x01\x28\x09\x48\x01\x52\x08\x6b\x65\x79\x77\x6f\x72\x64\x73\x88\x01" +
	"\x01\x12\x1f\x0a\x08\x62\x75\x79\x65\x72\x75\x69\x64\x18\x03\x20\x01\x28\x09\x48\x02\x52\x08\x62" +
	"\x75\x79\x65\x72\x75\x69\x64\x88\x01\x01\x42\x05\x0a\x03\x5f\x69\x64\x42\x0b\x0a\x09\x5f\x6b\x65" +
	"\x79\x77\x6f\x72\x64\x73\x42\x0b\x0a\x09\x5f\x62\x75\x79\x65\x72\x75\x69\x64\x22\x4d\x0a\x07\x53" +
	"\x65\x61\x74\x42\x69\x64\x12\x20\x0a\x03\x62\x69\x64\x18\x01\x20\x03\x28\x0b\x32\x0e\x2e\x6f\x72" +
	"\x74\x62\x5f\x56\x32\x5f\x35\x2e\x42\x69\x64\x52\x03\x62\x69\x64\x12\x17\x0a\x04\x73\x65\x61\x74" +
	"\x18\x02\x20\x01\x28\x09\x48\x00\x52\x04\x73\x65\x61\x74\x88\x01\x01\x42\x07\x0a\x05\x5f\x73\x65" +
	"\x61\x74\x22\xb5\x04\x0a\x03\x42\x69\x64\x12\x13\x0a\x02\x69\x64\x18\x01\x20\x01\x28\x09\x48\x00" +
	"\x52\x02\x69\x64\x88\x01\x01\x12\x19\x0a\x05\x69\x6d\x70\x69\x64\x18\x02\x20\x01\x28\x09\x48\x01" +
	"\x52\x05\x69\x6d\x70\x69\x64\x88\x01\x01\x12\x19\x0a\x05\x70\x72\x69\x63\x65\x18\x03\x20\x01\x28" +
	"\x02\x48\x02\x52\x05\x70\x72\x69\x63\x65\x88\x01\x01\x12\x17\x0a\x04\x61\x64\x69\x64\x18\x04\x20" +
	"\x01\x28\x09\x48\x03\x52\x04\x61\x64\x69\x64\x88\x01\x01\x12\x17\x0a\x04\x6e\x75\x72\x6c\x18\x05" +
	"\x20\x01\x28\x09\x48\x04\x52\x04\x6e\x75\x72\x6c\x88\x01\x01\x12\x17\x0a\x04\x62\x75\x72\x6c\x18" +
	"\x06\x20\x01\x28\x09\x48\x05\x52\x04\x62\x75\x72\x6c\x88\x01\x01\x12\x15\x0a\x03\x61\x64\x6d\x18" +
	"\x07\x20\x01\x28\x09\x48\x06\x52\x03\x61\x64\x6d\x88\x01\x01\x12\x18\x0a\x07\x61\x64\x6f\x6d\x61" +
	"\x69\x6e\x18\x08\x20\x03\x28\x09\x52\x07\x61\x64\x6f\x6d\x61\x69\x6e\x12\x1b\x0a\x06\x62\x75\x6e" +
	"\x64\x6c\x65\x18\x09\x20\x01\x28\x09\x48\x07\x52\x06\x62\x75\x6e\x64\x6c\x65\x88\x01\x01\x12\x17" +
	"\x0a\x04\x69\x75\x72\x6c\x18\x0a\x20\x01\x28\x09\x48\x08\x52\x04\x69\x75\x72\x6c\x88\x01\x01\x12" +
	"\x15\x0a\x03\x63\x69\x64\x18\x0b\x20\x01\x28\x09\x48\x09\x52\x03\x63\x69\x64\x88\x01\x01\x12\x17" +
	"\x0a\x04\x63\x72\x69\x64\x18\x0c\x20\x01\x28\x09\x48\x0a\x52\x04\x63\x72\x69\x64\x88\x01\x01\x12" +
	"\x12\x0a\x04\x61\x74\x74\x72\x18\x0d\x20\x03\x28\x05\x52\x04\x61\x74\x74\x72\x12\x1b\x0a\x06\x64" +
	"\x65\x61\x6c\x69\x64\x18\x0e\x20\x01\x28\x09\x48\x0b\x52\x06\x64\x65\x61\x6c\x69\x64\x88\x01\x01" +
	"\x12\x11\x0a\x01\x77\x18\x0f\x20\x01\x28\x05\x48\x0c\x52\x01\x77\x88\x01\x01\x12\x11\x0a\x01\x68" +
	"\x18\x10\x20\x01\x28\x05\x48\x0d\x52\x01\x68\x88\x01\x01\x12\x28\x0a\x03\x65\x78\x74\x18\x11\x20" +
	"\x01\x28\x0b\x32\x11\x2e\x6f\x72\x74\x62\x5f\x56\x32\x5f\x35\x2e\x42\x69\x64\x45\x78\x74\x48\x0e" +
	"\x52\x03\x65\x78\x74\x88\x01\x01\x42\x05\x0a\x03\x5f\x69\x64\x42\x08\x0a\x06\x5f\x69\x6d\x70\x69" +
	"\x64\x42\x08\x0a\x06\x5f\x70\x72\x69\x63\x65\x42\x07\x0a\x05\x5f\x61\x64\x69\x64\x42\x07\x0a\x05" +
	"\x5f\x6e\x75\x72\x6c\x42\x07\x0a\x05\x5f\x62\x75\x72\x6c\x42\x06\x0a\x04\x5f\x61\x64\x6d\x42\x09" +
	"\x0a\x07\x5f\x62\x75\x6e\x64\x6c\x65\x42\x07\x0a\x05\x5f\x69\x75\x72\x6c\x42\x06\x0a\x04\x5f\x63" +
	"\x69\x64\x42\x07\x0a\x05\x5f\x63\x72\x69\x64\x42\x09\x0a\x07\x5f\x64\x65\x61\x6c\x69\x64\x42\x04" +
	"\x0a\x02\x5f\x77\x42\x04\x0a\x02\x5f\x68\x42\x06\x0a\x04\x5f\x65\x78\x74\x22\x63\x0a\x06\x42\x69" +
	"\x64\x45\x78\x74\x12\x19\x0a\x05\x62\x74\x79\x70\x65\x18\x01\x20\x01\x28\x05\x48\x00\x52\x05\x62" +
	"\x74\x79\x70\x65\x88\x01\x01\x12\x24\x0a\x0b\x76\x65\x72\x74\x69\x63\x61\x6c\x5f\x69\x64\x18\x02" +
	"\x20\x01\x28\x05\x48\x01\x52\x0a\x76\x65\x72\x74\x69\x63\x61\x6c\x49\x64\x88\x01\x01\x42\x08\x0a" +
	"\x06\x5f\x62\x74\x79\x70\x65\x42\x0e\x0a\x0c\x5f\x76\x65\x72\x74\x69\x63\x61\x6c\x5f\x69\x64\x22" +
	"\xe9\x01\x0a\x0b\x42\x69\x64\x52\x65\x73\x70\x6f\x6e\x73\x65\x12\x13\x0a\x02\x69\x64\x18\x01\x20" +
	"\x01\x28\x09\x48\x00\x52\x02\x69\x64\x88\x01\x01\x12\x2c\x0a\x07\x73\x65\x61\x74\x62\x69\x64\x18" +
	"\x02\x20\x03\x28\x0b\x32\x12\x2e\x6f\x72\x74\x62\x5f\x56\x32\x5f\x35\x2e\x53\x65\x61\x74\x42\x69" +
	"\x64\x52\x07\x73\x65\x61\x74\x62\x69\x64\x12\x19\x0a\x05\x62\x69\x64\x69\x64\x18\x03\x20\x01\x28" +
	"\x09\x48\x01\x52\x05\x62\x69\x64\x69\x64\x88\x01\x01\x12\x15\x0a\x03\x63\x75\x72\x18\x04\x20\x01" +
	"\x28\x09\x48\x02\x52\x03\x63\x75\x72\x88\x01\x01\x12\x15\x0a\x03\x6e\x62\x72\x18\x05\x20\x01\x28" +
	"\x05\x48\x03\x52\x03\x6e\x62\x72\x88\x01\x01\x12\x25\x0a\x03\x65\x78\x74\x18\x06\x20\x01\x28\x0b" +
	"\x32\x0e\x2e\x6f\x72\x74\x62\x5f\x56\x32\x5f\x35\x2e\x45\x78\x74\x48\x04\x52\x03\x65\x78\x74\x88" +
	"\x01\x01\x42\x05\x0a\x03\x5f\x69\x64\x42\x08\x0a\x06\x5f\x62\x69\x64\x69\x64\x42\x06\x0a\x04\x5f" +
	"\x63\x75\x72\x42\x06\x0a\x04\x5f\x6e\x62\x72\x42\x06\x0a\x04\x5f\x65\x78\x74\x22\x74\x0a\x03\x45" +
	"\x78\x74\x12\x32\x0a\x06\x76\x61\x6c\x75\x65\x73\x18\x01\x20\x03\x28\x0b\x32\x1a\x2e\x6f\x72\x74" +
	"\x62\x5f\x56\x32\x5f\x35\x2e\x45\x78\x74\x2e\x56\x61\x6c\x75\x65\x73\x45\x6e\x74\x72\x79\x52\x06" +
	"\x76\x61\x6c\x75\x65\x73\x1a\x39\x0a\x0b\x56\x61\x6c\x75\x65\x73\x45\x6e\x74\x72\x79\x12\x10\x0a" +
	"\x03\x6b\x65\x79\x18\x01\x20\x01\x28\x09\x52\x03\x6b\x65\x79\x12\x14\x0a\x05\x76\x61\x6c\x75\x65" +
	"\x18\x02\x20\x01\x28\x09\x52\x05\x76\x61\x6c\x75\x65\x3a\x02\x38\x01\x42\x58\x5a\x56\x67\x69\x74" +
	"\x6c\x61\x62\x2e\x63\x6f\x6d\x2f\x74\x77\x69\x6e\x62\x69\x64\x2d\x65\x78\x63\x68\x61\x6e\x67\x65" +
	"\x2f\x52\x54\x42\x2d\x65\x78\x63\x68\x61\x6e\x67\x65\x2f\x69\x6e\x74\x65\x72\x6e\x61\x6c\x2f\x67" +
	"\x72\x70\x63\x2f\x70\x72\x6f\x74\x6f\x2f\x74\x79\x70\x65\x73\x2f\x6f\x72\x74\x62\x5f\x56\x32\x5f" +
	"\x35\x3b\x6f\x72\x74\x62\x5f\x56\x32\x5f\x35\x62\x06\x70\x72\x6f\x74\x6f\x33"

var (
	file_types_ortb_V2_5_ortb_proto_rawDescOnce sync.Once
	file_types_ortb_V2_5_ortb_proto_rawDescData []byte
)

func file_types_ortb_V2_5_ortb_proto_rawDescGZIP() []byte {
	file_types_ortb_V2_5_ortb_proto_rawDescOnce.Do(func() {
		file_types_ortb_V2_5_ortb_proto_rawDescData = protoimpl.X.CompressGZIP(unsafe.Slice(unsafe.StringData(file_types_ortb_V2_5_ortb_proto_rawDesc), len(file_types_ortb_V2_5_ortb_proto_rawDesc)))
	})
	return file_types_ortb_V2_5_ortb_proto_rawDescData
}

var file_types_ortb_V2_5_ortb_proto_msgTypes = make([]protoimpl.MessageInfo, 17)
var file_types_ortb_V2_5_ortb_proto_goTypes = []any{
	(*BidRequest)(nil),  // ortb_V2_5.BidRequest
	(*Imp)(nil),         // ortb_V2_5.Imp
	(*Banner)(nil),      // ortb_V2_5.Banner
	(*Format)(nil),      // ortb_V2_5.Format
	(*Native)(nil),      // ortb_V2_5.Native
	(*Imp_Ext)(nil),     // ortb_V2_5.Imp_Ext
	(*Device)(nil),      // ortb_V2_5.Device
	(*Geo)(nil),         // ortb_V2_5.Geo
	(*Site)(nil),        // ortb_V2_5.Site
	(*Publisher)(nil),   // ortb_V2_5.Publisher
	(*User)(nil),        // ortb_V2_5.User
	(*SeatBid)(nil),     // ortb_V2_5.SeatBid
	(*Bid)(nil),         // ortb_V2_5.Bid
	(*BidExt)(nil),      // ortb_V2_5.BidExt
	(*BidResponse)(nil), // ortb_V2_5.BidResponse
	(*Ext)(nil),         // ortb_V2_5.Ext
	nil,                 // ortb_V2_5.Ext.ValuesEntry
}

var file_types_ortb_V2_5_ortb_proto_depIdxs = []int32{
	1,
	6,
	8,
	10,
	5,
	2,
	4,
	3,
	7,
	15,
	9,
	12,
	13,
	11,
	15,
	16,
	16,
	16,
	16,
	16,
	0,
}

func init() { file_types_ortb_V2_5_ortb_proto_init() }
func file_types_ortb_V2_5_ortb_proto_init() {
	if File_types_ortb_V2_5_ortb_proto != nil {
		return
	}
	file_types_ortb_V2_5_ortb_proto_msgTypes[0].OneofWrappers = []any{}
	file_types_ortb_V2_5_ortb_proto_msgTypes[1].OneofWrappers = []any{}
	file_types_ortb_V2_5_ortb_proto_msgTypes[2].OneofWrappers = []any{}
	file_types_ortb_V2_5_ortb_proto_msgTypes[3].OneofWrappers = []any{}
	file_types_ortb_V2_5_ortb_proto_msgTypes[4].OneofWrappers = []any{}
	file_types_ortb_V2_5_ortb_proto_msgTypes[5].OneofWrappers = []any{}
	file_types_ortb_V2_5_ortb_proto_msgTypes[6].OneofWrappers = []any{}
	file_types_ortb_V2_5_ortb_proto_msgTypes[7].OneofWrappers = []any{}
	file_types_ortb_V2_5_ortb_proto_msgTypes[8].OneofWrappers = []any{}
	file_types_ortb_V2_5_ortb_proto_msgTypes[9].OneofWrappers = []any{}
	file_types_ortb_V2_5_ortb_proto_msgTypes[10].OneofWrappers = []any{}
	file_types_ortb_V2_5_ortb_proto_msgTypes[11].OneofWrappers = []any{}
	file_types_ortb_V2_5_ortb_proto_msgTypes[12].OneofWrappers = []any{}
	file_types_ortb_V2_5_ortb_proto_msgTypes[13].OneofWrappers = []any{}
	file_types_ortb_V2_5_ortb_proto_msgTypes[14].OneofWrappers = []any{}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_types_ortb_V2_5_ortb_proto_rawDesc), len(file_types_ortb_V2_5_ortb_proto_rawDesc)),
			NumEnums:      0,
			NumMessages:   17,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_types_ortb_V2_5_ortb_proto_goTypes,
		DependencyIndexes: file_types_ortb_V2_5_ortb_proto_depIdxs,
		MessageInfos:      file_types_ortb_V2_5_ortb_proto_msgTypes,
	}.Build()
	File_types_ortb_V2_5_ortb_proto = out.File
	file_types_ortb_V2_5_ortb_proto_goTypes = nil
	file_types_ortb_V2_5_ortb_proto_depIdxs = nil
}
