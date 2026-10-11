package service

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

func TestCapturedFactsPreserveNoncredentialEnvironmentValues(t *testing.T) {
	t.Parallel()
	const id = "8528a10f-6314-48ba-8230-542a4cf660f9"
	for _, value := range []string{"", "1", "1000000", id} {
		t.Run(value, func(t *testing.T) {
			t.Parallel()
			request := providers.ExecuteRequest{
				EnvVars:            map[string]string{"VALIDATION_ENVIRONMENT": value, "COMMAND_OUTPUT": value},
				ProcessEnvironment: []string{"VALIDATION_ENVIRONMENT=" + value, "PROMPT_MODE=" + value},
			}
			before := request.Clone()
			detail := `{"text":"` + id + `","usage":{"input_tokens":1000000,"output_tokens":2000000,"total_tokens":3000000}}`
			fact := providers.ExecuteProgress{Phase: "usage.updated", Detail: detail, Metadata: map[string]string{"input_tokens": "1000000", "safe": id}}
			var live providers.ExecuteProgress
			request.ProgressObserver = func(progress providers.ExecuteProgress) { live = progress }
			safeProgressRequest(request).ObserveProgress(fact)
			terminal := normalizeDiagnostics(providers.ExecuteDiagnostics{Progress: []providers.ExecuteProgress{fact}}, request).Progress[0]
			for _, got := range []providers.ExecuteProgress{live, terminal} {
				if got.Detail != detail || !json.Valid([]byte(got.Detail)) || !reflect.DeepEqual(got.Metadata, fact.Metadata) {
					t.Fatalf("safe captured UUID/typed usage changed: %+v", got)
				}
			}
			request.ProgressObserver = nil
			if !reflect.DeepEqual(request, before) {
				t.Fatal("sanitizer mutated request")
			}
		})
	}
}

func TestCapturedFactsKeepCredentialPrivacyAndDiagnosticBounds(t *testing.T) {
	t.Parallel()
	for _, key := range []string{"API_KEY", "api-key", "ApiKey", "AUTHORIZATION", "PASSWORD", "CREDENTIAL", "tool-secret", "ACCESS_TOKEN"} {
		t.Run(key, func(t *testing.T) {
			t.Parallel()
			request := providers.ExecuteRequest{SystemPrompt: "private prompt", UserMessage: "private input", OutputSchema: "private schema",
				EnvVars:            map[string]string{key: "xyz", "EXPANDED_SECRET": "xyz-expanded"},
				ProcessEnvironment: []string{key + "=inherited-credential", "YOU_WORKER_SESSION_TOKEN=worker-credential", "VALIDATION_ENVIRONMENT=1"}}
			fact := providers.ExecuteProgress{Phase: "tool.completed", Detail: "safe 123 xyz-expanded xyz inherited-credential worker-credential continuation-credential",
				Metadata: map[string]string{"safe": "123 xyz", "authorization": "hidden"}}
			want := "safe 123 <redacted> <redacted> <redacted> <redacted> <redacted>"
			live := sanitizeCapturedProgress(fact, request, "continuation-credential")
			terminal := normalizeDiagnostics(providers.ExecuteDiagnostics{Progress: []providers.ExecuteProgress{fact}}, request, "continuation-credential")
			for _, got := range []providers.ExecuteProgress{live, terminal.Progress[0]} {
				if got.Detail != want || got.Metadata["safe"] != "123 <redacted>" || got.Metadata["authorization"] != redactedValue {
					t.Fatalf("credential privacy/safe neighbors changed: %+v", got)
				}
			}
			private := "private prompt private input private schema " + strings.Repeat("safe ", maxDiagnosticRunes)
			bounded := normalizeDiagnostics(providers.ExecuteDiagnostics{Progress: []providers.ExecuteProgress{{Detail: private}}, Metadata: map[string]string{"stdout": "private output"}}, request)
			if len([]rune(bounded.Progress[0].Detail)) != maxDiagnosticRunes || !strings.HasPrefix(bounded.Progress[0].Detail, "<redacted> <redacted> <redacted>") || bounded.Metadata["stdout"] != redactedValue || fact.Metadata["safe"] != "123 xyz" {
				t.Fatal("private diagnostic filtering, bounds or producer immutability changed")
			}
		})
	}
}

func TestCapturedFactsRedactEvenOneCharacterCredential(t *testing.T) {
	t.Parallel()
	request := providers.ExecuteRequest{EnvVars: map[string]string{"API_KEY": "1", "VALIDATION_ENVIRONMENT": "0"}}
	live := sanitizeCapturedProgress(providers.ExecuteProgress{Detail: "safe 0 key=1"}, request)
	terminal, err := normalizeSuccess(providers.ExecuteResult{Content: "safe 0 key=1"}, providers.IDCodex, request)
	if err != nil || live.Detail != "safe 0 key=<redacted>" || terminal.Content != live.Detail {
		t.Fatal("short real credential was exempted or ordinary flag corrupted")
	}
}

