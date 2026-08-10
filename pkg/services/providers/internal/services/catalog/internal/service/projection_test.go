package service

import (
	"reflect"
	"strings"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

func TestProjectAvailabilityPostures(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		manifest      publishedProviderManifest
		wantAvail     providers.Availability
		wantReadiness providers.Readiness
	}{
		{
			name: "not-supported",
			manifest: publishedProviderManifest{
				ID:                         "blocked",
				TechnicalSupportLevel:      "not-supported",
				ImplementationAvailability: "bundled",
			},
			wantAvail:     providers.AvailabilityNotSupported,
			wantReadiness: providers.ReadinessUnavailable,
		},
		{
			name: "catalog-only",
			manifest: publishedProviderManifest{
				ID:                         "catalog-only",
				TechnicalSupportLevel:      "experimental",
				ImplementationAvailability: "catalog-only",
			},
			wantAvail:     providers.AvailabilityCatalogOnly,
			wantReadiness: providers.ReadinessUnavailable,
		},
		{
			name: "selectable bundled",
			manifest: publishedProviderManifest{
				ID:                         "selectable",
				TechnicalSupportLevel:      "production",
				ImplementationAvailability: "bundled",
			},
			wantAvail:     providers.AvailabilitySelectable,
			wantReadiness: providers.ReadinessReady,
		},
		{
			name: "selectable externally supplied",
			manifest: publishedProviderManifest{
				ID:                         "external",
				TechnicalSupportLevel:      "experimental",
				ImplementationAvailability: "externally-supplied",
			},
			wantAvail:     providers.AvailabilitySelectable,
			wantReadiness: providers.ReadinessReady,
		},
		{
			name: "supported-but-unavailable default",
			manifest: publishedProviderManifest{
				ID:                         "unsupported-impl",
				TechnicalSupportLevel:      "experimental",
				ImplementationAvailability: "unknown",
			},
			wantAvail:     providers.AvailabilitySupportedButUnavailable,
			wantReadiness: providers.ReadinessUnavailable,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			descriptors, err := projectManifests([]publishedProviderManifest{testCase.manifest})
			if err != nil {
				t.Fatalf("projectManifests() = %v", err)
			}
			if len(descriptors) != 1 {
				t.Fatalf("len(descriptors) = %d, want 1", len(descriptors))
			}
			got := descriptors[0]
			if got.Availability != testCase.wantAvail {
				t.Fatalf("availability = %q, want %q", got.Availability, testCase.wantAvail)
			}
			if got.Readiness != testCase.wantReadiness {
				t.Fatalf("readiness = %q, want %q", got.Readiness, testCase.wantReadiness)
			}
		})
	}
}

func TestProjectStaticPrerequisites(t *testing.T) {
	t.Parallel()

	manifest := publishedProviderManifest{
		ID:          "cursor",
		DisplayName: publishedNameValue{Value: "Cursor CLI"},
		Discovery: publishedProviderDiscovery{
			ConfigurationKeys: []string{"CURSOR_API_KEY"},
			EndpointKinds:     []string{"stdio"},
			ExecutableNames:   []string{"agent"},
		},
		TechnicalSupportLevel:      "experimental",
		ImplementationAvailability: "bundled",
	}

	descriptors, err := projectManifests([]publishedProviderManifest{manifest})
	if err != nil {
		t.Fatalf("projectManifests() = %v", err)
	}
	prerequisites := descriptors[0].Prerequisites
	if len(prerequisites) != 3 {
		t.Fatalf("len(prerequisites) = %d, want 3", len(prerequisites))
	}
	for _, prerequisite := range prerequisites {
		if prerequisite.Status != providers.PrerequisiteSatisfied {
			t.Fatalf("prerequisite %q status = %q, want satisfied", prerequisite.Name, prerequisite.Status)
		}
		if prerequisite.Description == "" {
			t.Fatalf("prerequisite %q missing bounded description", prerequisite.Name)
		}
		if strings.Contains(prerequisite.Description, "C:\\") ||
			strings.Contains(prerequisite.Description, "/Users/") ||
			len(prerequisite.Description) > 256 {
			t.Fatalf("prerequisite description leaks sensitive detail: %q", prerequisite.Description)
		}
	}
}

