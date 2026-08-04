package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	chatsessionsservice "github.com/portpowered/infinite-you/pkg/services/chat_sessions/internal/service"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

var errCollaboratorUnavailable = errors.New("collaborator unavailable")

func TestResolveFactoryTargetCatalogRejectsEmptyProfile(t *testing.T) {
	t.Parallel()

	settings := &operatorSettingsFake{
		resolveACPAgentProfile: func(string) (operatorsettings.ACPAgentProfile, error) {
			return operatorsettings.ACPAgentProfile{}, errCollaboratorUnavailable
		},
	}
	definitions := &factoryDefinitionsFake{}
	service, err := chatsessionsservice.New(settings, definitions, logging.NoopLogger{})
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}

	_, err = service.ResolveFactoryTargetCatalog(context.Background(), chatsessions.ResolveFactoryTargetCatalogRequest{
		OperatorSettingsPath: "/operator.json",
	})
	assertFactoryTargetCatalogError(t, err, chatsessions.ErrFactoryTargetProfileUnavailable, "")
	if errors.Is(err, errCollaboratorUnavailable) {
		t.Fatalf("error unwraps to the raw collaborator error, leaking internal detail: %v", err)
	}
}

func TestResolveFactoryTargetCatalogRejectsInvalidProfileNormalization(t *testing.T) {
	t.Parallel()

	// A default absent from its own allowlist fails Operator Settings'
	// Normalize while resolving the profile; this proves that failure
	// surfaces as a typed Chat Sessions profile failure, not a raw error.
	settings := &operatorSettingsFake{
		resolveACPAgentProfile: func(string) (operatorsettings.ACPAgentProfile, error) {
			profile := operatorsettings.ACPAgentProfile{
				DefaultTarget:  "factory:@you/review",
				AllowedTargets: []string{"factory:@you/factory-builder"},
			}
			return profile.Normalize()
		},
	}
	definitions := &factoryDefinitionsFake{}
	service, err := chatsessionsservice.New(settings, definitions, logging.NoopLogger{})
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}

	_, err = service.ResolveFactoryTargetCatalog(context.Background(), chatsessions.ResolveFactoryTargetCatalogRequest{
		OperatorSettingsPath: "/operator.json",
	})
	assertFactoryTargetCatalogError(t, err, chatsessions.ErrFactoryTargetProfileUnavailable, "")
}

func TestResolveFactoryTargetCatalogRejectsMalformedCurrentTarget(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		target string
	}{
		{name: "missing namespace", target: "@you/factory-builder"},
		{name: "internal whitespace", target: "factory:@you/bad ref"},
		{name: "version pinned", target: "factory:@you/factory-builder@1.2.3"},
		{name: "digest pinned", target: "factory:@you/factory-builder@sha256:abcdef0123456789"},
	}

	profile := operatorsettings.ACPAgentProfile{
		DefaultTarget:  "factory:@you/factory-builder",
		AllowedTargets: []string{"factory:@you/factory-builder"},
	}
	entries := []factorydefinitions.EffectiveFactoryCatalogEntry{
		installedFactoryEntry("@you/factory-builder", "Factory Builder"),
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			service := newTestService(t, profile, entries)

			_, err := service.ResolveFactoryTargetCatalog(context.Background(), chatsessions.ResolveFactoryTargetCatalogRequest{
				OperatorSettingsPath: "/operator.json",
				CurrentTarget:        testCase.target,
			})
			// A malformed CurrentTarget never appears in the public error's
			// Target field or rendered message: the value has not passed
			// lexical validation and may itself be unsafe caller-supplied
			// input (a path, credential-like value, or control text).
			assertFactoryTargetCatalogError(t, err, chatsessions.ErrFactoryTargetReferenceMalformed, "")
			if strings.Contains(err.Error(), testCase.target) {
				t.Fatalf("error message %q echoes the raw malformed CurrentTarget %q", err.Error(), testCase.target)
			}
		})
	}
}

