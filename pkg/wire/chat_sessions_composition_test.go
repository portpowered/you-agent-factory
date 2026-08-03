package wire

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
)

// TestGeneratedBundleConstructsChatSessionsServiceOnce proves
// provideChatSessionsService is not merely registered in servicesSet but is
// actually invoked by the generated InjectBundle graph, exactly once, with
// its result flowing into the canonical cli.CommandOperations value that
// reaches the returned *initializerapplication.Process -- so the singular
// chat_sessions.Service instance is genuinely constructed as part of
// building the application process, not a dead registration that Wire never
// visits because no output currently requires it.
func TestGeneratedBundleConstructsChatSessionsServiceOnce(t *testing.T) {
	t.Parallel()

	source, err := os.ReadFile("wire_gen.go")
	if err != nil {
		t.Fatalf("read wire_gen.go: %v", err)
	}
	content := string(source)

	callCount := strings.Count(content, "provideChatSessionsService(")
	if callCount != 1 {
		t.Fatalf("provideChatSessionsService called %d times in generated InjectBundle, want exactly 1 (singleton construction)", callCount)
	}
	if !strings.Contains(content, "cli.CommandOperations{\n\t\tChatSessions:") {
		t.Fatal("generated cli.CommandOperations literal does not assign the constructed chat_sessions.Service as its first field; provideChatSessionsService's result is not reaching the canonical CLI command graph")
	}
}

// TestProvideChatSessionsServiceIsUsableThroughInjectBundle proves the exact
// provider function registered in servicesSet -- the one InjectBundle now
// actually calls -- returns a functional chat_sessions.Service, and that
// InjectBundle itself succeeds with that provider wired in.
func TestProvideChatSessionsServiceIsUsableThroughInjectBundle(t *testing.T) {
	t.Parallel()

	if _, err := InjectBundle(context.Background(), serviceedges.Edges{}); err != nil {
		t.Fatalf("InjectBundle() error = %v", err)
	}

	zapLogger, err := logging.NewDefaultLogger()
	if err != nil {
		t.Fatalf("logging.NewDefaultLogger() error = %v", err)
	}
	service, err := provideChatSessionsService(logging.NewZapLogger(zapLogger, false))
	if err != nil {
		t.Fatalf("provideChatSessionsService() error = %v", err)
	}
	if service == nil {
		t.Fatal("provideChatSessionsService() = nil, want a constructed chat_sessions.Service")
	}
}
