package impl

import (
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestExpectedArtifactTargets_ReportsActionableOwningDefinitionPaths(t *testing.T) {
	cfg := &factorydefinitions.FactoryConfig{
		WorkTypes: []factorydefinitions.WorkTypeConfig{{
			Name: "story",
			ExpectedArtifacts: []factorydefinitions.ExpectedArtifactConfig{{
				Name: "", Pattern: "artifacts/story.json",
			}},
		}},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{{
			Name:   "execute-story",
			Inputs: []factorydefinitions.IOConfig{{WorkTypeName: "story", StateName: "init"}},
			ExpectedArtifacts: []factorydefinitions.ExpectedArtifactConfig{{
				Name: "manifest", Pattern: "artifacts/{{ (index .Inputs 1).Name }}/manifest.json",
			}},
		}},
	}

	targets := ExpectedArtifactTargets(cfg)
	if len(targets) != 2 {
		t.Fatalf("ExpectedArtifactTargets() returned %d targets, want 2: %#v", len(targets), targets)
	}
	workTypeTarget, workstationTarget := targets[0], targets[1]
	if workTypeTarget.Code != CodeWorkTypeInvalidExpectedArtifact ||
		workTypeTarget.Subject.Type != SubjectTypeWorkType || workTypeTarget.Subject.ID != "story" ||
		workTypeTarget.Path != "factory.workTypes[0](story).expectedArtifacts[0]" ||
		!strings.Contains(workTypeTarget.Message, "work type \"story\"") ||
		!strings.Contains(workTypeTarget.Message, "name is required") {
		t.Fatalf("work type target = %#v, want owning definition path and diagnostic", workTypeTarget)
	}
	if workstationTarget.Code != CodeWorkstationInvalidExpectedArtifact ||
		workstationTarget.Subject.Type != SubjectTypeWorkstation || workstationTarget.Subject.ID != "execute-story" ||
		workstationTarget.Path != "factory.workstations[0](execute-story).expectedArtifacts[0]" ||
		!strings.Contains(workstationTarget.Message, "workstation \"execute-story\"") ||
		!strings.Contains(workstationTarget.Message, "cannot be rendered") {
		t.Fatalf("workstation target = %#v, want owning definition path and diagnostic", workstationTarget)
	}
}

func TestWebhookTargetsAcceptsCanonicalFilterAndDeliveryPolicy(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Webhooks = []factorydefinitions.FactoryWebhookConfig{{
		Name:             "monitor",
		Enabled:          true,
		URL:              "https://hooks.example.test/factory",
		SigningSecretRef: "secrets/factory-monitor",
		Filter: factorydefinitions.FactoryWebhookFilterConfig{
			EventTypes:       []string{factorydefinitions.FactoryWebhookEventTypeWorkStateChange, factorydefinitions.FactoryWebhookEventTypeDispatchReconciled},
			DispatchStatuses: []string{factorydefinitions.FactoryWebhookDispatchStatusFailed},
		},
		DeliveryPolicy: &factorydefinitions.FactoryWebhookDeliveryPolicyConfig{
			RequestTimeout:    stringPointerForWebhookTest("15s"),
			MaxAttempts:       intPointerForWebhookTest(3),
			InitialBackoff:    stringPointerForWebhookTest("1s"),
			BackoffMultiplier: floatPointerForWebhookTest(1.5),
			MaxBackoff:        stringPointerForWebhookTest("5s"),
		},
	}}

	findings := WebhookTargets(cfg)
	if len(findings) != 0 {
		t.Fatalf("findings = %#v, want none", findings)
	}
}

func TestWebhookTargetsRejectsDispatchFiltersWithNoCompatibleStatus(t *testing.T) {
	tests := []struct {
		name       string
		eventTypes []string
		statuses   []string
	}{
		{
			name:       "response cannot select interrupted",
			eventTypes: []string{factorydefinitions.FactoryWebhookEventTypeDispatchResponse},
			statuses:   []string{factorydefinitions.FactoryWebhookDispatchStatusInterrupted},
		},
		{
			name:       "interrupted cannot select failed",
			eventTypes: []string{factorydefinitions.FactoryWebhookEventTypeDispatchInterrupted},
			statuses:   []string{factorydefinitions.FactoryWebhookDispatchStatusFailed},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := testBaseConfig()
			cfg.Webhooks = []factorydefinitions.FactoryWebhookConfig{{
				Name:             "monitor",
				URL:              "https://hooks.example.test/factory",
				SigningSecretRef: "secrets/factory-monitor",
				Filter: factorydefinitions.FactoryWebhookFilterConfig{
					EventTypes:       test.eventTypes,
					DispatchStatuses: test.statuses,
				},
			}}

			findings := WebhookTargets(cfg)
			assertWebhookTargetMatch(
				t,
				findings,
				CodeWebhookDispatchStatusIncompatible,
				"factory.webhooks[0](monitor).filter.dispatchStatuses",
				"no status compatible with the configured dispatch event types",
			)
		})
	}
}