func TestProjectManifestCapabilityFactsAreCanonicalAndCloneable(t *testing.T) {
	t.Parallel()

	maximum := int64(5)
	manifest := publishedProviderManifest{
		ID:                         "codex",
		DisplayName:                publishedNameValue{Value: "Codex"},
		TechnicalSupportLevel:      "production",
		ImplementationAvailability: "bundled",
		Models: []publishedModel{{
			ID:      "gpt-5.6",
			Efforts: []string{"max", "low", "minimal"},
			Modalities: []publishedModality{
				{Direction: "output", Kind: "text", Support: "supported", Transport: "inline"},
				{Direction: "input", Kind: "video", Support: "unsupported", Transport: "none"},
				{Direction: "input", Kind: "text", Support: "supported", Transport: "inline"},
			},
		}},
		Tools: []publishedTool{
			{Name: "web_search", Support: "supported", Description: "Search the web."},
			{Name: "filesystem", Support: "supported", Description: "Use the workspace."},
		},
		KnownLimits: []publishedKnownLimit{
			{Name: "z_limit", Kind: "maximum", Unit: "paths", Description: "Maximum paths.", Maximum: &maximum},
			{Name: "a_behavior", Kind: "behavior", Unit: "flag", Description: "Behavior flag.", Value: "--flag"},
		},
	}

	descriptors, err := projectManifests([]publishedProviderManifest{manifest})
	if err != nil {
		t.Fatalf("projectManifests() = %v", err)
	}
	got := descriptors[0]
	if got.TechnicalSupportLevel != providers.TechnicalSupportProduction || got.ImplementationAvailability != providers.ImplementationBundled {
		t.Fatalf("publication posture = %q/%q, want production/bundled", got.TechnicalSupportLevel, got.ImplementationAvailability)
	}
	if got.Models[0].ID != "gpt-5.6" {
		t.Fatalf("model ID = %q, want gpt-5.6", got.Models[0].ID)
	}
	if !reflect.DeepEqual(got.Models[0].Efforts, []providers.ReasoningEffort{"minimal", "low", "max"}) {
		t.Fatalf("efforts = %v, want minimal, low, max", got.Models[0].Efforts)
	}
	wantModalities := []providers.Modality{
		{Direction: providers.ModalityDirectionInput, Kind: providers.ModalityText, Support: providers.ModalitySupported, Transport: providers.ModalityTransportInline},
		{Direction: providers.ModalityDirectionInput, Kind: providers.ModalityVideo, Support: providers.ModalityUnsupported, Transport: providers.ModalityTransportNone},
		{Direction: providers.ModalityDirectionOutput, Kind: providers.ModalityText, Support: providers.ModalitySupported, Transport: providers.ModalityTransportInline},
	}
	if !reflect.DeepEqual(got.Models[0].Modalities, wantModalities) {
		t.Fatalf("modalities = %#v, want %#v", got.Models[0].Modalities, wantModalities)
	}
	if got.Tools[0].Name != "filesystem" || got.Tools[1].Name != "web_search" {
		t.Fatalf("tools = %#v, want filesystem then web_search", got.Tools)
	}
	if got.KnownLimits[0].Name != "a_behavior" || got.KnownLimits[1].Name != "z_limit" {
		t.Fatalf("known limits = %#v, want name order", got.KnownLimits)
	}

	cloned := got.Clone()
	cloned.Models[0].Efforts[0] = "mutated"
	cloned.Models[0].Modalities[0].Kind = providers.ModalityAudio
	cloned.Tools[0].Name = "mutated"
	*cloned.KnownLimits[1].Maximum = 99
	if got.Models[0].Efforts[0] == "mutated" || got.Models[0].Modalities[0].Kind == providers.ModalityAudio || got.Tools[0].Name == "mutated" || *got.KnownLimits[1].Maximum != 5 {
		t.Fatal("descriptor clone shares nested capability facts")
	}
}

func TestProjectStructuredPrerequisitesReplaceLegacyExecutableDuplicate(t *testing.T) {
	t.Parallel()

	manifest := publishedProviderManifest{
		ID:                         "antigravity",
		DisplayName:                publishedNameValue{Value: "Antigravity"},
		TechnicalSupportLevel:      "experimental",
		ImplementationAvailability: "bundled",
		Discovery: publishedProviderDiscovery{
			ExecutableNames: []string{"agy"},
			EndpointKinds:   []string{"stdio"},
			Prerequisites: []publishedPrerequisite{
				{Kind: "authentication", Name: "account-authentication", Description: "Authenticate first."},
				{Kind: "executable", Name: "agy", Description: "Use AGY."},
				{Kind: "workspace", Name: "writable-workspace", Description: "Use a writable workspace."},
			},
		},
	}

	descriptors, err := projectManifests([]publishedProviderManifest{manifest})
	if err != nil {
		t.Fatalf("projectManifests() = %v", err)
	}
	prerequisites := descriptors[0].Prerequisites
	if len(prerequisites) != 4 {
		t.Fatalf("len(prerequisites) = %d, want endpoint plus three structured facts", len(prerequisites))
	}
	for _, prerequisite := range prerequisites {
		if prerequisite.Kind == providers.PrerequisiteDependency && prerequisite.Name == "agy" {
			t.Fatal("legacy AGY executable prerequisite duplicated the structured executable fact")
		}
	}
}