func TestResolveFactoryTargetCatalogMalformedCurrentTargetNeverLeaksHostileInput(t *testing.T) {
	t.Parallel()

	profile := operatorsettings.ACPAgentProfile{
		DefaultTarget:  "factory:@you/factory-builder",
		AllowedTargets: []string{"factory:@you/factory-builder"},
	}
	entries := []factorydefinitions.EffectiveFactoryCatalogEntry{
		installedFactoryEntry("@you/factory-builder", "Factory Builder"),
	}

	cases := []struct {
		name   string
		target string
	}{
		{name: "filesystem path", target: "/etc/passwd"},
		{name: "windows filesystem path", target: `C:\Users\victim\.ssh\id_rsa`},
		{name: "credential-like value", target: "factory:token=sk-live-abcdef0123456789"},
		{name: "control characters", target: "factory:@you/bad\x00\x1bref"},
		{name: "shell metacharacters", target: "factory:@you/x`rm -rf /`"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			service := newTestService(t, profile, entries)

			_, err := service.ResolveFactoryTargetCatalog(context.Background(), chatsessions.ResolveFactoryTargetCatalogRequest{
				OperatorSettingsPath: "/operator.json",
				CurrentTarget:        testCase.target,
			})
			assertFactoryTargetCatalogError(t, err, chatsessions.ErrFactoryTargetReferenceMalformed, "")
			if strings.Contains(err.Error(), testCase.target) {
				t.Fatalf("error message %q echoes the supplied hostile input %q", err.Error(), testCase.target)
			}
		})
	}
}

func TestResolveFactoryTargetCatalogRejectsEmptyEffectiveCatalog(t *testing.T) {
	t.Parallel()

	profile := operatorsettings.ACPAgentProfile{
		DefaultTarget:  "factory:@you/factory-builder",
		AllowedTargets: []string{"factory:@you/factory-builder"},
	}
	// Nothing installed: the allowlist and installed catalog share no target.
	service := newTestService(t, profile, nil)

	_, err := service.ResolveFactoryTargetCatalog(context.Background(), chatsessions.ResolveFactoryTargetCatalogRequest{
		OperatorSettingsPath: "/operator.json",
	})
	assertFactoryTargetCatalogError(t, err, chatsessions.ErrFactoryTargetCatalogEmpty, "")
}

func TestResolveFactoryTargetCatalogRejectsUnknownUninstalledTarget(t *testing.T) {
	t.Parallel()

	profile := operatorsettings.ACPAgentProfile{
		DefaultTarget:  "factory:@you/factory-builder",
		AllowedTargets: []string{"factory:@you/factory-builder", "factory:@you/ghost"},
	}
	entries := []factorydefinitions.EffectiveFactoryCatalogEntry{
		installedFactoryEntry("@you/factory-builder", "Factory Builder"),
	}
	service := newTestService(t, profile, entries)

	_, err := service.ResolveFactoryTargetCatalog(context.Background(), chatsessions.ResolveFactoryTargetCatalogRequest{
		OperatorSettingsPath: "/operator.json",
		CurrentTarget:        "factory:@you/ghost",
	})
	assertFactoryTargetCatalogError(t, err, chatsessions.ErrFactoryTargetNotInstalled, "factory:@you/ghost")
}

// TestResolveFactoryTargetCatalogRejectsUnmaterializedPackagedDefault proves
// a packaged Factory definition that has not been materialized to a
// filesystem location (Location == nil) is never treated as installed, even
// when it is the configured default and the only allowed/effective entry.
func TestResolveFactoryTargetCatalogRejectsUnmaterializedPackagedDefault(t *testing.T) {
	t.Parallel()

	profile := operatorsettings.ACPAgentProfile{
		DefaultTarget:  "factory:@you/factory-builder",
		AllowedTargets: []string{"factory:@you/factory-builder"},
	}
	entries := []factorydefinitions.EffectiveFactoryCatalogEntry{
		packagedOnlyFactoryEntry("@you/factory-builder", "Factory Builder"),
	}
	service := newTestService(t, profile, entries)

	result, err := service.ResolveFactoryTargetCatalog(context.Background(), chatsessions.ResolveFactoryTargetCatalogRequest{
		OperatorSettingsPath: "/operator.json",
	})
	assertFactoryTargetCatalogError(t, err, chatsessions.ErrFactoryTargetCatalogEmpty, "")
	if len(result.Choices) != 0 || result.CurrentTarget != "" {
		t.Fatalf("ResolveFactoryTargetCatalog: result = %#v, want a zero (non-partial) result on failure", result)
	}
}

