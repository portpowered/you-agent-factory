package grpc

import (
	"context"
	"net"
	"testing"

	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	echoServiceName = "transport.test.Headers"
	echoMethodName  = "Echo"
	echoMethod      = "/" + echoServiceName + "/" + echoMethodName
	deniedRequest   = "deny"
	testHeaderKey   = "x-test"
)

func TestNormalizeEndpointRemovesLocalTransportMarker(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name     string
		endpoint string
		want     string
	}{
		{name: "grpc", endpoint: " grpc://127.0.0.1:50051 ", want: "127.0.0.1:50051"},
		{name: "tcp", endpoint: "tcp://127.0.0.1:50051", want: "127.0.0.1:50051"},
		{name: "plain", endpoint: "127.0.0.1:50051", want: "127.0.0.1:50051"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := normalizeEndpoint(test.endpoint)
			if err != nil {
				t.Fatalf("normalizeEndpoint() error = %v", err)
			}
			if got != test.want {
				t.Fatalf("normalizeEndpoint() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestNormalizeEndpointRejectsEmptyEndpoint(t *testing.T) {
	t.Parallel()

	if _, err := normalizeEndpoint(" "); err == nil {
		t.Fatal("normalizeEndpoint(empty) error = nil, want error")
	}
}

func TestNetworkConnectionInvokeWithHeadersReturnsResponseMetadata(t *testing.T) {
	t.Parallel()

	connection := startEchoServer(t)
	t.Cleanup(func() { _ = connection.Close() })
	headerConnection := requireHeaderConnection(t, connection)

	body, headers, err := headerConnection.InvokeWithHeaders(t.Context(), echoMethod, []byte("ping"))
	if err != nil {
		t.Fatalf("InvokeWithHeaders() error = %v", err)
	}
	if string(body) != "echo:ping" {
		t.Fatalf("InvokeWithHeaders() body = %q, want %q", body, "echo:ping")
	}
	if got := headers[testHeaderKey]; len(got) != 1 || got[0] != "yes" {
		t.Fatalf("InvokeWithHeaders() headers[%q] = %v, want [yes]", testHeaderKey, got)
	}

	existing, err := connection.Invoke(t.Context(), echoMethod, []byte("ping"))
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if string(existing) != "echo:ping" {
		t.Fatalf("Invoke() body = %q, want %q", existing, "echo:ping")
	}
}

func TestNetworkConnectionInvokeWithHeadersPropagatesCallFailures(t *testing.T) {
	t.Parallel()

	connection := startEchoServer(t)
	t.Cleanup(func() { _ = connection.Close() })
	headerConnection := requireHeaderConnection(t, connection)

	for _, test := range []struct {
		name             string
		request          string
		cancelBeforeCall bool
		wantCode         codes.Code
	}{
		{name: "permission denied", request: deniedRequest, wantCode: codes.PermissionDenied},
		{name: "canceled context", request: "ping", cancelBeforeCall: true, wantCode: codes.Canceled},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			t.Cleanup(cancel)
			if test.cancelBeforeCall {
				cancel()
			}

			body, headers, err := headerConnection.InvokeWithHeaders(ctx, echoMethod, []byte(test.request))
			if got := status.Code(err); got != test.wantCode {
				t.Fatalf("InvokeWithHeaders() code = %v, want %v (error = %v)", got, test.wantCode, err)
			}
			if body != nil || headers != nil {
				t.Fatalf("InvokeWithHeaders() = %q, %v, want nil values on failure", body, headers)
			}
		})
	}
}

func requireHeaderConnection(t *testing.T, connection Connection) HeaderConnection {
	t.Helper()

	headerConnection, ok := connection.(HeaderConnection)
	if !ok {
		t.Fatalf("%T does not implement HeaderConnection", connection)
	}
	return headerConnection
}

// startEchoServer runs one real loopback gRPC server that speaks the same raw
// byte codec as the client and answers Echo with a header plus echoed bytes.
func startEchoServer(t *testing.T) Connection {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen on loopback endpoint: %v", err)
	}
	server := grpcgo.NewServer(grpcgo.ForceServerCodec(rawCodec{}))
	server.RegisterService(echoServiceDesc(), &echoServer{})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)

	connection, err := NetworkDialer{}.Dial(t.Context(), listener.Addr().String())
	if err != nil {
		t.Fatalf("dial echo server at %s: %v", listener.Addr(), err)
	}
	return connection
}

type echoServer struct{}

func (*echoServer) echo(ctx context.Context, request []byte) ([]byte, error) {
	if string(request) == deniedRequest {
		return nil, status.Error(codes.PermissionDenied, "denied by echo server")
	}
	if err := grpcgo.SetHeader(ctx, metadata.Pairs(testHeaderKey, "yes")); err != nil {
		return nil, err
	}
	return []byte("echo:" + string(request)), nil
}

func echoServiceDesc() *grpcgo.ServiceDesc {
	return &grpcgo.ServiceDesc{
		ServiceName: echoServiceName,
		HandlerType: (*any)(nil),
		Methods: []grpcgo.MethodDesc{{
			MethodName: echoMethodName,
			Handler: func(service any, ctx context.Context, decode func(any) error, interceptor grpcgo.UnaryServerInterceptor) (any, error) {
				server, ok := service.(*echoServer)
				if !ok {
					return nil, status.Errorf(codes.Unimplemented, "unexpected service %T", service)
				}
				var request []byte
				if err := decode(&request); err != nil {
					return nil, err
				}
				if interceptor == nil {
					return server.echo(ctx, request)
				}
				return interceptor(ctx, request, &grpcgo.UnaryServerInfo{
					Server:     service,
					FullMethod: echoMethod,
				}, func(ctx context.Context, request any) (any, error) {
					return server.echo(ctx, request.([]byte))
				})
			},
		}},
		Metadata: echoServiceName,
	}
}
