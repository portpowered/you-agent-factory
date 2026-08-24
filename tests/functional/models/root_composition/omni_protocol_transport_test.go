package root_composition_test

import (
	"context"
	"fmt"
	"net"
	"testing"

	platformgrpc "github.com/portpowered/infinite-you/pkg/platform/grpc"
	grpcgo "google.golang.org/grpc"
	grpcencoding "google.golang.org/grpc/encoding"
)

func TestOmniProtocolTransportRoundTripsThroughNetworkDialer(t *testing.T) {
	t.Parallel()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for protocol fixture: %v", err)
	}
	server := grpcgo.NewServer(grpcgo.ForceServerCodec(rawBytesCodec{}))
	server.RegisterService(&rawBytesServiceDescription, rawBytesServiceImpl{})
	serveDone := make(chan error, 1)
	go func() { serveDone <- server.Serve(listener) }()
	t.Cleanup(func() {
		server.Stop()
		_ = listener.Close()
		<-serveDone
	})

	connection, err := (platformgrpc.NetworkDialer{}).Dial(
		context.Background(), "grpc://"+listener.Addr().String(),
	)
	if err != nil {
		t.Fatalf("NetworkDialer.Dial: %v", err)
	}
	response, err := connection.Invoke(
		context.Background(), "/fixture.Raw/Echo", []byte("ordered omni bytes"),
	)
	if err != nil {
		t.Fatalf("Connection.Invoke: %v", err)
	}
	if string(response) != "ordered omni bytes" {
		t.Fatalf("Connection.Invoke response = %q, want echoed bytes", response)
	}
	if err := connection.Close(); err != nil {
		t.Fatalf("Connection.Close: %v", err)
	}
}

type rawBytesService interface {
	Echo(context.Context, []byte) ([]byte, error)
}

type rawBytesServiceImpl struct{}

func (rawBytesServiceImpl) Echo(_ context.Context, request []byte) ([]byte, error) {
	return append([]byte(nil), request...), nil
}

var rawBytesServiceDescription = grpcgo.ServiceDesc{
	ServiceName: "fixture.Raw",
	HandlerType: (*rawBytesService)(nil),
	Methods: []grpcgo.MethodDesc{{
		MethodName: "Echo",
		Handler:    rawBytesEchoHandler,
	}},
}

func rawBytesEchoHandler(
	srv any,
	ctx context.Context,
	decode func(any) error,
	interceptor grpcgo.UnaryServerInterceptor,
) (any, error) {
	request := []byte(nil)
	if err := decode(&request); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(rawBytesService).Echo(ctx, request)
	}
	info := &grpcgo.UnaryServerInfo{Server: srv, FullMethod: "/fixture.Raw/Echo"}
	handler := func(ctx context.Context, request any) (any, error) {
		bytes, ok := request.([]byte)
		if !ok {
			return nil, fmt.Errorf("raw fixture request type = %T", request)
		}
		return srv.(rawBytesService).Echo(ctx, bytes)
	}
	return interceptor(ctx, request, info, handler)
}

type rawBytesCodec struct{}

var _ grpcencoding.Codec = rawBytesCodec{}

func (rawBytesCodec) Name() string { return "proto" }

func (rawBytesCodec) Marshal(value any) ([]byte, error) {
	bytes, ok := value.([]byte)
	if !ok {
		return nil, fmt.Errorf("raw fixture codec received %T, want []byte", value)
	}
	return bytes, nil
}

func (rawBytesCodec) Unmarshal(data []byte, value any) error {
	bytes, ok := value.(*[]byte)
	if !ok {
		return fmt.Errorf("raw fixture codec received %T, want *[]byte", value)
	}
	*bytes = append((*bytes)[:0], data...)
	return nil
}
