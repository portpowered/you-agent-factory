package grpc

import "testing"

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