// TestResolveFactoryTargetCatalogExcludesUnmaterializedPackagedEntryFromChoices
// proves a packaged-only entry never appears as a selectable choice even
// when a separate materialized entry keeps the overall resolution
// successful.
func TestResolveFactoryTargetCatalogExcludesUnmaterializedPackagedEntryFromChoices(t *testing.T) {
	t.Parallel()

	profile := operatorsettings.ACPAgentProfile{
		DefaultTarget:  "factory:@you/factory-builder",
		AllowedTargets: []string{"factory:@you/factory-builder", "factory:@you/packaged-only"},
	}
	entries := []factorydefinitions.EffectiveFactoryCatalogEntry{
		installedFactoryEntry("@you/factory-builder", "Factory Builder"),
		packagedOnlyFactoryEntry("@you/packaged-only", "Packaged Only"),
	}
	service := newTestService(t, profile, entries)

	result, err := service.ResolveFactoryTargetCatalog(context.Background(), chatsessions.ResolveFactoryTargetCatalogRequest{
		OperatorSettingsPath: "/operator.json",
	})
	if err != nil {
		t.Fatalf("ResolveFactoryTargetCatalog: unexpected error: %v", err)
	}
	if len(result.Choices) != 1 || result.Choices[0].Value != "factory:@you/factory-builder" {
		t.Fatalf("Choices = %+v, want only the materialized installed target", result.Choices)
	}
}

func TestResolveFactoryTargetCatalogRejectsRequestedTargetOutsideAllowlist(t *testing.T) {
	t.Parallel()

	profile := operatorsettings.ACPAgentProfile{
		DefaultTarget:  "factory:@you/factory-builder",
		AllowedTargets: []string{"factory:@you/factory-builder"},
	}
	entries := []factorydefinitions.EffectiveFactoryCatalogEntry{
		installedFactoryEntry("@you/factory-builder", "Factory Builder"),
		installedFactoryEntry("@you/review", "Review"),
	}
	service := newTestService(t, profile, entries)

	_, err := service.ResolveFactoryTargetCatalog(context.Background(), chatsessions.ResolveFactoryTargetCatalogRequest{
		OperatorSettingsPath: "/operator.json",
		CurrentTarget:        "factory:@you/review",
	})
	assertFactoryTargetCatalogError(t, err, chatsessions.ErrFactoryTargetNotAllowed, "factory:@you/review")
}

