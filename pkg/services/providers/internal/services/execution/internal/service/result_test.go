package service

import (
	"reflect"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

func TestNormalizeDiagnosticsRedactsDeclaredEnvironmentValues(t *testing.T) {
	t.Parallel()
	request := providers.ExecuteRequest{EnvVars: map[string]string{
		"API_KEY": "environment-credential", "tool-secret": "tool-credential", "LANG": "visible-language",
		"EXPANDED_SECRET": "tool-credential-expanded",
	}}
	before := request.Clone()
	diagnostics := normalizeDiagnostics(providers.ExecuteDiagnostics{
		Progress: []providers.ExecuteProgress{{
			Phase: "tool.completed", Detail: "visible environment-credential tool-credential tool-credential-expanded visible-language",
			Metadata: map[string]string{"safe": "environment-credential", "tool": "tool-credential"},
		}},
		Metadata: map[string]string{"safe": "environment-credential tool-credential visible-language"},
	}, request)
	progress := diagnostics.Progress[0]
	if progress.Detail != "visible <redacted> <redacted> <redacted> visible-language" ||
		progress.Metadata["safe"] != "<redacted>" || progress.Metadata["tool"] != "<redacted>" ||
		diagnostics.Metadata["safe"] != "<redacted> <redacted> visible-language" {
		t.Fatalf("environment redaction lost privacy or neighboring content: %+v", diagnostics)
	}
	if !reflect.DeepEqual(request, before) {
		t.Fatal("diagnostic redaction mutated the execution request")
	}
}

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
