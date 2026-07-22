package fixtures_test

import (
	"os"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/execution/fixtures"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/fileeffects"
)

func TestPublishedFixtureScenarios_DocumentStableIdentity(t *testing.T) {
	identities, err := fixtures.LoadFixtureScenarioIdentities(contractFixtureCatalogPath(t), fileeffects.ContractFixtureReader(os.ReadFile))
	if err != nil {
		t.Fatalf("fixtures.LoadFixtureScenarioIdentities: %v", err)
	}

	for _, tc := range stableFixtureIdentityCases() {
		t.Run(string(tc.purpose), func(t *testing.T) {
			identity, ok := identities[tc.scenarioID]
			if !ok {
				t.Fatalf("scenario %q missing from catalog", tc.scenarioID)
			}
			if identity.RequestID != tc.requestID {
				t.Fatalf("requestId = %q, want %q", identity.RequestID, tc.requestID)
			}
			if identity.SessionID != tc.sessionID {
				t.Fatalf("sessionId = %q, want %q", identity.SessionID, tc.sessionID)
			}
			if identity.LifecycleStatus != tc.lifecycleStatus {
				t.Fatalf("lifecycleStatus = %q, want %q", identity.LifecycleStatus, tc.lifecycleStatus)
			}
			if identity.ResultStatus != tc.resultStatus {
				t.Fatalf("resultStatus = %q, want %q", identity.ResultStatus, tc.resultStatus)
			}
			if identity.ProjectionHash != tc.projectionHash {
				t.Fatalf("projectionHash = %q, want %q", identity.ProjectionHash, tc.projectionHash)
			}
			assertStringSliceEqual(t, "dispatchIds", identity.DispatchIDs, tc.dispatchIDs)
			assertStringSliceEqual(t, "artifactIds", identity.ArtifactIDs, tc.artifactIDs)
			assertStringSliceEqual(t, "eventIds", identity.EventIDs, tc.eventIDs)
		})
	}
}

func TestPublishedFixtureScenarios_MatchExportedCatalogRows(t *testing.T) {
	identities, err := fixtures.LoadFixtureScenarioIdentities(contractFixtureCatalogPath(t), fileeffects.ContractFixtureReader(os.ReadFile))
	if err != nil {
		t.Fatalf("fixtures.LoadFixtureScenarioIdentities: %v", err)
	}
	hydrated := fixtures.HydratePublishedFixtureProjectionHashes(identities)
	if len(hydrated) != len(fixtures.PublishedFixtureScenarios) {
		t.Fatalf("hydrated rows = %d, want %d", len(hydrated), len(fixtures.PublishedFixtureScenarios))
	}
	for index, row := range fixtures.PublishedFixtureScenarios {
		hydratedRow := hydrated[index]
		if row.Purpose != hydratedRow.Purpose || row.ScenarioID != hydratedRow.ScenarioID {
			t.Fatalf("catalog row mismatch at %d: %#v vs %#v", index, row, hydratedRow)
		}
		if hydratedRow.ProjectionHash == "" {
			t.Fatalf("projection hash missing for purpose %q", row.Purpose)
		}
	}
}

func TestLoadFixtureScenarioIdentities_ReloadIsStable(t *testing.T) {
	path := contractFixtureCatalogPath(t)
	first, err := fixtures.LoadFixtureScenarioIdentities(path, fileeffects.ContractFixtureReader(os.ReadFile))
	if err != nil {
		t.Fatalf("first load: %v", err)
	}
	second, err := fixtures.LoadFixtureScenarioIdentities(path, fileeffects.ContractFixtureReader(os.ReadFile))
	if err != nil {
		t.Fatalf("second load: %v", err)
	}
	if len(first) != len(second) {
		t.Fatalf("scenario count mismatch: %d vs %d", len(first), len(second))
	}
	for scenarioID, firstIdentity := range first {
		secondIdentity, ok := second[scenarioID]
		if !ok {
			t.Fatalf("scenario %q missing on reload", scenarioID)
		}
		if firstIdentity.ScenarioID != secondIdentity.ScenarioID ||
			firstIdentity.RequestID != secondIdentity.RequestID ||
			firstIdentity.SessionID != secondIdentity.SessionID ||
			firstIdentity.LifecycleStatus != secondIdentity.LifecycleStatus ||
			firstIdentity.ResultStatus != secondIdentity.ResultStatus ||
			firstIdentity.ProjectionHash != secondIdentity.ProjectionHash {
			t.Fatalf("identity drift for %q:\nfirst=%#v\nsecond=%#v", scenarioID, firstIdentity, secondIdentity)
		}
		assertStringSliceEqual(t, "reload dispatchIds", firstIdentity.DispatchIDs, secondIdentity.DispatchIDs)
		assertStringSliceEqual(t, "reload artifactIds", firstIdentity.ArtifactIDs, secondIdentity.ArtifactIDs)
		assertStringSliceEqual(t, "reload eventIds", firstIdentity.EventIDs, secondIdentity.EventIDs)
	}
}