func TestResolveFactoryTargetCatalogRejectsUninstalledTargetAfterPriorSuccess(t *testing.T) {
	t.Parallel()

	profile := operatorsettings.ACPAgentProfile{
		DefaultTarget:  "factory:@you/factory-builder",
		AllowedTargets: []string{"factory:@you/factory-builder"},
	}
	installed := []factorydefinitions.EffectiveFactoryCatalogEntry{
		installedFactoryEntry("@you/factory-builder", "Factory Builder"),
	}

	settings := &operatorSettingsFake{
		resolveACPAgentProfile: func(string) (operatorsettings.ACPAgentProfile, error) { return profile, nil },
	}
	definitions := &factoryDefinitionsFake{
		listEffectiveFactories: func(context.Context, factorydefinitions.ListEffectiveFactoriesRequest) (factorydefinitions.ListEffectiveFactoriesResult, error) {
			return factorydefinitions.ListEffectiveFactoriesResult{Entries: installed}, nil
		},
	}
	service, err := chatsessionsservice.New(settings, definitions, logging.NoopLogger{})
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}

	first, err := service.ResolveFactoryTargetCatalog(context.Background(), chatsessions.ResolveFactoryTargetCatalogRequest{
		OperatorSettingsPath: "/operator.json",
	})
	if err != nil {
		t.Fatalf("first ResolveFactoryTargetCatalog: unexpected error: %v", err)
	}
	if first.CurrentTarget != "factory:@you/factory-builder" {
		t.Fatalf("first CurrentTarget = %q, want %q", first.CurrentTarget, "factory:@you/factory-builder")
	}

	// The Factory is uninstalled between calls; nothing is cached from the
	// prior successful resolution, so the next call observes the change live.
	installed = nil

	_, err = service.ResolveFactoryTargetCatalog(context.Background(), chatsessions.ResolveFactoryTargetCatalogRequest{
		OperatorSettingsPath: "/operator.json",
	})
	assertFactoryTargetCatalogError(t, err, chatsessions.ErrFactoryTargetCatalogEmpty, "")
}

func TestResolveFactoryTargetCatalogRejectsIncompatiblePinnedWorkingRoot(t *testing.T) {
	t.Parallel()

	profile := operatorsettings.ACPAgentProfile{
		DefaultTarget:  "factory:@you/factory-builder",
		AllowedTargets: []string{"factory:@you/factory-builder"},
	}
	entries := []factorydefinitions.EffectiveFactoryCatalogEntry{
		installedFactoryEntry("@you/factory-builder", "Factory Builder"),
	}

	settings := &operatorSettingsFake{
		resolveACPAgentProfile: func(string) (operatorsettings.ACPAgentProfile, error) { return profile, nil },
	}
	definitions := &factoryDefinitionsFake{
		listEffectiveFactories: func(context.Context, factorydefinitions.ListEffectiveFactoriesRequest) (factorydefinitions.ListEffectiveFactoriesResult, error) {
			return factorydefinitions.ListEffectiveFactoriesResult{Entries: entries}, nil
		},
		resolveNamedFactory: func(context.Context, factorydefinitions.ResolveNamedFactoryRequest) (factorydefinitions.ResolveNamedFactoryResult, error) {
			return factorydefinitions.ResolveNamedFactoryResult{
				Resolution: factorydefinitions.NamedFactoryResolution{
					Name:   "@you/factory-builder",
					Source: factorydefinitions.NamedFactoryResolutionSourceProjectLocal,
					// ProjectRoot always denotes a project-scoped Factory
					// root derived via factorydefinitions.ProjectFactoriesRoot
					// ("<workingDir>/factory"), matching what production
					// callers (see factorydefinitions.ResolveNamedFactory)
					// echo back -- not a bare working directory.
					ProjectRoot: factorydefinitions.ProjectFactoriesRoot("/repos/project-a"),
				},
			}, nil
		},
	}
	service, err := chatsessionsservice.New(settings, definitions, logging.NoopLogger{})
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}

	_, err = service.ResolveFactoryTargetCatalog(context.Background(), chatsessions.ResolveFactoryTargetCatalogRequest{
		OperatorSettingsPath: "/operator.json",
		FactoryDiscovery:     chatsessions.FactoryDiscoveryRoots{ProjectRoot: factorydefinitions.ProjectFactoriesRoot("/repos/project-a")},
		ClientWorkingRoot:    "/repos/project-b",
	})
	assertFactoryTargetCatalogError(t, err, chatsessions.ErrFactoryTargetWorkingRootIncompatible, "factory:@you/factory-builder")
}

