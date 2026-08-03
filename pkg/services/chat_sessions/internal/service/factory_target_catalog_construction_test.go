package service_test

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	chatsessionsservice "github.com/portpowered/infinite-you/pkg/services/chat_sessions/internal/service"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

// spyLoggedEntry captures one structured log call for assertion.
type spyLoggedEntry struct {
	level string
	msg   string
	kv    []any
}

// spyLogger is a logging.Logger fake that records every call so tests can
// assert on operation-log shape (message names, safe fields) without a real
// logging backend. Mirrors the equivalent fake in
// pkg/services/operator_settings/internal/service.
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

func (s *spyLogger) containsMessage(want string) bool {
	return slices.Contains(s.messages(), want)
}

func (s *spyLogger) containsKeyValue(key string, want any) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, entry := range s.entries {
		for index := 0; index+1 < len(entry.kv); index += 2 {
			if entry.kv[index] == key && entry.kv[index+1] == want {
				return true
			}
		}
	}
	return false
}

// assertNoForbiddenValuesLogged fails the test if any recorded log message or
// key/value field contains one of the forbidden substrings (raw targets,
// paths, or other sensitive facts the operation must never log).
func (s *spyLogger) assertNoForbiddenValuesLogged(t *testing.T, forbidden ...string) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, entry := range s.entries {
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

func TestResolveFactoryTargetCatalogLogsStartedAndFinishedSafely(t *testing.T) {
	t.Parallel()

	spy := &spyLogger{}
	settings := &operatorSettingsFake{
		resolveACPAgentProfile: func(string) (operatorsettings.ACPAgentProfile, error) {
			return operatorsettings.ACPAgentProfile{
				DefaultTarget:  "factory:@you/factory-builder",
				AllowedTargets: []string{"factory:@you/factory-builder"},
			}, nil
		},
	}
	definitions := &factoryDefinitionsFake{
		listEffectiveFactories: func(context.Context, factorydefinitions.ListEffectiveFactoriesRequest) (factorydefinitions.ListEffectiveFactoriesResult, error) {
			return factorydefinitions.ListEffectiveFactoriesResult{
				Entries: []factorydefinitions.EffectiveFactoryCatalogEntry{
					installedFactoryEntry("@you/factory-builder", "Factory Builder"),
				},
			}, nil
		},
	}
	service, err := chatsessionsservice.New(settings, definitions, spy)
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}

	operatorSettingsPath := "/very/private/operator-settings.json"
	if _, err := service.ResolveFactoryTargetCatalog(context.Background(), chatsessions.ResolveFactoryTargetCatalogRequest{
		OperatorSettingsPath: operatorSettingsPath,
	}); err != nil {
		t.Fatalf("ResolveFactoryTargetCatalog: unexpected error: %v", err)
	}

	if !spy.containsMessage("chat_sessions.resolve_factory_target_catalog.started") {
		t.Fatalf("log messages = %v, want a start log", spy.messages())
	}
	if !spy.containsMessage("chat_sessions.resolve_factory_target_catalog.finished") {
		t.Fatalf("log messages = %v, want a finished log", spy.messages())
	}
	if !spy.containsKeyValue("choice_count", 1) {
		t.Fatalf("log entries = %#v, want choice_count=1", spy.entries)
	}
	spy.assertNoForbiddenValuesLogged(t, operatorSettingsPath, "factory:@you/factory-builder")
}

func TestResolveFactoryTargetCatalogLogsFailureReasonWithoutLeakingValues(t *testing.T) {
	t.Parallel()

	spy := &spyLogger{}
	settings := &operatorSettingsFake{
		resolveACPAgentProfile: func(string) (operatorsettings.ACPAgentProfile, error) {
			return operatorsettings.ACPAgentProfile{
				DefaultTarget:  "factory:@you/factory-builder",
				AllowedTargets: []string{"factory:@you/factory-builder"},
			}, nil
		},
	}
	definitions := &factoryDefinitionsFake{
		listEffectiveFactories: func(context.Context, factorydefinitions.ListEffectiveFactoriesRequest) (factorydefinitions.ListEffectiveFactoriesResult, error) {
			// No installed Factories: the allowed/installed intersection is
			// empty, so resolution fails with ErrFactoryTargetCatalogEmpty.
			return factorydefinitions.ListEffectiveFactoriesResult{}, nil
		},
	}
	service, err := chatsessionsservice.New(settings, definitions, spy)
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}

	operatorSettingsPath := "/very/private/operator-settings.json"
	_, err = service.ResolveFactoryTargetCatalog(context.Background(), chatsessions.ResolveFactoryTargetCatalogRequest{
		OperatorSettingsPath: operatorSettingsPath,
	})
	if err == nil {
		t.Fatal("ResolveFactoryTargetCatalog: error = nil, want ErrFactoryTargetCatalogEmpty")
	}

	if !spy.containsMessage("chat_sessions.resolve_factory_target_catalog.failed") {
		t.Fatalf("log messages = %v, want a failed log", spy.messages())
	}
	if !spy.containsKeyValue("reason", "catalog_empty") {
		t.Fatalf("log entries = %#v, want reason=catalog_empty", spy.entries)
	}
	spy.assertNoForbiddenValuesLogged(t, operatorSettingsPath, "factory:@you/factory-builder")
}

