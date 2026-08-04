package service_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	chatsessionsservice "github.com/portpowered/infinite-you/pkg/services/chat_sessions/internal/service"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

func newTestService(t *testing.T, profile operatorsettings.ACPAgentProfile, entries []factorydefinitions.EffectiveFactoryCatalogEntry) chatsessions.FactoryTargetCatalogService {
	t.Helper()

	settings := &operatorSettingsFake{
		resolveACPAgentProfile: func(string) (operatorsettings.ACPAgentProfile, error) {
			return profile, nil
		},
	}
	definitions := &factoryDefinitionsFake{
		listEffectiveFactories: func(context.Context, factorydefinitions.ListEffectiveFactoriesRequest) (factorydefinitions.ListEffectiveFactoriesResult, error) {
			return factorydefinitions.ListEffectiveFactoriesResult{Entries: entries}, nil
		},
	}

	service, err := chatsessionsservice.New(settings, definitions, logging.NoopLogger{})
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}
	return service
}

func TestResolveFactoryTargetCatalogExactAllowlistFiltering(t *testing.T) {
	t.Parallel()

	profile := operatorsettings.ACPAgentProfile{
		DefaultTarget:  "factory:@you/factory-builder",
		AllowedTargets: []string{"factory:@you/factory-builder", "factory:@you/review"},
	}
	entries := []factorydefinitions.EffectiveFactoryCatalogEntry{
		installedFactoryEntry("@you/factory-builder", "Factory Builder"),
		installedFactoryEntry("@you/review", "Review"),
		installedFactoryEntry("@you/extra", "Extra"),
	}
	service := newTestService(t, profile, entries)

	result, err := service.ResolveFactoryTargetCatalog(context.Background(), chatsessions.ResolveFactoryTargetCatalogRequest{
		OperatorSettingsPath: "/operator.json",
	})
	if err != nil {
		t.Fatalf("ResolveFactoryTargetCatalog: unexpected error: %v", err)
	}

	if result.CurrentTarget != "factory:@you/factory-builder" {
		t.Fatalf("CurrentTarget = %q, want %q", result.CurrentTarget, "factory:@you/factory-builder")
	}
	wantValues := []string{"factory:@you/factory-builder", "factory:@you/review"}
	if len(result.Choices) != len(wantValues) {
		t.Fatalf("Choices = %+v, want values %v", result.Choices, wantValues)
	}
	for index, choice := range result.Choices {
		if choice.Value != wantValues[index] {
			t.Fatalf("Choices[%d].Value = %q, want %q", index, choice.Value, wantValues[index])
		}
		if choice.Name == "" {
			t.Fatalf("Choices[%d].Name is empty", index)
		}
	}
}

func TestResolveFactoryTargetCatalogMissingProfileDefaultsToFactoryBuilder(t *testing.T) {
	t.Parallel()

	entries := []factorydefinitions.EffectiveFactoryCatalogEntry{
		installedFactoryEntry("@you/factory-builder", "Factory Builder"),
	}
	// A missing authored profile resolves to Operator Settings' safe default,
	// which this fake models directly since ResolveACPAgentProfile itself
	// already owns that defaulting behavior.
	service := newTestService(t, operatorsettings.DefaultACPAgentProfile(), entries)

	result, err := service.ResolveFactoryTargetCatalog(context.Background(), chatsessions.ResolveFactoryTargetCatalogRequest{
		OperatorSettingsPath: "/operator.json",
	})
	if err != nil {
		t.Fatalf("ResolveFactoryTargetCatalog: unexpected error: %v", err)
	}

	if result.CurrentTarget != operatorsettings.DefaultACPAgentProfileTarget {
		t.Fatalf("CurrentTarget = %q, want %q", result.CurrentTarget, operatorsettings.DefaultACPAgentProfileTarget)
	}
	if len(result.Choices) != 1 || result.Choices[0].Value != operatorsettings.DefaultACPAgentProfileTarget {
		t.Fatalf("Choices = %+v, want exactly the default Factory Builder target", result.Choices)
	}
}

