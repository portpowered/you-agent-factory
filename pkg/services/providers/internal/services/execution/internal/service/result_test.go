package service

import (
	"reflect"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

func TestT7LiveProgressUsesBoundedDiagnosticRedaction(t *testing.T) {
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
	if len(observed) != maxProgressFacts || observed[0].Detail != "visible <redacted> <redacted> <redacted> <redacted> <redacted>" ||
		observed[0].Metadata["safe"] != "<redacted>" || observed[0].Metadata["api_key"] != "<redacted>" {
		t.Fatalf("live bounded redaction = %+v", observed)
	}
	if progress.Metadata["safe"] != "environment-secret" {
		t.Fatal("observer mutated provider metadata")
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

func TestLiveProgressRedactsExecutionOnlyWorkerTokenBeforeAdapterMutation(t *testing.T) {
	t.Parallel()
	var observed providers.ExecuteProgress
	request := providers.ExecuteRequest{
		ProcessEnvironment: []string{
			"YOU_WORKER_SESSION_TOKEN=planted-worker-token",
			"you_worker_session_token=second-worker-token",
			"YOU_WORKER_SESSION_ID=visible-worker",
		},
		EnvVars:          map[string]string{"API_KEY": "planted-api-key"},
		ProgressObserver: func(progress providers.ExecuteProgress) { observed = progress },
	}
	safe := safeProgressRequest(request)
	safe.ProcessEnvironment[0] = "YOU_WORKER_SESSION_TOKEN=changed-worker-token"
	safe.EnvVars["API_KEY"] = "changed-api-key"
	progress := providers.ExecuteProgress{
		Phase:    "delta",
		Detail:   "visible-worker planted-worker-token second-worker-token planted-api-key",
		Metadata: map[string]string{"safe": "planted-worker-token"},
	}
	safe.ObserveProgress(progress)
	if observed.Detail != "visible-worker <redacted> <redacted> <redacted>" || observed.Metadata["safe"] != "<redacted>" {
		t.Fatalf("execution-only token was not redacted")
	}
	if progress.Metadata["safe"] != "planted-worker-token" {
		t.Fatal("redaction mutated producer progress")
	}
}

func TestWorkerTokenClassificationPreservesNonsecretEnvironmentFacts(t *testing.T) {
	t.Parallel()
	request := providers.ExecuteRequest{ProcessEnvironment: []string{
		"YOU_WORKER_SESSION_TOKEN=planted-worker-token",
		"YOU_WORKER_SESSION_TOKEN=", "YOU_WORKER_SESSION_TOKEN",
		"OTHER_YOU_WORKER_SESSION_TOKEN=visible-other-value",
		"YOU_WORKER_SESSION_ID=visible-worker", "YOU_WORK_ID=visible-work",
	}}
	before := request.Clone()
	diagnostics := normalizeDiagnostics(providers.ExecuteDiagnostics{
		Metadata: map[string]string{"safe": "planted-worker-token visible-worker visible-work visible-other-value"},
	}, request)
	if diagnostics.Metadata["safe"] != "<redacted> visible-worker visible-work visible-other-value" {
		t.Fatal("classification lost privacy or nonsecret neighboring facts")
	}
	if !reflect.DeepEqual(request, before) {
		t.Fatal("classification mutated the execution environment")
	}
}