func TestResolveFactoryTargetCatalogUsesEachInjectedCollaboratorExactlyOnce(t *testing.T) {
	t.Parallel()

	var profileCalls, catalogCalls int
	settings := &operatorSettingsFake{
		resolveACPAgentProfile: func(string) (operatorsettings.ACPAgentProfile, error) {
			profileCalls++
			return operatorsettings.ACPAgentProfile{
				DefaultTarget:  "factory:@you/factory-builder",
				AllowedTargets: []string{"factory:@you/factory-builder"},
			}, nil
		},
	}
	definitions := &factoryDefinitionsFake{
		listEffectiveFactories: func(context.Context, factorydefinitions.ListEffectiveFactoriesRequest) (factorydefinitions.ListEffectiveFactoriesResult, error) {
			catalogCalls++
			return factorydefinitions.ListEffectiveFactoriesResult{
				Entries: []factorydefinitions.EffectiveFactoryCatalogEntry{
					installedFactoryEntry("@you/factory-builder", "Factory Builder"),
				},
			}, nil
		},
	}
	service, err := chatsessionsservice.New(settings, definitions, nil)
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}

	if _, err := service.ResolveFactoryTargetCatalog(context.Background(), chatsessions.ResolveFactoryTargetCatalogRequest{
		OperatorSettingsPath: "/operator.json",
	}); err != nil {
		t.Fatalf("ResolveFactoryTargetCatalog: unexpected error: %v", err)
	}

	if profileCalls != 1 {
		t.Fatalf("operator settings ResolveACPAgentProfile call count = %d, want exactly 1", profileCalls)
	}
	if catalogCalls != 1 {
		t.Fatalf("factory definitions ListEffectiveFactories call count = %d, want exactly 1", catalogCalls)
	}
}

// TestResolveFactoryTargetCatalogObservesLiveCollaboratorDrift proves the
// operation never caches a prior resolution: the same long-lived Service
// instance reflects an installation change on its very next call.
func TestResolveFactoryTargetCatalogObservesLiveCollaboratorDrift(t *testing.T) {
	t.Parallel()

	installed := true
	settings := &operatorSettingsFake{
		resolveACPAgentProfile: func(string) (operatorsettings.ACPAgentProfile, error) {
			return operatorsettings.ACPAgentProfile{
				DefaultTarget:  "factory:@you/factory-builder",
				AllowedTargets: []string{"factory:@you/factory-builder"},
			}, nil
		},
	}
	definitions := &factoryDefinitionsFake{
		listEffectiveFactories: func(context.Context, factorydefinitions.ListEffectiveFactoriesRequest) (factorydefinitions.ListEffectiveFactoriesResult, error) {
			if !installed {
				return factorydefinitions.ListEffectiveFactoriesResult{}, nil
			}
			return factorydefinitions.ListEffectiveFactoriesResult{
				Entries: []factorydefinitions.EffectiveFactoryCatalogEntry{
					installedFactoryEntry("@you/factory-builder", "Factory Builder"),
				},
			}, nil
		},
	}
	service, err := chatsessionsservice.New(settings, definitions, nil)
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}
	req := chatsessions.ResolveFactoryTargetCatalogRequest{OperatorSettingsPath: "/operator.json"}

	if _, err := service.ResolveFactoryTargetCatalog(context.Background(), req); err != nil {
		t.Fatalf("ResolveFactoryTargetCatalog (installed) = %v, want success", err)
	}

	installed = false
	_, err = service.ResolveFactoryTargetCatalog(context.Background(), req)
	if err == nil {
		t.Fatal("ResolveFactoryTargetCatalog (uninstalled) = nil error, want the drift to be observed on the very next call")
	}
}