func TestResolveFactoryTargetCatalogExcludesAllowedButUninstalled(t *testing.T) {
	t.Parallel()

	profile := operatorsettings.ACPAgentProfile{
		DefaultTarget:  "factory:@you/factory-builder",
		AllowedTargets: []string{"factory:@you/factory-builder", "factory:@you/missing"},
	}
	entries := []factorydefinitions.EffectiveFactoryCatalogEntry{
		installedFactoryEntry("@you/factory-builder", "Factory Builder"),
	}
	service := newTestService(t, profile, entries)

	result, err := service.ResolveFactoryTargetCatalog(context.Background(), chatsessions.ResolveFactoryTargetCatalogRequest{
		OperatorSettingsPath: "/operator.json",
	})
	if err != nil {
		t.Fatalf("ResolveFactoryTargetCatalog: unexpected error: %v", err)
	}

	if len(result.Choices) != 1 || result.Choices[0].Value != "factory:@you/factory-builder" {
		t.Fatalf("Choices = %+v, want only the installed allowed target", result.Choices)
	}
}

func TestResolveFactoryTargetCatalogExcludesInstalledButDisallowed(t *testing.T) {
	t.Parallel()

	profile := operatorsettings.ACPAgentProfile{
		DefaultTarget:  "factory:@you/factory-builder",
		AllowedTargets: []string{"factory:@you/factory-builder"},
	}
	entries := []factorydefinitions.EffectiveFactoryCatalogEntry{
		installedFactoryEntry("@you/factory-builder", "Factory Builder"),
		installedFactoryEntry("@you/extra", "Extra"),
	}
	service := newTestService(t, profile, entries)

	result, err := service.ResolveFactoryTargetCatalog(context.Background(), chatsessions.ResolveFactoryTargetCatalogRequest{
		OperatorSettingsPath: "/operator.json",
	})
	if err != nil {
		t.Fatalf("ResolveFactoryTargetCatalog: unexpected error: %v", err)
	}

	if len(result.Choices) != 1 || result.Choices[0].Value != "factory:@you/factory-builder" {
		t.Fatalf("Choices = %+v, want only the allowed installed target", result.Choices)
	}
}

func TestResolveFactoryTargetCatalogDeterministicOrderingAndDedup(t *testing.T) {
	t.Parallel()

	entries := []factorydefinitions.EffectiveFactoryCatalogEntry{
		installedFactoryEntry("@you/factory-builder", "Factory Builder"),
		installedFactoryEntry("@you/review", "Review"),
		installedFactoryEntry("@you/analysis", "Analysis"),
	}
	reorderedEntries := []factorydefinitions.EffectiveFactoryCatalogEntry{entries[2], entries[0], entries[1]}
	duplicatedEntries := append(append([]factorydefinitions.EffectiveFactoryCatalogEntry{}, entries...), entries[1], entries[0])

	wantValues := []string{"factory:@you/factory-builder", "factory:@you/analysis", "factory:@you/review"}

	cases := []struct {
		name           string
		allowedTargets []string
		entries        []factorydefinitions.EffectiveFactoryCatalogEntry
	}{
		{
			name:           "canonical enumeration order",
			allowedTargets: []string{"factory:@you/factory-builder", "factory:@you/review", "factory:@you/analysis"},
			entries:        entries,
		},
		{
			name:           "reordered allowlist and reordered installed catalog",
			allowedTargets: []string{"factory:@you/analysis", "factory:@you/factory-builder", "factory:@you/review"},
			entries:        reorderedEntries,
		},
		{
			name:           "duplicate allowlist entries",
			allowedTargets: []string{"factory:@you/review", "factory:@you/factory-builder", "factory:@you/review", "factory:@you/analysis", "factory:@you/factory-builder"},
			entries:        entries,
		},
		{
			name:           "duplicate installed catalog entries",
			allowedTargets: []string{"factory:@you/factory-builder", "factory:@you/review", "factory:@you/analysis"},
			entries:        duplicatedEntries,
		},
	}

	var results []chatsessions.ResolveFactoryTargetCatalogResult
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			// Deliberately sequential (no t.Parallel()): results are collected
			// below for a deep-equivalence check across cases, which requires
			// every subtest to have completed before that check runs.
			profile := operatorsettings.ACPAgentProfile{
				DefaultTarget:  "factory:@you/factory-builder",
				AllowedTargets: testCase.allowedTargets,
			}
			service := newTestService(t, profile, testCase.entries)

			result, err := service.ResolveFactoryTargetCatalog(context.Background(), chatsessions.ResolveFactoryTargetCatalogRequest{
				OperatorSettingsPath: "/operator.json",
			})
			if err != nil {
				t.Fatalf("ResolveFactoryTargetCatalog: unexpected error: %v", err)
			}
			if result.CurrentTarget != "factory:@you/factory-builder" {
				t.Fatalf("CurrentTarget = %q, want %q", result.CurrentTarget, "factory:@you/factory-builder")
			}
			if len(result.Choices) != len(wantValues) {
				t.Fatalf("Choices = %+v, want %d deduplicated values", result.Choices, len(wantValues))
			}
			for index, value := range wantValues {
				if result.Choices[index].Value != value {
					t.Fatalf("Choices[%d].Value = %q, want %q (current-first, then ascending)", index, result.Choices[index].Value, value)
				}
			}
			results = append(results, result)
		})
	}

	for index := 1; index < len(results); index++ {
		if !reflect.DeepEqual(results[0], results[index]) {
			t.Fatalf("case %d result %+v is not deeply equivalent to case 0 result %+v despite equivalent profile/catalog facts", index, results[index], results[0])
		}
	}
}

