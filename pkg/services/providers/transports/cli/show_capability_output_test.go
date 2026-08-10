package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerscli "github.com/portpowered/infinite-you/pkg/services/providers/transports/cli"
)

func TestShowOutputIncludesPrerequisiteFactsInHumanAndJSONModes(t *testing.T) {
	t.Parallel()

	root := &recordingProvidersRoot{getResult: providers.GetProviderResult{Provider: providers.Descriptor{
		ID:           providers.IDCodex,
		DisplayName:  "Codex",
		Availability: providers.AvailabilitySelectable,
		Readiness:    providers.ReadinessReady,
		Capabilities: []providers.Capability{providers.CapabilityPromptSubmission},
		Prerequisites: []providers.Prerequisite{
			{Kind: providers.PrerequisiteConfiguration, Name: "endpoint", Status: providers.PrerequisiteSatisfied},
			{Kind: providers.PrerequisiteAuthentication, Name: "account", Status: providers.PrerequisiteMissing, Description: "Sign in first."},
		},
	}}}
	service := constructedProvidersCLIService(t, root)

	var human bytes.Buffer
	if err := service.Show(providerscli.ShowConfig{Context: context.Background(), ProviderID: "codex", Output: &human}); err != nil {
		t.Fatalf("Show() human error = %v", err)
	}
	if !strings.Contains(human.String(), "Prerequisites:\tauthentication/account=missing (Sign in first.); configuration/endpoint=satisfied\n") {
		t.Fatalf("human output = %q, want sorted prerequisite facts", human.String())
	}

	var jsonOutput bytes.Buffer
	if err := service.Show(providerscli.ShowConfig{Context: context.Background(), ProviderID: "codex", JSON: true, Output: &jsonOutput}); err != nil {
		t.Fatalf("Show() JSON error = %v", err)
	}
	var result struct {
		Prerequisites []struct {
			Kind        string `json:"kind"`
			Name        string `json:"name"`
			Status      string `json:"status"`
			Description string `json:"description"`
		} `json:"prerequisites"`
	}
	if err := json.Unmarshal(jsonOutput.Bytes(), &result); err != nil {
		t.Fatalf("Show() JSON invalid: %v", err)
	}
	if len(result.Prerequisites) != 2 || result.Prerequisites[0].Kind != "configuration" || result.Prerequisites[1].Description != "Sign in first." {
		t.Fatalf("JSON prerequisites = %#v, want both provider facts", result.Prerequisites)
	}
}
