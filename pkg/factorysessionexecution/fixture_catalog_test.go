package factorysessionexecution

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func contractFixtureCatalogPath(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "api", "testdata", "durable-session-contract-fixtures.json")
}

func TestPublishedFixtureScenarios_DocumentStableIdentity(t *testing.T) {
	identities, err := LoadFixtureScenarioIdentities(contractFixtureCatalogPath(t))
	if err != nil {
		t.Fatalf("LoadFixtureScenarioIdentities: %v", err)
	}

	cases := []struct {
		purpose         FixtureScenarioPurpose
		scenarioID      string
		requestID       string
		sessionID       string
		lifecycleStatus LifecycleStatus
		resultStatus    ResultStatus
		projectionHash  string
		dispatchIDs     []string
		artifactIDs     []string
		eventIDs        []string
	}{
		{
			purpose:         FixturePurposeValidationFailure,
			scenarioID:      FixtureScenarioValidationFailure,
			requestID:       "req-missing-source-001",
			sessionID:       "dur-sess-missing-source-001",
			lifecycleStatus: LifecycleStatusFailed,
			resultStatus:    ResultStatusUnavailable,
			projectionHash:  "sha256:58291583771069f4b4572f667f24c5cd70294f70b2f01300a50dcc106608a8a7",
		},
		{
			purpose:         FixturePurposeAsyncRunning,
			scenarioID:      FixtureScenarioAsyncRunning,
			requestID:       "req-js-run-n-001",
			sessionID:       "dur-sess-js-run-n-001",
			lifecycleStatus: LifecycleStatusRunning,
			resultStatus:    ResultStatusPartial,
			projectionHash:  "sha256:5e5d07440546bcfe8c67a0b270b27bdaeec8158346a61ac98273441f60f2e0ca",
			dispatchIDs:     []string{"disp-js-001", "disp-js-002", "disp-js-003"},
			eventIDs: []string{
				"session-result-updated/dur-sess-js-run-n-001",
				"session-started/dur-sess-js-run-n-001",
			},
		},
		{
			purpose:         FixturePurposeSyncSuccess,
			scenarioID:      FixtureScenarioSyncSuccess,
			requestID:       "req-petri-success-001",
			sessionID:       "dur-sess-petri-success-001",
			lifecycleStatus: LifecycleStatusSucceeded,
			resultStatus:    ResultStatusFinal,
			projectionHash:  "sha256:80683379f0ad28cb98d0adce69606d3d7fa249df7e2dd45300517bd5be1cf064",
			dispatchIDs:     []string{"disp-petri-success-001"},
			artifactIDs:     []string{"art-petri-final-001"},
		},
		{
			purpose:         FixturePurposeSyncTimeout,
			scenarioID:      FixtureScenarioSyncTimeout,
			requestID:       "req-js-timeout-001",
			sessionID:       "dur-sess-js-timeout-001",
			lifecycleStatus: LifecycleStatusRunning,
			resultStatus:    ResultStatusNotReady,
			projectionHash:  "sha256:798e5ae557b537dd488032e7fb545f9a2bdd20a9e7e646d43ed1d258758d261c",
			dispatchIDs:     []string{"disp-js-timeout-001"},
		},
		{
			purpose:         FixturePurposeFailedRecoverable,
			scenarioID:      FixtureScenarioFailedRecoverable,
			requestID:       "req-js-interrupted-001",
			sessionID:       "dur-sess-js-interrupted-001",
			lifecycleStatus: LifecycleStatusInterrupted,
			resultStatus:    ResultStatusPartial,
			projectionHash:  "sha256:3b7a2fa4485d9f0e51f9dc7ad328cf6d390fd84d98fe06d3b00da0527573704f",
			dispatchIDs:     []string{"disp-js-interrupted-001", "disp-js-interrupted-002"},
		},
		{
			purpose:         FixturePurposeDispatchInspection,
			scenarioID:      FixtureScenarioDispatchInspection,
			requestID:       "req-petri-success-001",
			sessionID:       "dur-sess-petri-success-001",
			lifecycleStatus: LifecycleStatusSucceeded,
			resultStatus:    ResultStatusFinal,
			projectionHash:  "sha256:80683379f0ad28cb98d0adce69606d3d7fa249df7e2dd45300517bd5be1cf064",
			dispatchIDs:     []string{"disp-petri-success-001"},
			artifactIDs:     []string{"art-petri-final-001"},
		},
		{
			purpose:         FixturePurposeArtifactInspection,
			scenarioID:      FixtureScenarioArtifactInspection,
			requestID:       "req-js-paused-001",
			sessionID:       "dur-sess-js-paused-001",
			lifecycleStatus: LifecycleStatusPaused,
			resultStatus:    ResultStatusPartial,
			projectionHash:  "sha256:56cf36fbe81354e200dbb63c299c30de1d059a6d233fd6c977d956c6a646868c",
			dispatchIDs:     []string{"disp-js-pause-001", "disp-js-pause-002"},
			artifactIDs:     []string{"art-js-pause-001"},
		},
		{
			purpose:         FixturePurposeEventReconnect,
			scenarioID:      FixtureScenarioEventReconnect,
			requestID:       "req-js-run-n-001",
			sessionID:       "dur-sess-js-run-n-001",
			lifecycleStatus: LifecycleStatusRunning,
			resultStatus:    ResultStatusPartial,
			projectionHash:  "sha256:5e5d07440546bcfe8c67a0b270b27bdaeec8158346a61ac98273441f60f2e0ca",
			dispatchIDs:     []string{"disp-js-001", "disp-js-002", "disp-js-003"},
			eventIDs: []string{
				"session-result-updated/dur-sess-js-run-n-001",
				"session-started/dur-sess-js-run-n-001",
			},
		},
		{
			purpose:         FixturePurposeLifecycleControl,
			scenarioID:      FixtureScenarioLifecycleControl,
			requestID:       "req-js-paused-001",
			sessionID:       "dur-sess-js-paused-001",
			lifecycleStatus: LifecycleStatusPaused,
			resultStatus:    ResultStatusPartial,
			projectionHash:  "sha256:56cf36fbe81354e200dbb63c299c30de1d059a6d233fd6c977d956c6a646868c",
			dispatchIDs:     []string{"disp-js-pause-001", "disp-js-pause-002"},
			artifactIDs:     []string{"art-js-pause-001"},
		},
	}

	for _, tc := range cases {
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
	identities, err := LoadFixtureScenarioIdentities(contractFixtureCatalogPath(t))
	if err != nil {
		t.Fatalf("LoadFixtureScenarioIdentities: %v", err)
	}
	hydrated := HydratePublishedFixtureProjectionHashes(identities)
	if len(hydrated) != len(PublishedFixtureScenarios) {
		t.Fatalf("hydrated rows = %d, want %d", len(hydrated), len(PublishedFixtureScenarios))
	}
	for index, row := range PublishedFixtureScenarios {
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
	first, err := LoadFixtureScenarioIdentities(path)
	if err != nil {
		t.Fatalf("first load: %v", err)
	}
	second, err := LoadFixtureScenarioIdentities(path)
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

func TestContractFixtureCatalog_UsesCanonicalFactorySessionVocabulary(t *testing.T) {
	raw, err := os.ReadFile(contractFixtureCatalogPath(t))
	if err != nil {
		t.Fatalf("read fixtures: %v", err)
	}
	text := string(raw)
	forbidden := []string{"DynamicWorkflowRun", "workflow run"}
	for _, term := range forbidden {
		if strings.Contains(text, term) {
			t.Fatalf("fixture catalog contains forbidden term %q", term)
		}
	}

	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("decode fixtures: %v", err)
	}
	scenarios, ok := document["scenarios"].([]any)
	if !ok {
		t.Fatal("missing scenarios array")
	}
	for _, item := range scenarios {
		row, ok := item.(map[string]any)
		if !ok {
			continue
		}
		encoded, err := json.Marshal(row)
		if err != nil {
			t.Fatalf("marshal scenario: %v", err)
		}
		payload := string(encoded)
		for _, required := range []string{"sessionId", "session", "executionRequest"} {
			if !strings.Contains(payload, required) {
				t.Fatalf("scenario %v missing %q in public fixture fields", row["id"], required)
			}
		}
	}
}

func assertStringSliceEqual(t *testing.T, label string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s = %#v, want %#v", label, got, want)
	}
	for index := range got {
		if got[index] != want[index] {
			t.Fatalf("%s[%d] = %q, want %q", label, index, got[index], want[index])
		}
	}
}
