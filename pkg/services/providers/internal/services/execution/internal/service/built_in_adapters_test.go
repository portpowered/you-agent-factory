package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

func TestBuiltInRegistrationsSelectDistinctCodexClaudeCursorGeminiKiroAndOpenCodeAdapters(t *testing.T) {
	t.Parallel()

	registrations := BuiltInRegistrations()
	if len(registrations) != 7 {
		t.Fatalf("registration count = %d, want 7", len(registrations))
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
	if !strings.Contains(byID[providers.IDGemini], "Gemini") {
		t.Fatalf("Gemini adapter message = %q", byID[providers.IDGemini])
	}
	if !strings.Contains(byID[providers.IDKiro], "Kiro") {
		t.Fatalf("Kiro adapter message = %q", byID[providers.IDKiro])
	}
	if !strings.Contains(byID[providers.IDPi], "Pi") {
		t.Fatalf("Pi adapter message = %q", byID[providers.IDPi])
	}
}