func TestWebhookTargetsAcceptsMixedDispatchFilterStatusCombinations(t *testing.T) {
	tests := []struct {
		name       string
		eventTypes []string
		statuses   []string
	}{
		{
			name: "failed response and interrupted dispatch",
			eventTypes: []string{
				factorydefinitions.FactoryWebhookEventTypeDispatchResponse,
				factorydefinitions.FactoryWebhookEventTypeDispatchInterrupted,
			},
			statuses: []string{
				factorydefinitions.FactoryWebhookDispatchStatusFailed,
				factorydefinitions.FactoryWebhookDispatchStatusInterrupted,
			},
		},
		{
			name: "failed response among mixed work and dispatch events",
			eventTypes: []string{
				factorydefinitions.FactoryWebhookEventTypeWorkStateChange,
				factorydefinitions.FactoryWebhookEventTypeDispatchResponse,
			},
			statuses: []string{factorydefinitions.FactoryWebhookDispatchStatusFailed},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := testBaseConfig()
			cfg.Webhooks = []factorydefinitions.FactoryWebhookConfig{{
				Name:             "monitor",
				URL:              "https://hooks.example.test/factory",
				SigningSecretRef: "secrets/factory-monitor",
				Filter: factorydefinitions.FactoryWebhookFilterConfig{
					EventTypes:       test.eventTypes,
					DispatchStatuses: test.statuses,
				},
			}}

			findings := WebhookTargets(cfg)
			if len(findings) != 0 {
				t.Fatalf("findings = %#v, want none", findings)
			}
		})
	}
}

func TestWebhookTargetsRejectsInvalidFieldsWithEndpointPaths(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Webhooks = []factorydefinitions.FactoryWebhookConfig{
		{
			Name:             "monitor",
			URL:              "ftp://hooks.example.test/factory",
			SigningSecretRef: " ",
			Filter: factorydefinitions.FactoryWebhookFilterConfig{
				EventTypes:       []string{factorydefinitions.FactoryWebhookEventTypeWorkStateChange, "NOT_CANONICAL"},
				DispatchStatuses: []string{factorydefinitions.FactoryWebhookDispatchStatusFailed},
			},
			DeliveryPolicy: &factorydefinitions.FactoryWebhookDeliveryPolicyConfig{
				MaxAttempts:       intPointerForWebhookTest(0),
				InitialBackoff:    stringPointerForWebhookTest("5s"),
				MaxBackoff:        stringPointerForWebhookTest("1s"),
				BackoffMultiplier: floatPointerForWebhookTest(0.5),
			},
		},
		{
			Name:             " monitor ",
			Enabled:          true,
			URL:              "https://hooks.example.test/other",
			SigningSecretRef: "secrets/other",
			Filter: factorydefinitions.FactoryWebhookFilterConfig{
				EventTypes: []string{factorydefinitions.FactoryWebhookEventTypeWorkStateChange},
			},
		},
	}

	findings := WebhookTargets(cfg)
	assertWebhookTargetMatch(t, findings, CodeWebhookURLInvalid, "factory.webhooks[0](monitor).url", "absolute http or https")
	assertWebhookTargetMatch(t, findings, CodeWebhookSecretReferenceRequired, "factory.webhooks[0](monitor).signingSecretRef", "non-empty")
	assertWebhookTargetMatch(t, findings, CodeWebhookEventTypeUnsupported, "factory.webhooks[0](monitor).filter.eventTypes[1]", "NOT_CANONICAL")
	assertWebhookTargetMatch(t, findings, CodeWebhookDeliveryPolicyInvalid, "factory.webhooks[0](monitor).deliveryPolicy.maxAttempts", "positive")
	assertWebhookTargetMatch(t, findings, CodeWebhookDeliveryPolicyInvalid, "factory.webhooks[0](monitor).deliveryPolicy.backoffMultiplier", "at least 1")
	assertWebhookTargetMatch(t, findings, CodeWebhookDeliveryPolicyInvalid, "factory.webhooks[0](monitor).deliveryPolicy.maxBackoff", "initialBackoff")
	assertWebhookTargetMatch(t, findings, CodeWebhookNameDuplicate, "factory.webhooks[1]( monitor ).name", "duplicates")
}

func assertWebhookTargetMatch(t *testing.T, targets []Target, code, pathSubstring, messageSubstring string) {
	t.Helper()
	for _, target := range targets {
		if target.Code != code || target.Severity != SeverityError || !strings.Contains(target.Path, pathSubstring) {
			continue
		}
		if !strings.Contains(target.Message, messageSubstring) {
			t.Fatalf("target message = %q, want substring %q", target.Message, messageSubstring)
		}
		return
	}
	t.Fatalf("expected error target with code %q, got %v", code, targets)
}

func stringPointerForWebhookTest(value string) *string { return &value }

func intPointerForWebhookTest(value int) *int { return &value }

func floatPointerForWebhookTest(value float64) *float64 { return &value }