func TestResolveFactoryTargetCatalogAllowsCompatiblePinnedWorkingRoot(t *testing.T) {
	t.Parallel()

	profile := operatorsettings.ACPAgentProfile{
		DefaultTarget:  "factory:@you/factory-builder",
		AllowedTargets: []string{"factory:@you/factory-builder"},
	}
	entries := []factorydefinitions.EffectiveFactoryCatalogEntry{
		installedFactoryEntry("@you/factory-builder", "Factory Builder"),
	}

	settings := &operatorSettingsFake{
		resolveACPAgentProfile: func(string) (operatorsettings.ACPAgentProfile, error) { return profile, nil },
	}
	definitions := &factoryDefinitionsFake{
		listEffectiveFactories: func(context.Context, factorydefinitions.ListEffectiveFactoriesRequest) (factorydefinitions.ListEffectiveFactoriesResult, error) {
			return factorydefinitions.ListEffectiveFactoriesResult{Entries: entries}, nil
		},
		resolveNamedFactory: func(context.Context, factorydefinitions.ResolveNamedFactoryRequest) (factorydefinitions.ResolveNamedFactoryResult, error) {
			return factorydefinitions.ResolveNamedFactoryResult{
				Resolution: factorydefinitions.NamedFactoryResolution{
					Name:   "@you/factory-builder",
					Source: factorydefinitions.NamedFactoryResolutionSourceProjectLocal,
					// See the matching comment above: this is the
					// project-scoped root derived from the client's own
					// working directory "/repos/project-a", the same
					// derivation validateWorkingRootCompatibility applies to
					// ClientWorkingRoot before comparing.
					ProjectRoot: factorydefinitions.ProjectFactoriesRoot("/repos/project-a"),
				},
			}, nil
		},
	}
	service, err := chatsessionsservice.New(settings, definitions, logging.NoopLogger{})
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}

	result, err := service.ResolveFactoryTargetCatalog(context.Background(), chatsessions.ResolveFactoryTargetCatalogRequest{
		OperatorSettingsPath: "/operator.json",
		FactoryDiscovery:     chatsessions.FactoryDiscoveryRoots{ProjectRoot: factorydefinitions.ProjectFactoriesRoot("/repos/project-a")},
		ClientWorkingRoot:    "/repos/project-a/",
	})
	if err != nil {
		t.Fatalf("ResolveFactoryTargetCatalog: unexpected error: %v", err)
	}
	if result.CurrentTarget != "factory:@you/factory-builder" {
		t.Fatalf("CurrentTarget = %q, want %q", result.CurrentTarget, "factory:@you/factory-builder")
	}
}

func TestResolveFactoryTargetCatalogWrapsCanonicalResolutionDependencyFailure(t *testing.T) {
	t.Parallel()

	profile := operatorsettings.ACPAgentProfile{
		DefaultTarget:  "factory:@you/factory-builder",
		AllowedTargets: []string{"factory:@you/factory-builder"},
	}
	entries := []factorydefinitions.EffectiveFactoryCatalogEntry{
		installedFactoryEntry("@you/factory-builder", "Factory Builder"),
	}

	settings := &operatorSettingsFake{
		resolveACPAgentProfile: func(string) (operatorsettings.ACPAgentProfile, error) { return profile, nil },
	}
	definitions := &factoryDefinitionsFake{
		listEffectiveFactories: func(context.Context, factorydefinitions.ListEffectiveFactoriesRequest) (factorydefinitions.ListEffectiveFactoriesResult, error) {
			return factorydefinitions.ListEffectiveFactoriesResult{Entries: entries}, nil
		},
		resolveNamedFactory: func(context.Context, factorydefinitions.ResolveNamedFactoryRequest) (factorydefinitions.ResolveNamedFactoryResult, error) {
			return factorydefinitions.ResolveNamedFactoryResult{}, errCollaboratorUnavailable
		},
	}
	service, err := chatsessionsservice.New(settings, definitions, logging.NoopLogger{})
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}

	_, err = service.ResolveFactoryTargetCatalog(context.Background(), chatsessions.ResolveFactoryTargetCatalogRequest{
		OperatorSettingsPath: "/operator.json",
		FactoryDiscovery:     chatsessions.FactoryDiscoveryRoots{ProjectRoot: "/repos/project-a"},
		ClientWorkingRoot:    "/repos/project-a",
	})
	assertFactoryTargetCatalogError(t, err, chatsessions.ErrFactoryTargetCatalogUnavailable, "factory:@you/factory-builder")
	if errors.Is(err, errCollaboratorUnavailable) {
		t.Fatalf("error unwraps to the raw collaborator error, leaking internal detail: %v", err)
	}
}

