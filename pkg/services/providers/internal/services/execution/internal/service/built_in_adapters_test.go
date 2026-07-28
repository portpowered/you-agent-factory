package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

func TestBuiltInRegistrationsSelectDistinctCodexClaudeCursorAndOpenCodeAdapters(t *testing.T) {
	t.Parallel()

	registrations := BuiltInRegistrations()
	if len(registrations) != 4 {
		t.Fatalf("registration count = %d, want 4", len(registrations))
	}

	byID := make(map[providers.ID]string, len(registrations))
	for _, registration := range registrations {
		_, err := registration.Attempt(
			context.Background(),
			providers.ExecuteRequest{Provider: registration.Provider},
		)
		var failure providers.ExecuteFailure
		if !errors.As(err, &failure) ||
			failure.Kind != providers.ExecuteFailureKindDependency {
			t.Fatalf("adapter %q error = %#v, want dependency failure", registration.Provider, err)
		}
		byID[registration.Provider] = failure.Message
	}

	if !strings.Contains(byID[providers.IDCodex], "Codex") {
		t.Fatalf("Codex adapter message = %q", byID[providers.IDCodex])
	}
	if !strings.Contains(byID[providers.IDClaude], "Claude") {
		t.Fatalf("Claude adapter message = %q", byID[providers.IDClaude])
	}
	if !strings.Contains(byID[providers.IDCursor], "Cursor") {
		t.Fatalf("Cursor adapter message = %q", byID[providers.IDCursor])
	}
	if !strings.Contains(byID[providers.IDOpenCode], "OpenCode") {
		t.Fatalf("OpenCode adapter message = %q", byID[providers.IDOpenCode])
	}
}
