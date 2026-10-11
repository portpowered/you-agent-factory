package service_test

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	operatorservice "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/service"
)

func TestRootResolveSubagentRoute(t *testing.T) {
	t.Parallel()
	text := func(value string) *string { return &value }
	for _, tc := range []struct {
		name      string
		baseline  *operatorsettings.SubagentRoute
		overrides operatorsettings.SubagentRouteUpdate
		want      operatorsettings.SubagentRoute
		invalid   bool
	}{
		{name: "missing dedicated default", invalid: true},
		{name: "explicit route without default", overrides: operatorsettings.SubagentRouteUpdate{Provider: text("codex"), Model: text("luna"), ReasoningEffort: text("low")}, want: operatorsettings.SubagentRoute{Provider: "CODEX", Model: "luna", ReasoningEffort: "low"}},
		{name: "model alone without default", overrides: operatorsettings.SubagentRouteUpdate{Model: text("model")}, invalid: true},
		{name: "dedicated default", baseline: &operatorsettings.SubagentRoute{Provider: "opencode", Model: "free/model", ReasoningEffort: "low"}, want: operatorsettings.SubagentRoute{Provider: "OPENCODE", Model: "free/model", ReasoningEffort: "low"}},
		{name: "model and effort override", baseline: &operatorsettings.SubagentRoute{Provider: "opencode", Model: "free/model", ReasoningEffort: "low"}, overrides: operatorsettings.SubagentRouteUpdate{Model: text(" other/model "), ReasoningEffort: text("high")}, want: operatorsettings.SubagentRoute{Provider: "OPENCODE", Model: "other/model", ReasoningEffort: "high"}},
		{name: "same provider alias", baseline: &operatorsettings.SubagentRoute{Provider: "codex", Model: "model"}, overrides: operatorsettings.SubagentRouteUpdate{Provider: text("CODEX")}, want: operatorsettings.SubagentRoute{Provider: "CODEX", Model: "model"}},
		{name: "provider switch requires model", baseline: &operatorsettings.SubagentRoute{Provider: "opencode", Model: "free/model"}, overrides: operatorsettings.SubagentRouteUpdate{Provider: text("claude")}, invalid: true},
		{name: "explicit claude", baseline: &operatorsettings.SubagentRoute{Provider: "opencode", Model: "free/model"}, overrides: operatorsettings.SubagentRouteUpdate{Provider: text("claude"), Model: text("haiku")}, want: operatorsettings.SubagentRoute{Provider: "CLAUDE", Model: "haiku"}},
		{name: "explicit agy", overrides: operatorsettings.SubagentRouteUpdate{Provider: text("agy"), Model: text("model")}, want: operatorsettings.SubagentRoute{Provider: "agy", Model: "model"}},
		{name: "empty explicit model", baseline: &operatorsettings.SubagentRoute{Provider: "codex", Model: "model"}, overrides: operatorsettings.SubagentRouteUpdate{Model: text(" ")}, invalid: true},
		{name: "symbolic provider", overrides: operatorsettings.SubagentRouteUpdate{Provider: text("DEFAULT"), Model: text("model")}, invalid: true},
		{name: "invalid effort", overrides: operatorsettings.SubagentRouteUpdate{Provider: text("codex"), Model: text("model"), ReasoningEffort: text("unknown")}, invalid: true},
		{name: "malformed baseline fails closed", baseline: &operatorsettings.SubagentRoute{Provider: "opencode"}, overrides: operatorsettings.SubagentRouteUpdate{Provider: text("codex"), Model: text("model")}, invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			document := &profileDocument{path: "selected-config", document: operatorsettings.Document{Defaults: operatorsettings.DocumentDefaults{WorkerModelProvider: "CLAUDE", WorkerModel: "generic-paid", Subagent: tc.baseline}}}
			prior := document.document.Clone()
			root := newControlledRoot(t, document, &constructionResolution{})
			got, err := root.ResolveSubagentRoute(t.Context(), document.path, tc.overrides)
			if tc.invalid {
				if !errors.Is(err, operatorsettings.ErrResolutionInvalidInput) || got != (operatorsettings.SubagentRoute{}) {
					t.Fatalf("resolve = %#v, %v", got, err)
				}
			} else if err != nil || got != tc.want {
				t.Fatalf("resolve = %#v, %v; want %#v", got, err, tc.want)
			}
			got.Model = "mutated"
			if document.published || !reflect.DeepEqual(document.document, prior) {
				t.Fatal("resolution mutated or persisted operator settings")
			}
		})
	}
}