func TestResolveFactoryTargetCatalogResultIsDetached(t *testing.T) {
	t.Parallel()

	profile := operatorsettings.ACPAgentProfile{
		DefaultTarget:  "factory:@you/factory-builder",
		AllowedTargets: []string{"factory:@you/factory-builder", "factory:@you/review"},
	}
	entries := []factorydefinitions.EffectiveFactoryCatalogEntry{
		installedFactoryEntry("@you/factory-builder", "Factory Builder"),
		installedFactoryEntry("@you/review", "Review"),
	}
	service := newTestService(t, profile, entries)

	first, err := service.ResolveFactoryTargetCatalog(context.Background(), chatsessions.ResolveFactoryTargetCatalogRequest{
		OperatorSettingsPath: "/operator.json",
	})
	if err != nil {
		t.Fatalf("ResolveFactoryTargetCatalog: unexpected error: %v", err)
	}
	first.Choices[0].Value = "mutated"
	first.Choices[0].Name = "mutated"

	second, err := service.ResolveFactoryTargetCatalog(context.Background(), chatsessions.ResolveFactoryTargetCatalogRequest{
		OperatorSettingsPath: "/operator.json",
	})
	if err != nil {
		t.Fatalf("ResolveFactoryTargetCatalog: unexpected error: %v", err)
	}
	if second.Choices[0].Value == "mutated" {
		t.Fatalf("second result observed mutation of a prior result's Choices slice")
	}
}

// TestResolveFactoryTargetCatalogDisplayNameFallsBackToCanonicalName proves
// an entry with no Factory Definition (packaged-only, never materialized)
// still resolves to a non-empty, stable display name: the catalog entry's
// own canonical Name, not an empty string.
func TestResolveFactoryTargetCatalogDisplayNameFallsBackToCanonicalName(t *testing.T) {
	t.Parallel()

	profile := operatorsettings.ACPAgentProfile{
		DefaultTarget:  "factory:@you/factory-builder",
		AllowedTargets: []string{"factory:@you/factory-builder"},
	}
	location := "/factories/@you/factory-builder"
	entries := []factorydefinitions.EffectiveFactoryCatalogEntry{
		{Name: "@you/factory-builder", Location: &location},
	}
	service := newTestService(t, profile, entries)

	result, err := service.ResolveFactoryTargetCatalog(context.Background(), chatsessions.ResolveFactoryTargetCatalogRequest{
		OperatorSettingsPath: "/operator.json",
	})
	if err != nil {
		t.Fatalf("ResolveFactoryTargetCatalog: unexpected error: %v", err)
	}
	if len(result.Choices) != 1 {
		t.Fatalf("Choices = %+v, want exactly 1", result.Choices)
	}
	if result.Choices[0].Name != "@you/factory-builder" {
		t.Fatalf("Choices[0].Name = %q, want the fallback canonical name %q", result.Choices[0].Name, "@you/factory-builder")
	}
}

