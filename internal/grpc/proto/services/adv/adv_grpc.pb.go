// Code generated manually to match protoc-gen-go-grpc style. DO NOT EDIT.
// source: services/adv.proto

package advGrpc

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

const _ = grpc.SupportPackageIsVersion9

const (
	AdvService_DoAuction_FullMethodName = "/adv.AdvService/DoAuction"
)

type AdvServiceClient interface {
	DoAuction(ctx context.Context, in *DoAuctionRequest, opts ...grpc.CallOption) (*DoAuctionResponse, error)
}

type advServiceClient struct {
	cc grpc.ClientConnInterface
}

func NewAdvServiceClient(cc grpc.ClientConnInterface) AdvServiceClient {
	return &advServiceClient{cc}
}

func (c *advServiceClient) DoAuction(ctx context.Context, in *DoAuctionRequest, opts ...grpc.CallOption) (*DoAuctionResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(DoAuctionResponse)
	err := c.cc.Invoke(ctx, AdvService_DoAuction_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

type AdvServiceServer interface {
	DoAuction(context.Context, *DoAuctionRequest) (*DoAuctionResponse, error)
	mustEmbedUnimplementedAdvServiceServer()
}

type UnimplementedAdvServiceServer struct{}

func (UnimplementedAdvServiceServer) DoAuction(context.Context, *DoAuctionRequest) (*DoAuctionResponse, error) {
	return nil, status.Error(codes.Unimplemented, "method DoAuction not implemented")
}
func (UnimplementedAdvServiceServer) mustEmbedUnimplementedAdvServiceServer() {}
func (UnimplementedAdvServiceServer) testEmbeddedByValue()                    {}

type UnsafeAdvServiceServer interface {
	mustEmbedUnimplementedAdvServiceServer()
}

func RegisterAdvServiceServer(s grpc.ServiceRegistrar, srv AdvServiceServer) {
	if t, ok := srv.(interface{ testEmbeddedByValue() }); ok {
		t.testEmbeddedByValue()
	}
	s.RegisterService(&AdvService_ServiceDesc, srv)
}

func _AdvService_DoAuction_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(DoAuctionRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdvServiceServer).DoAuction(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: AdvService_DoAuction_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdvServiceServer).DoAuction(ctx, req.(*DoAuctionRequest))
	}
	return interceptor(ctx, in, info, handler)
}

var AdvService_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "adv.AdvService",
	HandlerType: (*AdvServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "DoAuction",
			Handler:    _AdvService_DoAuction_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "services/adv.proto",
}