func TestResolveFactoryTargetCatalogPreservesProfileDependencyContextCause(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		cause error
	}{
		{name: "canceled", cause: context.Canceled},
		{name: "deadline exceeded", cause: context.DeadlineExceeded},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			spy := &spyLogger{}
			settings := &operatorSettingsFake{
				resolveACPAgentProfile: func(string) (operatorsettings.ACPAgentProfile, error) {
					return operatorsettings.ACPAgentProfile{}, testCase.cause
				},
			}
			definitions := &factoryDefinitionsFake{}
			service, err := chatsessionsservice.New(settings, definitions, spy)
			if err != nil {
				t.Fatalf("New: unexpected error: %v", err)
			}

			result, err := service.ResolveFactoryTargetCatalog(context.Background(), chatsessions.ResolveFactoryTargetCatalogRequest{
				OperatorSettingsPath: "/operator.json",
			})
			assertFactoryTargetCatalogError(t, err, chatsessions.ErrFactoryTargetProfileUnavailable, "")
			if !errors.Is(err, testCase.cause) {
				t.Fatalf("ResolveFactoryTargetCatalog: error = %v, want errors.Is match for %v", err, testCase.cause)
			}
			if len(result.Choices) != 0 || result.CurrentTarget != "" {
				t.Fatalf("ResolveFactoryTargetCatalog: result = %#v, want a zero (non-partial) result on failure", result)
			}
			spy.assertNoForbiddenValuesLogged(t, "/operator.json")
		})
	}
}

func TestResolveFactoryTargetCatalogPreservesCatalogListingDependencyContextCause(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		cause error
	}{
		{name: "canceled", cause: context.Canceled},
		{name: "deadline exceeded", cause: context.DeadlineExceeded},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
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
					return factorydefinitions.ListEffectiveFactoriesResult{}, testCase.cause
				},
			}
			service, err := chatsessionsservice.New(settings, definitions, spy)
			if err != nil {
				t.Fatalf("New: unexpected error: %v", err)
			}

			result, err := service.ResolveFactoryTargetCatalog(context.Background(), chatsessions.ResolveFactoryTargetCatalogRequest{
				OperatorSettingsPath: "/operator.json",
			})
			assertFactoryTargetCatalogError(t, err, chatsessions.ErrFactoryTargetCatalogUnavailable, "")
			if !errors.Is(err, testCase.cause) {
				t.Fatalf("ResolveFactoryTargetCatalog: error = %v, want errors.Is match for %v", err, testCase.cause)
			}
			if len(result.Choices) != 0 || result.CurrentTarget != "" {
				t.Fatalf("ResolveFactoryTargetCatalog: result = %#v, want a zero (non-partial) result on failure", result)
			}
			spy.assertNoForbiddenValuesLogged(t, "/operator.json")
		})
	}
}