// operatorSettingsFake is a focused Operator Settings root fake exercising
// only ResolveACPAgentProfile, the sole collaborator method the Factory
// target-catalog operation depends on.
type operatorSettingsFake struct {
	operatorsettings.Service

	resolveACPAgentProfile func(string) (operatorsettings.ACPAgentProfile, error)
}

func (fake *operatorSettingsFake) ResolveACPAgentProfile(path string) (operatorsettings.ACPAgentProfile, error) {
	if fake.resolveACPAgentProfile != nil {
		return fake.resolveACPAgentProfile(path)
	}
	return operatorsettings.ACPAgentProfile{}, errUnexpectedCall
}

// factoryDefinitionsFake is a focused Factory Definitions root fake
// exercising only ListEffectiveFactories and ResolveNamedFactory, the
// collaborator methods the Factory target-catalog operation depends on.
// resolveNamedFactory is only exercised when a test supplies a
// ClientWorkingRoot, since the operation only calls it in that case.
type factoryDefinitionsFake struct {
	factorydefinitions.Service

	listEffectiveFactories func(context.Context, factorydefinitions.ListEffectiveFactoriesRequest) (factorydefinitions.ListEffectiveFactoriesResult, error)
	resolveNamedFactory    func(context.Context, factorydefinitions.ResolveNamedFactoryRequest) (factorydefinitions.ResolveNamedFactoryResult, error)
}

func (fake *factoryDefinitionsFake) ListEffectiveFactories(
	ctx context.Context,
	request factorydefinitions.ListEffectiveFactoriesRequest,
) (factorydefinitions.ListEffectiveFactoriesResult, error) {
	if fake.listEffectiveFactories != nil {
		return fake.listEffectiveFactories(ctx, request)
	}
	return factorydefinitions.ListEffectiveFactoriesResult{}, errUnexpectedCall
}

func (fake *factoryDefinitionsFake) ResolveNamedFactory(
	ctx context.Context,
	request factorydefinitions.ResolveNamedFactoryRequest,
) (factorydefinitions.ResolveNamedFactoryResult, error) {
	if fake.resolveNamedFactory != nil {
		return fake.resolveNamedFactory(ctx, request)
	}
	return factorydefinitions.ResolveNamedFactoryResult{}, errUnexpectedCall
}

type unexpectedCallError struct{}

func (unexpectedCallError) Error() string { return "unexpected fake call" }

var errUnexpectedCall = unexpectedCallError{}

// installedFactoryEntry returns an EffectiveFactoryCatalogEntry representing
// a materialized (installed) Factory. Factory Definitions' contract leaves
// Location nil for packaged definitions that have not been materialized, so
// every fixture standing in for an installed Factory must set it explicitly.
func installedFactoryEntry(name, displayName string) factorydefinitions.EffectiveFactoryCatalogEntry {
	location := "/factories/" + name
	return factorydefinitions.EffectiveFactoryCatalogEntry{
		Name:       name,
		Location:   &location,
		Definition: &factorydefinitions.FactoryConfig{Name: displayName},
	}
}

// packagedOnlyFactoryEntry returns an EffectiveFactoryCatalogEntry
// representing a packaged Factory definition that has not been materialized:
// it is effective/listable but never counts as installed.
func packagedOnlyFactoryEntry(name, displayName string) factorydefinitions.EffectiveFactoryCatalogEntry {
	return factorydefinitions.EffectiveFactoryCatalogEntry{
		Name:       name,
		Definition: &factorydefinitions.FactoryConfig{Name: displayName},
	}
}
