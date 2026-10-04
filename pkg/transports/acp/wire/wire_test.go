package wire_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/contextscope"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/transports/acp"
	"github.com/portpowered/infinite-you/pkg/transports/acp/wire"
)

func testInvocationScope(ctx context.Context) acp.InvocationScope { return contextscope.New(ctx) }

func TestNewServerConstructsWithoutIO(t *testing.T) {
	server := wire.NewServer(logging.NoopLogger{},

		nil, nil, nil, nil, nil, nil, nil, nil, testInvocationScope,
	)
	if server == nil {
		t.Fatal("NewServer() returned nil")
	}
}

func TestNewServerServesOneConnection(t *testing.T) {
	server := wire.NewServer(logging.NoopLogger{},

		nil, nil, nil, nil, nil, nil, nil, nil, testInvocationScope,
	)
	out := &bytes.Buffer{}

	if err := server.Serve(context.Background(), strings.NewReader(""), out); err != nil {
		t.Fatalf("Serve() error = %v, want nil on clean EOF", err)
	}
}

func TestNewServerRejectsMissingStreams(t *testing.T) {
	server := wire.NewServer(logging.NoopLogger{},

		nil, nil, nil, nil, nil, nil, nil, nil, testInvocationScope,
	)

	if err := server.Serve(context.Background(), nil, nil); err == nil {
		t.Fatal("Serve() error = nil, want an actionable error for missing streams")
	}
}
