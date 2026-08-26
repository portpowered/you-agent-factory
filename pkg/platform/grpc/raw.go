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
)

// Connection is the minimal unary transport used by generated or hand-owned
// protocol adapters. The transport deliberately accepts serialized messages
// so backend-native protobuf types remain behind their owning boundary.
type Connection interface {
	Invoke(context.Context, string, []byte) ([]byte, error)
	Close() error
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

func (connection networkConnection) Invoke(
	ctx context.Context,
	method string,
	request []byte,
) ([]byte, error) {
	if connection.next == nil {
		return nil, fmt.Errorf("gRPC connection is unavailable")
	}
	var response []byte
	if err := connection.next.Invoke(
		ctx,
		method,
		request,
		&response,
		grpcgo.ForceCodec(rawCodec{}),
	); err != nil {
		return nil, err
	}
	return response, nil
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
