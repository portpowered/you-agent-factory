package service

import (
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
