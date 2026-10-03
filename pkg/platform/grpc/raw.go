// Package grpc contains the policy-free transport used by services that speak
// a pinned gRPC protocol. Protocol method names and messages belong to the
// owning service; this package only owns dialing, cancellation, and bytes.
package grpc

import (
	"context"
	"fmt"
	"strings"

	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// Connection is the minimal unary transport used by generated or hand-owned
// protocol adapters. The transport deliberately accepts serialized messages
// so backend-native protobuf types remain behind their owning boundary.
type Connection interface {
	Invoke(context.Context, string, []byte) ([]byte, error)
	Close() error
}

// HeaderConnection is the optional extension of Connection for callers that
// also need response metadata. The transport only reports the headers the
// peer sent; it does not interpret them or decide what they mean. Callers
// that only need response bytes keep using Connection.
type HeaderConnection interface {
	Connection
	InvokeWithHeaders(context.Context, string, []byte) ([]byte, map[string][]string, error)
}

// Dialer creates one transport connection to an already selected endpoint.
// It does not choose endpoints, retry policy, or protocol methods.
type Dialer interface {
	Dial(context.Context, string) (Connection, error)
}

// NetworkDialer is the production TCP gRPC dialer. The caller's context owns
// connection setup and every unary invocation; no background retry loop is
// created here.
type NetworkDialer struct{}

var _ Dialer = NetworkDialer{}

// Dial opens one insecure local model-host connection. LocalAI workers use
// grpc:// endpoints in authored runtime configuration; grpc-go expects the
// host:port target after that local transport marker is removed.
func (NetworkDialer) Dial(ctx context.Context, endpoint string) (Connection, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	target, err := normalizeEndpoint(endpoint)
	if err != nil {
		return nil, err
	}
	connection, err := grpcgo.DialContext(
		ctx,
		target,
		grpcgo.WithTransportCredentials(insecure.NewCredentials()),
		grpcgo.WithBlock(),
	)
	if err != nil {
		return nil, err
	}
	return networkConnection{next: connection}, nil
}

type networkConnection struct {
	next *grpcgo.ClientConn
}

var _ HeaderConnection = networkConnection{}

// Invoke keeps the byte-only call contract by discarding response headers.
func (connection networkConnection) Invoke(
	ctx context.Context,
	method string,
	request []byte,
) ([]byte, error) {
	response, _, err := connection.InvokeWithHeaders(ctx, method, request)
	if err != nil {
		return nil, err
	}
	return response, nil
}

// InvokeWithHeaders returns the same response bytes as Invoke together with
// the response metadata the peer sent. Header keys are normalized to lower
// case and the returned map is a copy the caller may keep.
func (connection networkConnection) InvokeWithHeaders(
	ctx context.Context,
	method string,
	request []byte,
) ([]byte, map[string][]string, error) {
	if connection.next == nil {
		return nil, nil, fmt.Errorf("gRPC connection is unavailable")
	}
	var response []byte
	var headers metadata.MD
	if err := connection.next.Invoke(
		ctx,
		method,
		request,
		&response,
		grpcgo.ForceCodec(rawCodec{}),
		grpcgo.Header(&headers),
	); err != nil {
		return nil, nil, err
	}
	return response, copyHeaderValues(headers), nil
}

func copyHeaderValues(headers metadata.MD) map[string][]string {
	if len(headers) == 0 {
		return nil
	}
	values := make(map[string][]string, len(headers))
	for key, entries := range headers {
		values[key] = append([]string(nil), entries...)
	}
	return values
}

func (connection networkConnection) Close() error {
	if connection.next == nil {
		return nil
	}
	return connection.next.Close()
}

// rawCodec lets grpc-go carry protobuf bytes already serialized by the owning
// protocol adapter. The server still sees the normal proto content subtype.
type rawCodec struct{}

func (rawCodec) Name() string { return "proto" }

func (rawCodec) Marshal(value any) ([]byte, error) {
	bytes, ok := value.([]byte)
	if !ok {
		return nil, fmt.Errorf("gRPC raw codec received %T, want []byte", value)
	}
	return bytes, nil
}

func (rawCodec) Unmarshal(data []byte, value any) error {
	bytes, ok := value.(*[]byte)
	if !ok {
		return fmt.Errorf("gRPC raw codec received %T, want *[]byte", value)
	}
	*bytes = append((*bytes)[:0], data...)
	return nil
}

func normalizeEndpoint(endpoint string) (string, error) {
	target := strings.TrimSpace(endpoint)
	for _, prefix := range []string{"grpc://", "tcp://"} {
		if strings.HasPrefix(strings.ToLower(target), prefix) {
			target = strings.TrimSpace(target[len(prefix):])
			break
		}
	}
	if target == "" {
		return "", fmt.Errorf("gRPC endpoint is required")
	}
	return target, nil
}
