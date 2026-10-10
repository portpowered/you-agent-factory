package service

import (
	"reflect"
	"strings"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

func TestRecordingContentPreservesOrdinaryEchoAndRedactsDeclaredSecrets(t *testing.T) {
	t.Parallel()
	var observed []providers.ExecuteProgress
	request := providers.ExecuteRequest{
		SystemPrompt: "system-secret", UserMessage: "prompt-secret", OutputSchema: "schema-secret",
		EnvVars:          map[string]string{"API_KEY": "environment-secret"},
		ProgressObserver: func(progress providers.ExecuteProgress) { observed = append(observed, progress) },
	}
	safe := safeProgressRequest(request, "continuation-secret")
	progress := providers.ExecuteProgress{Phase: "delta", Detail: "visible system-secret prompt-secret schema-secret environment-secret continuation-secret",
		Metadata: map[string]string{"safe": "environment-secret", "api_key": "hidden"}}
	for count := 0; count < maxProgressFacts+1; count++ {
		safe.ObserveProgress(progress)
	}
	if len(observed) != maxProgressFacts || observed[0].Detail != "visible system-secret prompt-secret schema-secret <redacted> <redacted>" ||
		observed[0].Metadata["safe"] != "<redacted>" || observed[0].Metadata["api_key"] != "<redacted>" {
		t.Fatalf("live bounded redaction = %+v", observed)
	}
	if progress.Metadata["safe"] != "environment-secret" {
		t.Fatal("observer mutated provider metadata")
	}
}

func TestRecordingContentKeepsAdmittedDetailAndShortSecrets(t *testing.T) {
	t.Parallel()
	ordinary := strings.Repeat("ordinary echo ", 100)
	request := providers.ExecuteRequest{UserMessage: ordinary, EnvVars: map[string]string{"API_KEY": "xyz"}}
	progress := sanitizeCapturedProgress(providers.ExecuteProgress{Phase: "message.completed", Detail: ordinary + "xyz"}, request)
	if progress.Detail != ordinary+redactedValue {
		t.Fatal("capture lost ordinary text or retained a short declared secret")
	}
	diagnostics := normalizeDiagnostics(providers.ExecuteDiagnostics{Progress: []providers.ExecuteProgress{{Phase: "message.completed", Detail: ordinary}}}, request)
	if diagnostics.Progress[0].Detail != redactedValue {
		t.Fatal("diagnostic prompt redaction was weakened")
	}
}

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
