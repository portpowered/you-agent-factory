package service_test

import (
	"strings"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	internalservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog/internal/service"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestProjectAvailabilityPostures(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		manifest      factoryapi.ProviderManifest
		wantAvail     providers.Availability
		wantReadiness providers.Readiness
	}{
		{
			name: "not-supported",
			manifest: factoryapi.ProviderManifest{
				Id:                         "blocked",
				TechnicalSupportLevel:      factoryapi.ProviderTechnicalSupportLevelNotSupported,
				ImplementationAvailability: factoryapi.ProviderImplementationAvailabilityBundled,
			},
			wantAvail:     providers.AvailabilityNotSupported,
			wantReadiness: providers.ReadinessUnavailable,
		},
		{
			name: "catalog-only",
			manifest: factoryapi.ProviderManifest{
				Id:                         "catalog-only",
				TechnicalSupportLevel:      factoryapi.ProviderTechnicalSupportLevelExperimental,
				ImplementationAvailability: factoryapi.ProviderImplementationAvailabilityCatalogOnly,
			},
			wantAvail:     providers.AvailabilityCatalogOnly,
			wantReadiness: providers.ReadinessUnavailable,
		},
		{
			name: "selectable bundled",
			manifest: factoryapi.ProviderManifest{
				Id:                         "selectable",
				TechnicalSupportLevel:      factoryapi.ProviderTechnicalSupportLevelProduction,
				ImplementationAvailability: factoryapi.ProviderImplementationAvailabilityBundled,
			},
			wantAvail:     providers.AvailabilitySelectable,
			wantReadiness: providers.ReadinessReady,
		},
		{
			name: "selectable externally supplied",
			manifest: factoryapi.ProviderManifest{
				Id:                         "external",
				TechnicalSupportLevel:      factoryapi.ProviderTechnicalSupportLevelExperimental,
				ImplementationAvailability: factoryapi.ProviderImplementationAvailabilityExternallySupplied,
			},
			wantAvail:     providers.AvailabilitySelectable,
			wantReadiness: providers.ReadinessReady,
		},
		{
			name: "supported-but-unavailable default",
			manifest: factoryapi.ProviderManifest{
				Id:                         "unsupported-impl",
				TechnicalSupportLevel:      factoryapi.ProviderTechnicalSupportLevelExperimental,
				ImplementationAvailability: factoryapi.ProviderImplementationAvailability("unknown"),
			},
			wantAvail:     providers.AvailabilitySupportedButUnavailable,
			wantReadiness: providers.ReadinessUnavailable,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			descriptors, err := internalservice.ProjectManifestsForTest([]factoryapi.ProviderManifest{testCase.manifest})
			if err != nil {
				t.Fatalf("ProjectManifestsForTest() = %v", err)
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

	manifest := factoryapi.ProviderManifest{
		Id: "cursor",
		DisplayName: factoryapi.NameValue{
			Type:  factoryapi.LOCALIZABLEASSET,
			Value: "Cursor CLI",
		},
		Discovery: factoryapi.ProviderDiscoveryPrerequisites{
			ConfigurationKeys: []string{"CURSOR_API_KEY"},
			EndpointKinds: []factoryapi.ProviderDiscoveryEndpointKind{
				factoryapi.ProviderDiscoveryEndpointKindStdio,
			},
			ExecutableNames: []string{"agent"},
		},
		TechnicalSupportLevel:      factoryapi.ProviderTechnicalSupportLevelExperimental,
		ImplementationAvailability: factoryapi.ProviderImplementationAvailabilityBundled,
	}

	descriptors, err := internalservice.ProjectManifestsForTest([]factoryapi.ProviderManifest{manifest})
	if err != nil {
		t.Fatalf("ProjectManifestsForTest() = %v", err)
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