func TestResolveFactoryTargetCatalogPreservesCanonicalResolutionDependencyContextCause(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		cause error
	}{
		{name: "canceled", cause: context.Canceled},
		{name: "deadline exceeded", cause: context.DeadlineExceeded},
	}

	profile := operatorsettings.ACPAgentProfile{
		DefaultTarget:  "factory:@you/factory-builder",
		AllowedTargets: []string{"factory:@you/factory-builder"},
	}
	entries := []factorydefinitions.EffectiveFactoryCatalogEntry{
		installedFactoryEntry("@you/factory-builder", "Factory Builder"),
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			spy := &spyLogger{}
			settings := &operatorSettingsFake{
				resolveACPAgentProfile: func(string) (operatorsettings.ACPAgentProfile, error) { return profile, nil },
			}
			definitions := &factoryDefinitionsFake{
				listEffectiveFactories: func(context.Context, factorydefinitions.ListEffectiveFactoriesRequest) (factorydefinitions.ListEffectiveFactoriesResult, error) {
					return factorydefinitions.ListEffectiveFactoriesResult{Entries: entries}, nil
				},
				resolveNamedFactory: func(context.Context, factorydefinitions.ResolveNamedFactoryRequest) (factorydefinitions.ResolveNamedFactoryResult, error) {
					return factorydefinitions.ResolveNamedFactoryResult{}, testCase.cause
				},
			}
			service, err := chatsessionsservice.New(settings, definitions, spy)
			if err != nil {
				t.Fatalf("New: unexpected error: %v", err)
			}

			result, err := service.ResolveFactoryTargetCatalog(context.Background(), chatsessions.ResolveFactoryTargetCatalogRequest{
				OperatorSettingsPath: "/operator.json",
				FactoryDiscovery:     chatsessions.FactoryDiscoveryRoots{ProjectRoot: "/repos/project-a"},
				ClientWorkingRoot:    "/repos/project-a",
			})
			assertFactoryTargetCatalogError(t, err, chatsessions.ErrFactoryTargetCatalogUnavailable, "factory:@you/factory-builder")
			if !errors.Is(err, testCase.cause) {
				t.Fatalf("ResolveFactoryTargetCatalog: error = %v, want errors.Is match for %v", err, testCase.cause)
			}
			if len(result.Choices) != 0 || result.CurrentTarget != "" {
				t.Fatalf("ResolveFactoryTargetCatalog: result = %#v, want a zero (non-partial) result on failure", result)
			}
			spy.assertNoForbiddenValuesLogged(t, "/repos/project-a", "factory:@you/factory-builder")
		})
	}
}

func TestResolveFactoryTargetCatalogWrapsInstalledCatalogDependencyFailure(t *testing.T) {
	t.Parallel()

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
			return factorydefinitions.ListEffectiveFactoriesResult{}, errCollaboratorUnavailable
		},
	}
	service, err := chatsessionsservice.New(settings, definitions, logging.NoopLogger{})
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}

	_, err = service.ResolveFactoryTargetCatalog(context.Background(), chatsessions.ResolveFactoryTargetCatalogRequest{
		OperatorSettingsPath: "/operator.json",
	})
	assertFactoryTargetCatalogError(t, err, chatsessions.ErrFactoryTargetCatalogUnavailable, "")
	if errors.Is(err, errCollaboratorUnavailable) {
		t.Fatalf("error unwraps to the raw collaborator error, leaking internal detail: %v", err)
	}
}

// assertFactoryTargetCatalogError fails the test unless err is a
// *chatsessions.FactoryTargetCatalogError classifiable via errors.Is against
// wantSentinel, carrying wantTarget. It never inspects err.Error() text,
// since this operation's typed errors are meant to be classified by
// errors.Is/errors.As rather than parsed.
func assertFactoryTargetCatalogError(t *testing.T, err error, wantSentinel error, wantTarget string) {
	t.Helper()
	if err == nil {
		t.Fatalf("ResolveFactoryTargetCatalog: expected error wrapping %v, got nil", wantSentinel)
	}
	if !errors.Is(err, wantSentinel) {
		t.Fatalf("ResolveFactoryTargetCatalog: error = %v, want errors.Is match for %v", err, wantSentinel)
	}
	var typed *chatsessions.FactoryTargetCatalogError
	if !errors.As(err, &typed) {
		t.Fatalf("ResolveFactoryTargetCatalog: error = %v, want *chatsessions.FactoryTargetCatalogError", err)
	}
	if typed.Target != wantTarget {
		t.Fatalf("FactoryTargetCatalogError.Target = %q, want %q", typed.Target, wantTarget)
	}
}
