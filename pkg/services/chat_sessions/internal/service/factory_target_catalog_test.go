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
		{Name: "@you/factory-builder", Definition: &factorydefinitions.FactoryConfig{Name: "Factory Builder"}},
		{Name: "@you/review", Definition: &factorydefinitions.FactoryConfig{Name: "Review"}},
		{Name: "@you/extra", Definition: &factorydefinitions.FactoryConfig{Name: "Extra"}},
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
		{Name: "@you/factory-builder", Definition: &factorydefinitions.FactoryConfig{Name: "Factory Builder"}},
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
		{Name: "@you/factory-builder", Definition: &factorydefinitions.FactoryConfig{Name: "Factory Builder"}},
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
		{Name: "@you/factory-builder", Definition: &factorydefinitions.FactoryConfig{Name: "Factory Builder"}},
		{Name: "@you/extra", Definition: &factorydefinitions.FactoryConfig{Name: "Extra"}},
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
		{Name: "@you/factory-builder", Definition: &factorydefinitions.FactoryConfig{Name: "Factory Builder"}},
		{Name: "@you/review", Definition: &factorydefinitions.FactoryConfig{Name: "Review"}},
		{Name: "@you/analysis", Definition: &factorydefinitions.FactoryConfig{Name: "Analysis"}},
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
		{Name: "@you/factory-builder", Definition: &factorydefinitions.FactoryConfig{Name: "Factory Builder"}},
		{Name: "@you/review", Definition: &factorydefinitions.FactoryConfig{Name: "Review"}},
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
