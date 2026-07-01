package initializer_test

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/initializer"
)

func TestAPITransport_NilReceiverMethodsAreSafe(t *testing.T) {
	t.Parallel()

	var transport *initializer.APITransport
	if transport.SessionAPISurface() != nil {
		t.Fatal("expected nil session API surface for nil transport")
	}
	if err := transport.Run(context.Background()); err != nil {
		t.Fatalf("Run on nil transport: %v", err)
	}
}

func TestCLITransport_NilReceiverMethodsAreSafe(t *testing.T) {
	t.Parallel()

	var transport *initializer.CLITransport
	if transport.Runner() != nil {
		t.Fatal("expected nil runner for nil transport")
	}
}

func TestMCPTransport_NilReceiverSessionClientUsesDefault(t *testing.T) {
	t.Parallel()

	var transport *initializer.MCPTransport
	if transport.SessionClient() == nil {
		t.Fatal("expected default MCP session client for nil transport")
	}
}

func TestServices_NilStartupWorkerConfig(t *testing.T) {
	t.Parallel()

	var services *initializer.Services
	if worker, ok := services.StartupWorkerConfig("worker-a"); worker != nil || ok {
		t.Fatalf("StartupWorkerConfig(nil) = (%v, %v), want (nil, false)", worker, ok)
	}
}
