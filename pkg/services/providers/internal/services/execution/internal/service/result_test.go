package service

import (
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

func TestNormalizeDiagnosticsPreservesUsageTokenMetadataKeys(t *testing.T) {
	t.Parallel()

	diagnostics := normalizeDiagnostics(providers.ExecuteDiagnostics{
		Progress: []providers.ExecuteProgress{{
			Phase: "usage.updated",
			Metadata: map[string]string{
				"input_tokens":     "12",
				"output_tokens":    "7",
				"reasoning_tokens": "3",
				"api-token":        "secret-value",
			},
		}},
	}, providers.ExecuteRequest{UserMessage: "hello"})

	usage := diagnostics.Progress[0].Metadata
	if usage["input_tokens"] != "12" || usage["output_tokens"] != "7" || usage["reasoning_tokens"] != "3" {
		t.Fatalf("usage metadata = %#v, want numeric token counts preserved", usage)
	}
	if usage["api-token"] != "<redacted>" {
		t.Fatalf("api-token = %q, want redacted", usage["api-token"])
	}
}
