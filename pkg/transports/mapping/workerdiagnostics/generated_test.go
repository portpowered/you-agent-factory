package workerdiagnostics

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestProviderSessionMetadataFromGenerated_PreservesProviderRepresentation(t *testing.T) {
	t.Parallel()

	generated := GeneratedProviderSessionMetadata(&workers.ProviderSessionMetadata{
		Provider: "agent",
		Kind:     "session_id",
		ID:       "cursor-session-123",
	})
	if generated == nil || generated.Provider == nil || *generated.Provider != "agent" {
		t.Fatalf("generated = %#v, want lossless provider representation", generated)
	}

	canonical := ProviderSessionMetadataFromGenerated(&factoryapi.ProviderSessionMetadata{
		Provider: stringPtr("cursor-agent"),
		Kind:     stringPtr("session_id"),
		Id:       stringPtr("cursor-session-456"),
	})
	if canonical == nil || canonical.Provider != "cursor-agent" {
		t.Fatalf("canonical = %#v, want lossless provider representation", canonical)
	}
}

func stringPtr(value string) *string { return &value }