func TestRootResolveSubagentRouteRejectsCanceledAndFailedLoad(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	root := newControlledRoot(t, &constructionDocument{}, &constructionResolution{})
	if _, err := root.ResolveSubagentRoute(ctx, "selected-config", operatorsettings.SubagentRouteUpdate{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled request = %v", err)
	}
	//nolint:staticcheck // Verify that missing caller context is rejected before document effects.
	if _, err := root.ResolveSubagentRoute(nil, "selected-config", operatorsettings.SubagentRouteUpdate{}); !errors.Is(err, operatorsettings.ErrResolutionInvalidInput) {
		t.Fatalf("nil context = %v", err)
	}
	loadError := operatorsettings.DocumentFailure{Kind: operatorsettings.DocumentFailureKindMalformed}
	document := &profileDocument{path: "selected-config", loadError: loadError}
	root = newControlledRoot(t, document, &constructionResolution{})
	if _, err := root.ResolveSubagentRoute(t.Context(), document.path, operatorsettings.SubagentRouteUpdate{}); !errors.Is(err, operatorsettings.ErrDocumentMalformed) {
		t.Fatalf("load failure = %v", err)
	}
}

func TestRootResolveSubagentRouteLogsOutcomeWithoutConfiguration(t *testing.T) {
	t.Parallel()
	spy := &spyLogger{}
	document := &profileDocument{path: "secret-config-path", document: operatorsettings.Document{Defaults: operatorsettings.DocumentDefaults{Subagent: &operatorsettings.SubagentRoute{Provider: "opencode", Model: "secret-model"}}}}
	root, err := operatorservice.New(document, &constructionResolution{}, rootTestFileSystem{}, rootTestCreateTemporaryFile, rootTestConfigDecoder, rootTestConfigEncoder, rootTestIDGenerator, spy, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := root.ResolveSubagentRoute(t.Context(), document.path, operatorsettings.SubagentRouteUpdate{}); err != nil {
		t.Fatal(err)
	}
	document.loadError = operatorsettings.DocumentFailure{Kind: operatorsettings.DocumentFailureKindMalformed, Message: "secret-authored-value"}
	if _, err := root.ResolveSubagentRoute(t.Context(), document.path, operatorsettings.SubagentRouteUpdate{}); !errors.Is(err, operatorsettings.ErrDocumentMalformed) {
		t.Fatalf("load error = %v", err)
	}
	want := []string{"operator_settings.resolve_subagent_route.started", "operator_settings.resolve_subagent_route.finished", "operator_settings.resolve_subagent_route.started", "operator_settings.resolve_subagent_route.failed"}
	if !reflect.DeepEqual(spy.messages(), want) {
		t.Fatalf("logs = %v, want %v", spy.messages(), want)
	}
	assertNoSensitiveValuesLogged(t, spy, document.path, "secret-model", "secret-authored-value")
}

func TestRootResolveACPAgentProfile_DefaultIsReadOnly(t *testing.T) {
	t.Parallel()
	document := &profileDocument{path: "selected-config"}
	root := newControlledRoot(t, document, &constructionResolution{})
	resolved, err := root.ResolveACPAgentProfile(document.path)
	if err != nil || !reflect.DeepEqual(resolved, operatorsettings.DefaultACPAgentProfile()) {
		t.Fatalf("resolve = %#v, %v", resolved, err)
	}
	if document.published {
		t.Fatal("resolve published an absent profile")
	}
}

func TestRootResolveACPAgentProfile_ValidatesAndDetachesAuthoredProfile(t *testing.T) {
	t.Parallel()
	for _, invalid := range []bool{false, true} {
		t.Run(map[bool]string{false: "valid", true: "invalid"}[invalid], func(t *testing.T) {
			t.Parallel()
			profile := operatorsettings.ACPAgentProfile{DefaultTarget: " factory:@you/reviewer ", AllowedTargets: []string{" factory:@you/reviewer ", "factory:@you/factory-builder"}}
			if invalid {
				profile.DefaultTarget = "factory:@you/missing"
			}
			document := &profileDocument{path: "selected-config", document: operatorsettings.Document{Workers: operatorsettings.DocumentWorkerSettings{ACP: operatorsettings.DocumentACPSettings{AgentProfile: &profile}}}}
			root := newControlledRoot(t, document, &constructionResolution{})
			resolved, err := root.ResolveACPAgentProfile(document.path)
			if invalid {
				if !errors.Is(err, operatorsettings.ErrACPAgentProfileInvalid) {
					t.Fatalf("invalid profile = %v", err)
				}
			} else {
				if err != nil || resolved.DefaultTarget != "factory:@you/reviewer" || len(resolved.AllowedTargets) != 2 {
					t.Fatalf("resolve = %#v, %v", resolved, err)
				}
				resolved.AllowedTargets[0] = "factory:@you/mutated"
				if profile.AllowedTargets[0] != " factory:@you/reviewer " {
					t.Fatal("resolved profile aliases loaded document")
				}
			}
			if document.published {
				t.Fatal("resolve published a profile")
			}
		})
	}
}

func TestRootUpdateACPAgentProfile_RejectsInvalidAndCanceledCandidatesBeforeOwnerCalls(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, tc := range []struct {
		name    string
		ctx     context.Context
		profile operatorsettings.ACPAgentProfile
		want    error
	}{
		{"invalid", context.Background(), operatorsettings.ACPAgentProfile{DefaultTarget: "not-a-factory-reference", AllowedTargets: []string{"factory:@you/reviewer"}}, operatorsettings.ErrACPAgentProfileInvalid},
		{"canceled", ctx, operatorsettings.DefaultACPAgentProfile(), context.Canceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Any load or persistence call on this collaborator fails the test by panicking.
			root := newControlledRoot(t, &constructionDocument{}, &constructionResolution{})
			_, err := root.UpdateACPAgentProfile(tc.ctx, "selected-config", tc.profile)
			if !errors.Is(err, tc.want) {
				t.Fatalf("update = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestRootUpdateACPAgentProfile_PreservesUnrelatedSettingsAndDetachesCandidate(t *testing.T) {
	t.Parallel()
	integration := operatorsettings.ACPIntegration{ID: "entry-1", Name: "cursor-acp", Transport: "stdio", Command: "cursor-agent acp"}
	document := &profileDocument{path: "selected-config", document: operatorsettings.Document{
		BackendScopeID: "local-11111111-1111-4111-8111-111111111111",
		Defaults:       operatorsettings.DocumentDefaults{WorkerModelProvider: "CODEX", WorkerModel: "gpt-5"},
		Workers:        operatorsettings.DocumentWorkerSettings{ACP: operatorsettings.DocumentACPSettings{Integrations: []operatorsettings.ACPIntegration{integration}}},
	}}
	prior := document.document.Clone()
	root := newControlledRoot(t, document, &constructionResolution{})
	profile := operatorsettings.ACPAgentProfile{DefaultTarget: " factory:@you/reviewer ", AllowedTargets: []string{" factory:@you/reviewer "}}
	updated, err := root.UpdateACPAgentProfile(context.Background(), document.path, profile)
	if err != nil || !document.published || updated.DefaultTarget != "factory:@you/reviewer" {
		t.Fatalf("update = %#v, %v", updated, err)
	}
	if document.document.BackendScopeID != prior.BackendScopeID || document.document.Defaults != prior.Defaults || !reflect.DeepEqual(document.document.Workers.ACP.Integrations, prior.Workers.ACP.Integrations) {
		t.Fatalf("unrelated settings changed: %#v", document.document)
	}
	profile.AllowedTargets[0] = "factory:@you/input-mutated"
	updated.AllowedTargets[0] = "factory:@you/output-mutated"
	if document.document.Workers.ACP.AgentProfile.AllowedTargets[0] != "factory:@you/reviewer" {
		t.Fatal("candidate or result aliases the published profile")
	}
}

func TestRootACPIntegrationUpdates_PreserveAuthoredACPAgentProfile(t *testing.T) {
	t.Parallel()
	profile := operatorsettings.DefaultACPAgentProfile()
	document := &profileDocument{path: "selected-config", document: operatorsettings.Document{Workers: operatorsettings.DocumentWorkerSettings{ACP: operatorsettings.DocumentACPSettings{AgentProfile: &profile}}}}
	root := newControlledRoot(t, document, &constructionResolution{})
	integration := operatorsettings.ACPIntegration{ID: "entry-1", Name: "cursor-acp", Transport: "stdio", Command: "cursor-agent acp"}
	if _, err := root.ConfigureACPIntegrationAdd(context.Background(), document.path, integration); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(document.document.Workers.ACP.AgentProfile, &profile) {
		t.Fatal("integration update dropped profile")
	}
}

func TestRootACPAgentProfileLogsSuccessAndTypedFailureSafely(t *testing.T) {
	t.Parallel()
	for _, update := range []bool{false, true} {
		for _, fail := range []bool{false, true} {
			name := map[bool]string{false: "resolve", true: "update"}[update] + map[bool]string{false: " success", true: " failure"}[fail]
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				spy := &spyLogger{}
				document := &profileDocument{path: "secret-config-path"}
				if fail {
					document.loadError = operatorsettings.DocumentFailure{Kind: operatorsettings.DocumentFailureKindMalformed, Message: "secret-profile-value"}
				}
				root, err := operatorservice.New(document, &constructionResolution{}, rootTestFileSystem{}, rootTestCreateTemporaryFile, rootTestConfigDecoder, rootTestConfigEncoder, rootTestIDGenerator, spy, nil)
				if err != nil {
					t.Fatal(err)
				}
				operation := "resolve_acp_agent_profile"
				if update {
					operation = "update_acp_agent_profile"
					_, err = root.UpdateACPAgentProfile(context.Background(), document.path, operatorsettings.DefaultACPAgentProfile())
				} else {
					_, err = root.ResolveACPAgentProfile(document.path)
				}
				outcome := "finished"
				if fail {
					outcome = "failed"
					if !errors.Is(err, document.loadError) || !containsKeyValue(spy, "reason", "document_malformed") {
						t.Fatalf("failure = %v; logs=%#v", err, spy.entries)
					}
				} else if err != nil {
					t.Fatal(err)
				}
				want := []string{"operator_settings." + operation + ".started", "operator_settings." + operation + "." + outcome}
				if !reflect.DeepEqual(spy.messages(), want) {
					t.Fatalf("logs=%v, want %v", spy.messages(), want)
				}
				assertNoSensitiveValuesLogged(t, spy, document.path, "secret-profile-value")
			})
		}
	}
}

func TestRootUpdateACPAgentProfile_LogsInvalidCandidateSafely(t *testing.T) {
	t.Parallel()
	spy := &spyLogger{}
	root, err := operatorservice.New(&constructionDocument{}, &constructionResolution{}, rootTestFileSystem{}, rootTestCreateTemporaryFile, rootTestConfigDecoder, rootTestConfigEncoder, rootTestIDGenerator, spy, nil)
	if err != nil {
		t.Fatal(err)
	}
	profile := operatorsettings.ACPAgentProfile{DefaultTarget: "secret-invalid-target", AllowedTargets: []string{"factory:@you/reviewer"}}
	if _, err := root.UpdateACPAgentProfile(context.Background(), "secret-config-path", profile); !errors.Is(err, operatorsettings.ErrACPAgentProfileInvalid) {
		t.Fatalf("update = %v", err)
	}
	if !containsKeyValue(spy, "reason", "profile_invalid") {
		t.Fatal("missing invalid-profile reason")
	}
	assertNoSensitiveValuesLogged(t, spy, "secret-config-path", profile.DefaultTarget, profile.AllowedTargets[0])
}

// spyLoggedEntry captures one structured log call for assertion.
type spyLoggedEntry struct {
	level string
	msg   string
	kv    []any
}

// spyLogger is a logging.Logger fake that records every call so tests can
// assert on operation-log shape (message names, safe fields) without a real
// logging backend.
type spyLogger struct {
	mu      sync.Mutex
	entries []spyLoggedEntry
}

func (s *spyLogger) Debug(msg string, kv ...any)   { s.record("debug", msg, kv) }
func (s *spyLogger) Info(msg string, kv ...any)    { s.record("info", msg, kv) }
func (s *spyLogger) Warn(msg string, kv ...any)    { s.record("warn", msg, kv) }
func (s *spyLogger) Error(msg string, kv ...any)   { s.record("error", msg, kv) }
func (s *spyLogger) Verbose(msg string, kv ...any) { s.record("verbose", msg, kv) }

func (s *spyLogger) record(level, msg string, kv []any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = append(s.entries, spyLoggedEntry{level: level, msg: msg, kv: append([]any(nil), kv...)})
}

func (s *spyLogger) messages() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.entries))
	for index, entry := range s.entries {
		out[index] = entry.msg
	}
	return out
}

func containsString(values []string, want string) bool {
	return slices.Contains(values, want)
}

// containsKeyValue reports whether any recorded entry has key immediately
// followed by want in its key/value pairs.
func containsKeyValue(spy *spyLogger, key string, want any) bool {
	spy.mu.Lock()
	defer spy.mu.Unlock()
	for _, entry := range spy.entries {
		for index := 0; index+1 < len(entry.kv); index += 2 {
			if entry.kv[index] == key && reflect.DeepEqual(entry.kv[index+1], want) {
				return true
			}
		}
	}
	return false
}

// assertNoSensitiveValuesLogged fails the test if any recorded log message or
// key/value field contains the config path or any other forbidden value
// (profile target references, raw configuration contents).
func assertNoSensitiveValuesLogged(t *testing.T, spy *spyLogger, forbidden ...string) {
	t.Helper()
	spy.mu.Lock()
	defer spy.mu.Unlock()
	for _, entry := range spy.entries {
		if containsForbiddenSubstring(entry.msg, forbidden) {
			t.Fatalf("log message %q leaked a forbidden value from %v", entry.msg, forbidden)
		}
		for _, value := range entry.kv {
			text, ok := value.(string)
			if !ok {
				continue
			}
			if containsForbiddenSubstring(text, forbidden) {
				t.Fatalf("log field %q leaked a forbidden value from %v", text, forbidden)
			}
		}
	}
}

func containsForbiddenSubstring(value string, forbidden []string) bool {
	for _, needle := range forbidden {
		if needle != "" && strings.Contains(value, needle) {
			return true
		}
	}
	return false
}
