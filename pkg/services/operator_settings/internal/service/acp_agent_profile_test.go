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