func TestCapturedFactsSeparateTerminalCredentialsFromDiagnosticContext(t *testing.T) {
	t.Parallel()
	const sessionID = "8528a10f-6314-48ba-8230-542a4cf660f9"
	request := providers.ExecuteRequest{
		EnvVars:            map[string]string{"API_KEY": "api-credential"},
		ProcessEnvironment: []string{"YOU_WORKER_SESSION_TOKEN=worker-credential"},
	}
	text := "safe " + sessionID + " api-credential worker-credential"
	result, err := normalizeSuccess(providers.ExecuteResult{
		Content:     text,
		Diagnostics: &providers.ExecuteDiagnostics{Progress: []providers.ExecuteProgress{{Detail: text}}},
	}, providers.IDCodex, request, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Content != "safe "+sessionID+" <redacted> <redacted>" {
		t.Fatalf("terminal answer corrupted or exposed credentials: %q", result.Content)
	}
	if result.Diagnostics.Progress[0].Detail != "safe <redacted> <redacted> <redacted>" {
		t.Fatalf("diagnostic context exposed: %+v", result.Diagnostics)
	}
}

func TestCapturedFactsPreserveTypedFailureContext(t *testing.T) {
	t.Parallel()
	const id = "8528a10f-6314-48ba-8230-542a4cf660f9"
	for _, kind := range []providers.ExecuteFailureKind{providers.ExecuteFailureKindDependency, providers.ExecuteFailureKindAuthentication, providers.ExecuteFailureKindInvalidRequest, "malformed-kind"} {
		t.Run(string(kind), func(t *testing.T) {
			t.Parallel()
			request := providers.ExecuteRequest{SystemPrompt: "private-request", EnvVars: map[string]string{"API_KEY": "private-key", "VALIDATION_ENVIRONMENT": "1"}}
			var failure providers.ExecuteFailure
			if !errors.As(normalizeDeclaredFailure(providers.ExecuteFailure{Kind: kind, Message: "safe " + id + " private-request private-key\xff"}, request), &failure) {
				t.Fatal("failure lost its typed classification")
			}
			wantKind := kind
			if kind == "malformed-kind" {
				wantKind = providers.ExecuteFailureKindUnknown
			}
			if failure.Kind != wantKind || failure.Message != "safe "+id+" <redacted> <redacted>" {
				t.Fatalf("typed/safe failure changed: %+v", failure)
			}
			var fallback providers.ExecuteFailure
			if !errors.As(normalizeDeclaredFailure(providers.ExecuteFailure{Kind: kind}, request), &fallback) {
				t.Fatal("empty failure lost its typed classification")
			}
			if fallback.Message == "" || fallback.Kind != wantKind {
				t.Fatal("empty failure did not retain safe typed default")
			}
		})
	}
}

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

func TestRecordingContentRedactsInheritedCredentialsWithoutMutatingMetadata(t *testing.T) {
	t.Parallel()
	request := providers.ExecuteRequest{ProcessEnvironment: []string{"API_TOKEN=inherited-secret", "LANG=ordinary", "NO_EQUALS"}}
	metadata := map[string]string{"safe": "inherited-secret ordinary", "authorization": "credential", "input_tokens": "0"}
	progress := sanitizeCapturedProgress(providers.ExecuteProgress{
		Phase: "message.completed", Detail: "inherited-secret ordinary", Metadata: metadata,
	}, request)
	if progress.Detail != "<redacted> ordinary" || progress.Metadata["safe"] != "<redacted> ordinary" ||
		progress.Metadata["authorization"] != redactedValue || progress.Metadata["input_tokens"] != "0" {
		t.Fatalf("inherited credential capture = %+v", progress)
	}
	if metadata["safe"] != "inherited-secret ordinary" || metadata["authorization"] != "credential" {
		t.Fatal("capture sanitizer mutated adapter metadata")
	}
	diagnostic := normalizeDiagnostics(providers.ExecuteDiagnostics{Metadata: metadata}, request)
	if diagnostic.Metadata["safe"] != "<redacted> ordinary" {
		t.Fatal("inherited credential escaped diagnostics")
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

func TestSanitizeCodexToolSummariesPreserveJSONAndPolicy(t *testing.T) {
	t.Parallel()
	request := providers.ExecuteRequest{EnvVars: map[string]string{"API_KEY": "planted-secret"}}
	fact := providers.ExecuteProgress{Phase: "tool.completed", Metadata: map[string]string{
		"raw_input":  `{"command":"private command planted\u002dsecret","arguments":{"query":"planted\u002dsecret","password":"private-password","safe":"visible"}}`,
		"raw_output": `{"aggregated_output":"private-output planted\u002dsecret","exit_code":0,"status":"completed","result":{"value":"planted\u002dsecret","safe":"visible"}}`,
	}}
	live := sanitizeCapturedProgress(fact, request)
	terminal := normalizeDiagnostics(providers.ExecuteDiagnostics{Progress: []providers.ExecuteProgress{fact}}, request).Progress[0]
	for index, got := range []providers.ExecuteProgress{live, terminal} {
		for key, expected := range map[string]string{
			"raw_input":  `{"arguments":{"password":"<redacted>","query":"<redacted>","safe":"visible"},"command":"<redacted>"}`,
			"raw_output": `{"aggregated_output":"<redacted>","exit_code":0,"result":{"safe":"visible","value":"<redacted>"},"status":"completed"}`,
		} {
			if index == 0 {
				expected = strings.ReplaceAll(expected, `"command":"<redacted>"`, `"command":"private command <redacted>"`)
				expected = strings.ReplaceAll(expected, `"aggregated_output":"<redacted>"`, `"aggregated_output":"private-output <redacted>"`)
			}
			var actual, want any
			if json.Unmarshal([]byte(got.Metadata[key]), &actual) != nil || json.Unmarshal([]byte(expected), &want) != nil || !reflect.DeepEqual(actual, want) {
				t.Fatal("summary lost policy-safe typed facts")
			}
		}
	}
	if fact.Metadata["raw_output"] == live.Metadata["raw_output"] {
		t.Fatal("sanitizer aliased native metadata")
	}
}

func TestSanitizeCodexToolSummaryBoundaries(t *testing.T) {
	t.Parallel()
	request := providers.ExecuteRequest{EnvVars: map[string]string{"API_KEY": "planted-secret"}}
	for _, test := range []struct {
		name, key, value, want string
	}{
		{"input array", "raw_input", `["visible","planted-secret",0,null]`, `["visible","<redacted>",0,null]`},
		{"output array", "raw_output", `["private output",0,false]`, `["<redacted>",0,false]`},
		{"output scalar", "raw_output", `"private output"`, `"<redacted>"`},
		{"private reasoning", "raw_input", `{"private_reasoning":"private thought","encrypted_content":"private blob"}`, `{"encrypted_content":"<redacted>","private_reasoning":"<redacted>"}`},
		{"credential input keys", "raw_input", `{"arguments":{"planted\u002dsecret":"private value","prefix-planted-secret":1,"safe":0}}`, `{"arguments":{"safe":0}}`},
		{"credential output key collisions", "raw_output", `{"result":{"planted-secret":"private value","prefix-planted-secret":1,"prefix-<redacted>":2,"<redacted>":3,"nested":[{"planted-secret":false,"safe":true}]},"exit_code":0,"status":"completed"}`, `{"result":{"prefix-<redacted>":2,"<redacted>":3,"nested":[{"safe":true}]},"exit_code":0,"status":"completed"}`},
		{"malformed input", "raw_input", `{"value":"planted-secret"`, `"<redacted>"`},
		{"malformed output", "raw_output", `{"value":"private output"`, `"<redacted>"`},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			fact := providers.ExecuteProgress{Phase: "tool.updated", Metadata: map[string]string{test.key: test.value}}
			live := sanitizeCapturedProgress(fact, request)
			terminal := normalizeDiagnostics(providers.ExecuteDiagnostics{Progress: []providers.ExecuteProgress{fact}}, request).Progress[0]
			for index, got := range []providers.ExecuteProgress{live, terminal} {
				var actual, expected any
				want := test.want
				if index == 0 && (test.name == "output array" || test.name == "output scalar") {
					want = test.value
				}
				matches := got.Metadata[test.key] == want
				if json.Valid([]byte(want)) {
					matches = json.Unmarshal([]byte(got.Metadata[test.key]), &actual) == nil && json.Unmarshal([]byte(want), &expected) == nil && reflect.DeepEqual(actual, expected)
				}
				if !matches {
					t.Fatalf("tool summary boundary changed: got=%q want=%q", got.Metadata[test.key], test.want)
				}
			}
		})
	}
	// Many small safe values exceed the terminal metadata budget without
	// requiring a single large string to be individually truncated.
	fact := providers.ExecuteProgress{Metadata: map[string]string{"raw_output": `[` + strings.Repeat("0,", maxDiagnosticRunes) + `0]`}}
	terminal := normalizeDiagnostics(providers.ExecuteDiagnostics{Progress: []providers.ExecuteProgress{fact}}, request).Progress[0]
	if terminal.Metadata["raw_output"] != `"<redacted>"` || !json.Valid([]byte(terminal.Metadata["raw_output"])) {
		t.Fatal("oversized tool summary lost valid bounded redaction")
	}
}
