package factorysessionexecution

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
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
	for _, term := range ForbiddenFixtureVocabularyTerms() {
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
func publishedScenarioByPurpose(t *testing.T, purpose FixtureScenarioPurpose) PublishedFixtureScenario {
	t.Helper()
	for _, row := range PublishedFixtureScenarios {
		if row.Purpose == purpose {
			return row
		}
	}
	t.Fatalf("published scenario missing for purpose %q", purpose)
	return PublishedFixtureScenario{}
}

func startRequestForPublished(row PublishedFixtureScenario) StartRequest {
	switch row.RequestID {
	case "req-js-timeout-001":
		return StartRequest{
			RequestID: row.RequestID,
			Source: Source{
				Kind:         workflowsource.KindWorkflowName,
				WorkflowName: "long-running-audit",
			},
			Wait: &WaitOptions{TimeoutMillis: int64Ptr(30000)},
		}
	case "req-idempotent-replay-001":
		return StartRequest{
			RequestID: row.RequestID,
			Source: Source{
				Kind:         workflowsource.KindWorkflowFile,
				WorkflowFile: ".claude/workflows/idempotent.yaml",
			},
			Args: map[string]any{"task": "replay"},
			RequestedPolicy: map[string]any{
				"policyHash": "req-policy-idempotent",
			},
		}
	default:
		return StartRequest{
			RequestID: row.RequestID,
			Source: Source{
				Kind:      workflowsource.KindFactoryID,
				FactoryID: "customer-support-triage",
			},
		}
	}
}

func TestFakeService_PublishedScenarios_AsyncStartInspectionLinksAndEventPrefix(t *testing.T) {
	service := newContractFakeService(t)
	row := publishedScenarioByPurpose(t, FixturePurposeAsyncRunning)

	started, err := service.StartAsync(context.Background(), startRequestForPublished(row))
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	if started.SessionID != row.SessionID {
		t.Fatalf("sessionId = %q, want %q", started.SessionID, row.SessionID)
	}
	if started.Status != string(LifecycleStatusRunning) {
		t.Fatalf("status = %q, want RUNNING", started.Status)
	}
	if started.Links.Session != "/factory-sessions/"+row.SessionID {
		t.Fatalf("session link = %q", started.Links.Session)
	}
	if started.Links.Results != "/factory-sessions/"+row.SessionID+"/results" {
		t.Fatalf("results link = %q", started.Links.Results)
	}

	events, err := service.ReadEvents(context.Background(), row.SessionID, EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	if len(events.Events) == 0 {
		t.Fatal("event prefix missing")
	}
	assertCanonicalEventEnvelope(t, events.Events[0], "SESSION_STARTED", "session-started/"+row.SessionID)
}

func TestFakeService_PublishedScenarios_SyncStartTerminalAndTimeout(t *testing.T) {
	service := newContractFakeService(t)

	successRow := publishedScenarioByPurpose(t, FixturePurposeSyncSuccess)
	terminal, err := service.StartSync(context.Background(), startRequestForPublished(successRow))
	if err != nil {
		t.Fatalf("StartSync success: %v", err)
	}
	if terminal.SyncOutcome != SyncOutcomeCompleted {
		t.Fatalf("syncOutcome = %q, want COMPLETED", terminal.SyncOutcome)
	}
	if terminal.Status != string(LifecycleStatusSucceeded) {
		t.Fatalf("status = %q, want SUCCEEDED", terminal.Status)
	}
	terminalHash, err := SyncStartResultHash(terminal)
	if err != nil {
		t.Fatalf("SyncStartResultHash: %v", err)
	}
	if terminalHash != "sha256:89b3a278be3192017c6fcd9fbd4ca57154fb84ab6154ce961e4a597ba5fa6c05" {
		t.Fatalf("sync success hash = %q, want sha256:89b3a278be3192017c6fcd9fbd4ca57154fb84ab6154ce961e4a597ba5fa6c05", terminalHash)
	}

	timeoutRow := publishedScenarioByPurpose(t, FixturePurposeSyncTimeout)
	timedOut, err := service.StartSync(context.Background(), startRequestForPublished(timeoutRow))
	if err != nil {
		t.Fatalf("StartSync timeout: %v", err)
	}
	if timedOut.SyncOutcome != SyncOutcomeTimedOut || !timedOut.TimedOut {
		t.Fatalf("timeout response = %#v", timedOut)
	}
	if timedOut.SessionID != timeoutRow.SessionID {
		t.Fatalf("sessionId = %q, want %q", timedOut.SessionID, timeoutRow.SessionID)
	}
	timeoutHash, err := SyncStartResultHash(timedOut)
	if err != nil {
		t.Fatalf("SyncStartResultHash: %v", err)
	}
	if timeoutHash != "sha256:58f35deb8326d37ab4048ebf7f1d7d6f6a994e04b82de300a61f52f7f72e5378" {
		t.Fatalf("sync timeout hash = %q, want sha256:58f35deb8326d37ab4048ebf7f1d7d6f6a994e04b82de300a61f52f7f72e5378", timeoutHash)
	}
}

func TestFakeService_PublishedScenarios_ListSessionsScopedWithDedup(t *testing.T) {
	service := newContractFakeService(t)
	runningRow := publishedScenarioByPurpose(t, FixturePurposeAsyncRunning)
	successRow := publishedScenarioByPurpose(t, FixturePurposeSyncSuccess)

	if _, err := service.StartAsync(context.Background(), startRequestForPublished(runningRow)); err != nil {
		t.Fatalf("StartAsync running: %v", err)
	}
	if _, err := service.StartSync(context.Background(), startRequestForPublished(successRow)); err != nil {
		t.Fatalf("StartSync success: %v", err)
	}

	live, err := service.ListSessions(context.Background(), ListSessionsRequest{Scope: SessionListScopeLive})
	if err != nil {
		t.Fatalf("ListSessions live: %v", err)
	}
	if len(live.DurableSessions) != 0 {
		t.Fatalf("live durable rows = %#v, want none", live.DurableSessions)
	}
	if !containsLiveSessionID(live.LiveSessions, runningRow.SessionID) {
		t.Fatalf("live sessions = %#v, want running row %q", live.LiveSessions, runningRow.SessionID)
	}
	if !containsLiveSessionID(live.LiveSessions, successRow.SessionID) {
		t.Fatalf("live sessions = %#v, want terminal row %q", live.LiveSessions, successRow.SessionID)
	}

	persisted, err := service.ListSessions(context.Background(), ListSessionsRequest{Scope: SessionListScopePersisted})
	if err != nil {
		t.Fatalf("ListSessions persisted: %v", err)
	}
	if containsDurableSessionID(persisted.DurableSessions, runningRow.SessionID) {
		t.Fatalf("persisted rows unexpectedly contain running session %q", runningRow.SessionID)
	}
	if !containsDurableSessionID(persisted.DurableSessions, successRow.SessionID) {
		t.Fatalf("persisted rows = %#v, want terminal row %q", persisted.DurableSessions, successRow.SessionID)
	}

	all, err := service.ListSessions(context.Background(), ListSessionsRequest{Scope: SessionListScopeAll})
	if err != nil {
		t.Fatalf("ListSessions all: %v", err)
	}
	if containsLiveSessionID(all.LiveSessions, successRow.SessionID) {
		t.Fatalf("all-scope live rows still contain deduped id %q", successRow.SessionID)
	}
	if !containsLiveSessionID(all.LiveSessions, runningRow.SessionID) {
		t.Fatalf("all-scope live rows = %#v, want running row %q", all.LiveSessions, runningRow.SessionID)
	}
	if !containsDurableSessionID(all.DurableSessions, successRow.SessionID) {
		t.Fatalf("all-scope durable rows = %#v, want terminal row %q", all.DurableSessions, successRow.SessionID)
	}
}

func TestFakeService_PublishedScenarios_GetSessionReadModels(t *testing.T) {
	service := newContractFakeService(t)
	cases := []struct {
		purpose FixtureScenarioPurpose
		sync    bool
	}{
		{FixturePurposeAsyncRunning, false},
		{FixturePurposeSyncSuccess, true},
		{FixturePurposeSyncTimeout, true},
		{FixturePurposeFailedRecoverable, false},
	}
	for _, tc := range cases {
		row := publishedScenarioByPurpose(t, tc.purpose)
		t.Run(string(tc.purpose), func(t *testing.T) {
			req := startRequestForPublished(row)
			if tc.sync {
				if _, err := service.StartSync(context.Background(), req); err != nil {
					t.Fatalf("StartSync: %v", err)
				}
			} else if _, err := service.StartAsync(context.Background(), req); err != nil {
				t.Fatalf("StartAsync: %v", err)
			}

			read, err := service.GetSession(context.Background(), row.SessionID)
			if err != nil {
				t.Fatalf("GetSession: %v", err)
			}
			if read.SessionID != row.SessionID {
				t.Fatalf("sessionId = %q, want %q", read.SessionID, row.SessionID)
			}
			if read.Status != row.LifecycleStatus {
				t.Fatalf("status = %q, want %q", read.Status, row.LifecycleStatus)
			}
			if read.Links.Session != "/factory-sessions/"+row.SessionID {
				t.Fatalf("session link = %q", read.Links.Session)
			}
			if read.ResultSummary != nil && read.ResultSummary.ResultStatus != string(row.ResultStatus) {
				t.Fatalf("resultSummary status = %q, want %q", read.ResultSummary.ResultStatus, row.ResultStatus)
			}
			result, err := service.GetResult(context.Background(), row.SessionID, ResultRequest{Mode: ResultModePartial})
			if tc.purpose == FixturePurposeSyncSuccess || tc.purpose == FixturePurposeSyncTimeout {
				result, err = service.GetResult(context.Background(), row.SessionID, ResultRequest{Mode: ResultModeFinal})
			}
			if err != nil {
				t.Fatalf("GetResult: %v", err)
			}
			if result.ResultStatus != row.ResultStatus {
				t.Fatalf("resultStatus = %q, want %q", result.ResultStatus, row.ResultStatus)
			}
		})
	}
}

func TestFakeService_PublishedScenarios_ResultReadsWithStableHash(t *testing.T) {
	service := newContractFakeService(t)
	cases := []struct {
		purpose         FixtureScenarioPurpose
		sync            bool
		resultRequest   ResultRequest
		wantHash        string
		wantAvailability string
	}{
		{
			purpose:       FixturePurposeAsyncRunning,
			resultRequest: ResultRequest{Mode: ResultModePartial},
			wantHash:      "sha256:f4830cd3534f5de6491b04dd4c05b2b1e01cf73844877ad922ea7d6547ae07f6",
		},
		{
			purpose:       FixturePurposeSyncSuccess,
			sync:          true,
			resultRequest: ResultRequest{Mode: ResultModeFinal, IncludeArtifacts: false},
			wantHash:      "sha256:977772c884f0ec53b9292ca8fa0374fec1673fec8d0d481e358b3dd4ae65fb95",
		},
		{
			purpose:          FixturePurposeSyncTimeout,
			sync:             true,
			resultRequest:    ResultRequest{Mode: ResultModeFinal},
			wantHash:         "sha256:ab30784fe4f173cd457d0fe83d90425eeee0212ce2942869886a31316d70b4ba",
			wantAvailability: "SYNC_WAIT_TIMED_OUT",
		},
		{
			purpose:       FixturePurposeFailedRecoverable,
			resultRequest: ResultRequest{Mode: ResultModePartial},
			wantHash:      "sha256:266b2572ecbf4d6e87f9143ac2852866069365beefe329e886a6827ff0de3746",
		},
	}
	for _, tc := range cases {
		row := publishedScenarioByPurpose(t, tc.purpose)
		t.Run(string(tc.purpose), func(t *testing.T) {
			req := startRequestForPublished(row)
			if tc.sync {
				if _, err := service.StartSync(context.Background(), req); err != nil {
					t.Fatalf("StartSync: %v", err)
				}
			} else if _, err := service.StartAsync(context.Background(), req); err != nil {
				t.Fatalf("StartAsync: %v", err)
			}

			result, err := service.GetResult(context.Background(), row.SessionID, tc.resultRequest)
			if err != nil {
				t.Fatalf("GetResult: %v", err)
			}
			if result.ResultStatus != row.ResultStatus {
				t.Fatalf("resultStatus = %q, want %q", result.ResultStatus, row.ResultStatus)
			}
			if tc.wantAvailability != "" {
				if result.Availability == nil || result.Availability.Reason != tc.wantAvailability {
					t.Fatalf("availability = %#v, want reason %q", result.Availability, tc.wantAvailability)
				}
			}
			hash, err := ProjectedResultReadHash(result)
			if err != nil {
				t.Fatalf("ProjectedResultReadHash: %v", err)
			}
			if hash != tc.wantHash {
				t.Fatalf("result hash = %q, want %q", hash, tc.wantHash)
			}
		})
	}
}

func TestFakeService_PublishedScenarios_ResultArtifactInclusion(t *testing.T) {
	service := newContractFakeService(t)
	row := publishedScenarioByPurpose(t, FixturePurposeSyncSuccess)
	if _, err := service.StartSync(context.Background(), startRequestForPublished(row)); err != nil {
		t.Fatalf("StartSync: %v", err)
	}

	excluded, err := service.GetResult(context.Background(), row.SessionID, ResultRequest{
		Mode:             ResultModeFinal,
		IncludeArtifacts: false,
	})
	if err != nil {
		t.Fatalf("GetResult excluded: %v", err)
	}
	if len(excluded.ArtifactIDs) != 1 || excluded.ArtifactIDs[0] != "art-petri-final-001" {
		t.Fatalf("artifactIds = %#v", excluded.ArtifactIDs)
	}
	if len(excluded.ArtifactRefs) != 0 {
		t.Fatalf("artifactRefs = %#v, want omitted", excluded.ArtifactRefs)
	}

	included, err := service.GetResult(context.Background(), row.SessionID, ResultRequest{
		Mode:             ResultModeFinal,
		IncludeArtifacts: true,
	})
	if err != nil {
		t.Fatalf("GetResult included: %v", err)
	}
	if len(included.ArtifactRefs) != 1 || included.ArtifactRefs[0].ID != "art-petri-final-001" {
		t.Fatalf("artifactRefs = %#v", included.ArtifactRefs)
	}
	if len(included.ArtifactIDs) != 0 {
		t.Fatalf("artifactIds = %#v, want omitted when refs included", included.ArtifactIDs)
	}
}

func TestFakeService_PublishedScenarios_StartIdempotentReplay(t *testing.T) {
	service := newContractFakeService(t)
	req := startRequestForPublished(PublishedFixtureScenario{
		RequestID: FixtureScenarioIdempotentReplay,
	})
	req.RequestID = "req-idempotent-replay-001"

	first, err := service.StartAsync(context.Background(), req)
	if err != nil {
		t.Fatalf("first StartAsync: %v", err)
	}
	second, err := service.StartAsync(context.Background(), req)
	if err != nil {
		t.Fatalf("replay StartAsync: %v", err)
	}
	if second.SessionID != first.SessionID {
		t.Fatalf("replay sessionId = %q, want %q", second.SessionID, first.SessionID)
	}
	if first.SessionID != "dur-sess-idempotent-001" {
		t.Fatalf("sessionId = %q, want dur-sess-idempotent-001", first.SessionID)
	}

	conflict := req
	conflict.Args = map[string]any{"task": "different"}
	_, err = service.StartAsync(context.Background(), conflict)
	if !errors.Is(err, ErrExecutionRequestIDConflict) {
		t.Fatalf("error = %v, want ErrExecutionRequestIDConflict", err)
	}
}

func containsLiveSessionID(sessions []LiveSessionSummary, sessionID string) bool {
	for _, session := range sessions {
		if session.ID == sessionID {
			return true
		}
	}
	return false
}

func containsDurableSessionID(sessions []DurableSessionListSummary, sessionID string) bool {
	for _, session := range sessions {
		if session.SessionID == sessionID {
			return true
		}
	}
	return false
}

func TestProjectedResultReadHash_IsStableAcrossEquivalentReads(t *testing.T) {
	service := newContractFakeService(t)
	row := publishedScenarioByPurpose(t, FixturePurposeSyncSuccess)
	if _, err := service.StartSync(context.Background(), startRequestForPublished(row)); err != nil {
		t.Fatalf("StartSync: %v", err)
	}
	first, err := service.GetResult(context.Background(), row.SessionID, ResultRequest{Mode: ResultModeFinal})
	if err != nil {
		t.Fatalf("first GetResult: %v", err)
	}
	second, err := service.GetResult(context.Background(), row.SessionID, ResultRequest{Mode: ResultModeFinal})
	if err != nil {
		t.Fatalf("second GetResult: %v", err)
	}
	firstHash, err := ProjectedResultReadHash(first)
	if err != nil {
		t.Fatalf("first hash: %v", err)
	}
	secondHash, err := ProjectedResultReadHash(second)
	if err != nil {
		t.Fatalf("second hash: %v", err)
	}
	if firstHash != secondHash {
		t.Fatalf("hash drift: %q vs %q", firstHash, secondHash)
	}
}
func TestFakeService_PublishedScenarios_ListDispatchesStableSummaries(t *testing.T) {
	service := newContractFakeService(t)
	cases := []struct {
		purpose  FixtureScenarioPurpose
		sync     bool
		wantHash string
		wantIDs  []string
	}{
		{
			purpose:  FixturePurposeDispatchInspection,
			sync:     true,
			wantHash: "sha256:a32d5d0f136dcfef8061746c8f270702163c92a04e3c9f75eb9248e19bebd34a",
			wantIDs:  []string{"disp-petri-success-001"},
		},
		{
			purpose:  FixturePurposeAsyncRunning,
			wantHash: "sha256:51df934ba2b35b5baa20c4b64b1907cf66f109ffbffe2d3e9eedac747b07ded9",
			wantIDs:  []string{"disp-js-001", "disp-js-002", "disp-js-003"},
		},
		{
			purpose:  FixturePurposeArtifactInspection,
			wantHash: "sha256:9387a745d2699e5b22d92b2152183aecf3a8db85966630de7b0899a3f19e504c",
			wantIDs:  []string{"disp-js-pause-001", "disp-js-pause-002"},
		},
	}
	for _, tc := range cases {
		row := publishedScenarioByPurpose(t, tc.purpose)
		t.Run(string(tc.purpose), func(t *testing.T) {
			req := startRequestForPublished(row)
			if tc.sync {
				if _, err := service.StartSync(context.Background(), req); err != nil {
					t.Fatalf("StartSync: %v", err)
				}
			} else if _, err := service.StartAsync(context.Background(), req); err != nil {
				t.Fatalf("StartAsync: %v", err)
			}

			listed, err := service.ListDispatches(context.Background(), row.SessionID)
			if err != nil {
				t.Fatalf("ListDispatches: %v", err)
			}
			if listed.SessionID != row.SessionID {
				t.Fatalf("sessionId = %q, want %q", listed.SessionID, row.SessionID)
			}
			if len(listed.Dispatches) != len(tc.wantIDs) {
				t.Fatalf("dispatches = %#v, want %d rows", listed.Dispatches, len(tc.wantIDs))
			}
			for index, wantID := range tc.wantIDs {
				got := listed.Dispatches[index]
				if got.ID != wantID {
					t.Fatalf("dispatch[%d].id = %q, want %q", index, got.ID, wantID)
				}
				if got.Status == "" || got.DispatchKind == "" {
					t.Fatalf("dispatch[%d] missing status/kind: %#v", index, got)
				}
			}

			read, err := service.GetSession(context.Background(), row.SessionID)
			if err != nil {
				t.Fatalf("GetSession: %v", err)
			}
			if err := ValidateDispatchListMatchesSessionProgress(read, listed.Dispatches); err != nil {
				t.Fatalf("ValidateDispatchListMatchesSessionProgress: %v", err)
			}

			hash, err := ListDispatchesResultHash(listed)
			if err != nil {
				t.Fatalf("ListDispatchesResultHash: %v", err)
			}
			if hash != tc.wantHash {
				t.Fatalf("dispatch list hash = %q, want %q", hash, tc.wantHash)
			}
		})
	}
}

func TestFakeService_PublishedScenarios_GetDispatchDetailAndUnknownError(t *testing.T) {
	service := newContractFakeService(t)
	dispatchRow := publishedScenarioByPurpose(t, FixturePurposeDispatchInspection)
	if _, err := service.StartSync(context.Background(), startRequestForPublished(dispatchRow)); err != nil {
		t.Fatalf("StartSync dispatch scenario: %v", err)
	}

	detail, err := service.GetDispatch(context.Background(), dispatchRow.SessionID, "disp-petri-success-001")
	if err != nil {
		t.Fatalf("GetDispatch: %v", err)
	}
	if detail.SessionID != dispatchRow.SessionID {
		t.Fatalf("sessionId = %q, want %q", detail.SessionID, dispatchRow.SessionID)
	}
	if detail.OrchestratorKind != "PETRI" {
		t.Fatalf("orchestratorKind = %q, want PETRI", detail.OrchestratorKind)
	}
	if detail.Petri == nil || detail.Petri.TransitionID != "transition-plan-task" {
		t.Fatalf("petri projection = %#v", detail.Petri)
	}
	hash, err := DispatchDetailHash(detail)
	if err != nil {
		t.Fatalf("DispatchDetailHash: %v", err)
	}
	if hash != "sha256:0309e245dc0354d3d0083b0d3a083fe9862ac01415d243914098a49c819cf37f" {
		t.Fatalf("dispatch detail hash = %q, want sha256:0309e245dc0354d3d0083b0d3a083fe9862ac01415d243914098a49c819cf37f", hash)
	}

	artifactRow := publishedScenarioByPurpose(t, FixturePurposeArtifactInspection)
	if _, err := service.StartAsync(context.Background(), startRequestForPublished(artifactRow)); err != nil {
		t.Fatalf("StartAsync artifact scenario: %v", err)
	}
	jsDetail, err := service.GetDispatch(context.Background(), artifactRow.SessionID, "disp-js-pause-002")
	if err != nil {
		t.Fatalf("GetDispatch javascript detail: %v", err)
	}
	if jsDetail.JavaScript == nil || jsDetail.JavaScript.TaskKind != "SYSTEM" {
		t.Fatalf("javascript projection = %#v", jsDetail.JavaScript)
	}

	_, err = service.GetDispatch(context.Background(), dispatchRow.SessionID, "missing-dispatch-id")
	if !errors.Is(err, ErrDispatchNotFound) {
		t.Fatalf("unknown dispatch error = %v, want ErrDispatchNotFound", err)
	}
}

func TestFakeService_PublishedScenarios_ListArtifactsStableSummaries(t *testing.T) {
	service := newContractFakeService(t)
	cases := []struct {
		purpose  FixtureScenarioPurpose
		sync     bool
		wantHash string
		wantIDs  []string
	}{
		{
			purpose:  FixturePurposeDispatchInspection,
			sync:     true,
			wantHash: "sha256:c42d891189b507df18e127e6cf10deeacf3d56a97c48786491d0ddfd3ed65fce",
			wantIDs:  []string{"art-petri-final-001"},
		},
		{
			purpose:  FixturePurposeArtifactInspection,
			wantHash: "sha256:57fa7af131ce29cb2a254d2548ef8b8f9b0ccf6de7fb6cc185beabf8190f1dcb",
			wantIDs:  []string{"art-js-pause-001"},
		},
	}
	for _, tc := range cases {
		row := publishedScenarioByPurpose(t, tc.purpose)
		t.Run(string(tc.purpose), func(t *testing.T) {
			req := startRequestForPublished(row)
			if tc.sync {
				if _, err := service.StartSync(context.Background(), req); err != nil {
					t.Fatalf("StartSync: %v", err)
				}
			} else if _, err := service.StartAsync(context.Background(), req); err != nil {
				t.Fatalf("StartAsync: %v", err)
			}

			listed, err := service.ListArtifacts(context.Background(), row.SessionID)
			if err != nil {
				t.Fatalf("ListArtifacts: %v", err)
			}
			if listed.SessionID != row.SessionID {
				t.Fatalf("sessionId = %q, want %q", listed.SessionID, row.SessionID)
			}
			if len(listed.Artifacts) != len(tc.wantIDs) {
				t.Fatalf("artifacts = %#v, want %d rows", listed.Artifacts, len(tc.wantIDs))
			}
			for index, wantID := range tc.wantIDs {
				got := listed.Artifacts[index]
				if got.ID != wantID {
					t.Fatalf("artifact[%d].id = %q, want %q", index, got.ID, wantID)
				}
				if got.Kind == "" || got.ContentHash == "" {
					t.Fatalf("artifact[%d] missing kind/contentHash: %#v", index, got)
				}
				if got.RetrievalRef == nil || got.RetrievalRef.Href == "" {
					t.Fatalf("artifact[%d] missing retrieval ref: %#v", index, got)
				}
				wantHref := "/factory-sessions/" + row.SessionID + "/artifacts/" + wantID
				if got.RetrievalRef.Href != wantHref {
					t.Fatalf("retrieval href = %q, want %q", got.RetrievalRef.Href, wantHref)
				}
			}

			hash, err := ListArtifactsResultHash(listed)
			if err != nil {
				t.Fatalf("ListArtifactsResultHash: %v", err)
			}
			if hash != tc.wantHash {
				t.Fatalf("artifact list hash = %q, want %q", hash, tc.wantHash)
			}
		})
	}
}

func TestFakeService_PublishedScenarios_GetArtifactDetailAndUnknownError(t *testing.T) {
	service := newContractFakeService(t)
	row := publishedScenarioByPurpose(t, FixturePurposeArtifactInspection)
	if _, err := service.StartAsync(context.Background(), startRequestForPublished(row)); err != nil {
		t.Fatalf("StartAsync: %v", err)
	}

	detail, err := service.GetArtifact(context.Background(), row.SessionID, "art-js-pause-001")
	if err != nil {
		t.Fatalf("GetArtifact: %v", err)
	}
	if detail.SessionID != row.SessionID {
		t.Fatalf("sessionId = %q, want %q", detail.SessionID, row.SessionID)
	}
	if detail.DispatchID != "disp-js-pause-001" {
		t.Fatalf("dispatchId = %q, want disp-js-pause-001", detail.DispatchID)
	}
	if detail.ContentRef == nil || detail.ContentRef.Href == "" {
		t.Fatalf("contentRef missing: %#v", detail.ContentRef)
	}
	hash, err := ArtifactDetailHash(detail)
	if err != nil {
		t.Fatalf("ArtifactDetailHash: %v", err)
	}
	if hash != "sha256:0b4d4d6d8483cb9ad7f867019145f069752b2663890689775bbd20325716cf20" {
		t.Fatalf("artifact detail hash = %q, want sha256:0b4d4d6d8483cb9ad7f867019145f069752b2663890689775bbd20325716cf20", hash)
	}

	successRow := publishedScenarioByPurpose(t, FixturePurposeDispatchInspection)
	if _, err := service.StartSync(context.Background(), startRequestForPublished(successRow)); err != nil {
		t.Fatalf("StartSync success: %v", err)
	}
	successDetail, err := service.GetArtifact(context.Background(), successRow.SessionID, "art-petri-final-001")
	if err != nil {
		t.Fatalf("GetArtifact terminal: %v", err)
	}
	if len(successDetail.Content) == 0 {
		t.Fatal("terminal artifact content missing")
	}

	_, err = service.GetArtifact(context.Background(), row.SessionID, "missing-artifact-id")
	if !errors.Is(err, ErrArtifactNotFound) {
		t.Fatalf("unknown artifact error = %v, want ErrArtifactNotFound", err)
	}
}

func TestFakeService_PublishedScenarios_ReadEventsCanonicalAndReconnect(t *testing.T) {
	service := newContractFakeService(t)
	cases := []struct {
		name           string
		requestID      string
		sessionID      string
		sync           bool
		wantCount      int
		wantHash       string
		reconnectAfter string
		wantAfterCount int
	}{
		{
			name:           "running",
			requestID:      "req-js-run-n-001",
			sessionID:      "dur-sess-js-run-n-001",
			wantCount:      2,
			wantHash:       "sha256:11a22ce83ca44464c5a8d90062542e6bf9f16d4350005808795b95df7e461c65",
			reconnectAfter: "session-started/dur-sess-js-run-n-001",
			wantAfterCount: 1,
		},
		{
			name:      "terminal",
			requestID: "req-js-success-002",
			sessionID: "dur-sess-js-success-002",
			wantCount: 3,
			wantHash:  "sha256:956aeb10de9e9e3a8e5ced44d32e1a15c41d770359259ad148d446611e6fce5c",
		},
		{
			name:      "dispatch-inspection",
			requestID: "req-petri-success-001",
			sessionID: "dur-sess-petri-success-001",
			sync:      true,
			wantCount: 3,
			wantHash:  "sha256:9dbb55ddc666ebae19e02b67b3eab9e0e1916241a08341949dec6d5f11f49348",
		},
		{
			name:      "artifact-inspection",
			requestID: "req-js-paused-001",
			sessionID: "dur-sess-js-paused-001",
			wantCount: 2,
			wantHash:  "sha256:4fc92b6cff30745dfe1112fcbbf1bb70fc1f132bdfec25b5b0e39128ac6f054c",
		},
		{
			name:      "awaiting-approval",
			requestID: "req-js-awaiting-001",
			sessionID: "dur-sess-js-awaiting-001",
			wantCount: 2,
			wantHash:  "sha256:330aaa8847dbd0ef3e40b573fbda9354fbd38b075dfb7402360d82fd617f4a40",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := StartRequest{
				RequestID: tc.requestID,
				Source: Source{
					Kind:      workflowsource.KindFactoryID,
					FactoryID: "customer-support-triage",
				},
			}
			if tc.requestID == "req-js-success-002" {
				req.Source = Source{
					Kind:         workflowsource.KindWorkflowFile,
					WorkflowFile: ".claude/workflows/docs-refresh.yaml",
				}
			}
			if tc.requestID == "req-js-awaiting-001" {
				req.Source = Source{
					Kind:         workflowsource.KindWorkflowFile,
					WorkflowFile: ".claude/workflows/policy-gated-release.yaml",
				}
			}
			if tc.sync {
				if _, err := service.StartSync(context.Background(), req); err != nil {
					t.Fatalf("StartSync: %v", err)
				}
			} else if _, err := service.StartAsync(context.Background(), req); err != nil {
				t.Fatalf("StartAsync: %v", err)
			}

			all, err := service.ReadEvents(context.Background(), tc.sessionID, EventReconnectRequest{})
			if err != nil {
				t.Fatalf("ReadEvents: %v", err)
			}
			if len(all.Events) != tc.wantCount {
				t.Fatalf("events = %d, want %d", len(all.Events), tc.wantCount)
			}
			for _, raw := range all.Events {
				assertCanonicalEventEnvelope(t, raw, "", "")
			}
			if tc.wantHash != "" {
				hash, err := EventReadResultHash(all)
				if err != nil {
					t.Fatalf("EventReadResultHash: %v", err)
				}
				if hash != tc.wantHash {
					t.Fatalf("event hash = %q, want %q", hash, tc.wantHash)
				}
			}

			if tc.reconnectAfter == "" {
				return
			}
			after, err := service.ReadEvents(context.Background(), tc.sessionID, EventReconnectRequest{
				AfterEventID: tc.reconnectAfter,
			})
			if err != nil {
				t.Fatalf("ReadEvents reconnect: %v", err)
			}
			if len(after.Events) != tc.wantAfterCount {
				t.Fatalf("reconnect events = %d, want %d", len(after.Events), tc.wantAfterCount)
			}
		})
	}
}

func TestFakeService_PublishedScenarios_ReadEventsMissingCursorReturnsTypedError(t *testing.T) {
	service := newContractFakeService(t)
	row := publishedScenarioByPurpose(t, FixturePurposeEventReconnect)
	if _, err := service.StartAsync(context.Background(), startRequestForPublished(row)); err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	_, err := service.ReadEvents(context.Background(), row.SessionID, EventReconnectRequest{
		AfterEventID: "missing-event-cursor",
	})
	if !errors.Is(err, ErrReconnectCursorNotFound) {
		t.Fatalf("error = %v, want ErrReconnectCursorNotFound", err)
	}
}

func TestFakeService_PublishedScenarios_DispatchListIncludesProviderSessionRefs(t *testing.T) {
	service := newContractFakeService(t)
	if _, err := service.StartAsync(context.Background(), StartRequest{
		RequestID: "req-petri-run-001",
		Source: Source{
			Kind:      workflowsource.KindFactoryID,
			FactoryID: "customer-support-triage",
		},
	}); err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	listed, err := service.ListDispatches(context.Background(), "dur-sess-petri-run-001")
	if err != nil {
		t.Fatalf("ListDispatches: %v", err)
	}
	if len(listed.Dispatches) != 1 {
		t.Fatalf("dispatches = %#v", listed.Dispatches)
	}
	refs := listed.Dispatches[0].ProviderSessionRefs
	if len(refs) != 1 || refs[0].ID != "prov-sess-disp-petri-001" {
		t.Fatalf("providerSessionRefs = %#v", refs)
	}
}
func startPublishedScenario(t *testing.T, service *FakeService, row PublishedFixtureScenario) {
	t.Helper()
	req := startRequestForPublished(row)
	if row.Purpose == FixturePurposeSyncSuccess || row.Purpose == FixturePurposeSyncTimeout {
		if _, err := service.StartSync(context.Background(), req); err != nil {
			t.Fatalf("StartSync(%s): %v", row.Purpose, err)
		}
		return
	}
	if _, err := service.StartAsync(context.Background(), req); err != nil {
		t.Fatalf("StartAsync(%s): %v", row.Purpose, err)
	}
}

func startAwaitingApprovalSession(t *testing.T, service *FakeService) {
	t.Helper()
	_, err := service.StartAsync(context.Background(), StartRequest{
		RequestID: "req-js-awaiting-001",
		Source: Source{
			Kind:         workflowsource.KindWorkflowFile,
			WorkflowFile: ".claude/workflows/approval-gate.yaml",
		},
	})
	if err != nil {
		t.Fatalf("StartAsync awaiting approval: %v", err)
	}
}

func startFailedPartialSession(t *testing.T, service *FakeService) {
	t.Helper()
	startAsyncByRequestID(t, service, "req-js-failed-partial-001")
}

func TestFakeService_PublishedScenarios_LifecycleControlPauseResumeOutcomes(t *testing.T) {
	service := newContractFakeService(t)

	pausedRow := publishedScenarioByPurpose(t, FixturePurposeLifecycleControl)
	startPublishedScenario(t, service, pausedRow)

	pauseNoOp, err := service.Pause(context.Background(), pausedRow.SessionID, ControlRequest{})
	if err != nil {
		t.Fatalf("Pause on paused session: %v", err)
	}
	if pauseNoOp.Outcome != LifecycleControlOutcomeNoOp || pauseNoOp.Status != LifecycleStatusPaused {
		t.Fatalf("pause on paused = %#v, want NO_OP/PAUSED", pauseNoOp)
	}
	pauseNoOpHash, err := LifecycleControlResultHash(pauseNoOp)
	if err != nil {
		t.Fatalf("LifecycleControlResultHash pause no-op: %v", err)
	}
	if pauseNoOpHash != "sha256:dff882b64856e2bf56d03e29643fc65abe7129532ff38615e1033ae39873df7c" {
		t.Fatalf("pause no-op hash = %q", pauseNoOpHash)
	}

	resumed, err := service.Resume(context.Background(), pausedRow.SessionID, ControlRequest{})
	if err != nil {
		t.Fatalf("Resume paused session: %v", err)
	}
	if resumed.Outcome != LifecycleControlOutcomeAccepted || resumed.Status != LifecycleStatusRunning {
		t.Fatalf("resume = %#v, want ACCEPTED/RUNNING", resumed)
	}
	resumeHash, err := LifecycleControlResultHash(resumed)
	if err != nil {
		t.Fatalf("LifecycleControlResultHash resume: %v", err)
	}
	if resumeHash != "sha256:c12be84234b44996999436577f3967f4bccfc9b5be1d9ad179146b064d56df5a" {
		t.Fatalf("resume hash = %q", resumeHash)
	}

	runningRow := publishedScenarioByPurpose(t, FixturePurposeAsyncRunning)
	startPublishedScenario(t, service, runningRow)

	pauseAccepted, err := service.Pause(context.Background(), runningRow.SessionID, ControlRequest{})
	if err != nil {
		t.Fatalf("Pause running session: %v", err)
	}
	if pauseAccepted.Outcome != LifecycleControlOutcomeAccepted || pauseAccepted.Status != LifecycleStatusPaused {
		t.Fatalf("pause running = %#v, want ACCEPTED/PAUSED", pauseAccepted)
	}
}

func TestFakeService_PublishedScenarios_LifecycleControlCancelTerminateOutcomes(t *testing.T) {
	service := newContractFakeService(t)
	runningRow := publishedScenarioByPurpose(t, FixturePurposeAsyncRunning)
	startPublishedScenario(t, service, runningRow)

	canceled, err := service.Cancel(context.Background(), runningRow.SessionID, ControlRequest{})
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if canceled.Outcome != LifecycleControlOutcomeAccepted || canceled.Status != LifecycleStatusCanceling {
		t.Fatalf("cancel = %#v, want ACCEPTED/CANCELING", canceled)
	}

	service = newContractFakeService(t)
	startPublishedScenario(t, service, runningRow)
	terminated, err := service.Terminate(context.Background(), runningRow.SessionID, ControlRequest{})
	if err != nil {
		t.Fatalf("Terminate: %v", err)
	}
	if terminated.Outcome != LifecycleControlOutcomeAccepted || terminated.Status != LifecycleStatusTerminated {
		t.Fatalf("terminate = %#v, want ACCEPTED/TERMINATED", terminated)
	}

	terminalRow := publishedScenarioByPurpose(t, FixturePurposeSyncSuccess)
	startPublishedScenario(t, service, terminalRow)
	_, err = service.Cancel(context.Background(), terminalRow.SessionID, ControlRequest{})
	var controlErr *ControlError
	if !errors.As(err, &controlErr) || controlErr.Outcome != LifecycleControlOutcomeTerminalSession {
		t.Fatalf("cancel on terminal = %v, want TERMINAL_SESSION ControlError", err)
	}
}

func TestFakeService_PublishedScenarios_LifecycleControlApproveAwaitingApproval(t *testing.T) {
	service := newContractFakeService(t)
	startAwaitingApprovalSession(t, service)

	_, err := service.Pause(context.Background(), "dur-sess-js-awaiting-001", ControlRequest{})
	var invalidErr *ControlError
	if !errors.As(err, &invalidErr) || invalidErr.Outcome != LifecycleControlOutcomeInvalidState {
		t.Fatalf("pause on awaiting approval = %v, want INVALID_STATE ControlError", err)
	}

	approved, err := service.Approve(context.Background(), "dur-sess-js-awaiting-001", ApproveRequest{
		ControlRequest: ControlRequest{RequestID: "ctrl-approve-001"},
		ApprovedPolicy: map[string]any{"maxAgents": 2},
	})
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if approved.Outcome != LifecycleControlOutcomeAccepted || approved.Status != LifecycleStatusRunning {
		t.Fatalf("approve = %#v, want ACCEPTED/RUNNING", approved)
	}
	approveHash, err := LifecycleControlResultHash(approved)
	if err != nil {
		t.Fatalf("LifecycleControlResultHash approve: %v", err)
	}
	if approveHash != "sha256:100080e8a28d1703922847890405ca20f554419b8e6d2f3690b227c845447633" {
		t.Fatalf("approve hash = %q", approveHash)
	}
	if approved.Links.Results != "/factory-sessions/dur-sess-js-awaiting-001/results" {
		t.Fatalf("approve links = %#v", approved.Links)
	}
}

func TestFakeService_PublishedScenarios_LifecycleControlRetryDispatchPaths(t *testing.T) {
	service := newContractFakeService(t)

	terminalRow := publishedScenarioByPurpose(t, FixturePurposeSyncSuccess)
	startPublishedScenario(t, service, terminalRow)
	_, err := service.RetryDispatch(context.Background(), terminalRow.SessionID, RetryDispatchRequest{
		ControlRequest: ControlRequest{},
		DispatchID:     "disp-petri-success-001",
	})
	var terminalErr *ControlError
	if !errors.As(err, &terminalErr) || terminalErr.Outcome != LifecycleControlOutcomeTerminalSession {
		t.Fatalf("retry on terminal = %v, want TERMINAL_SESSION ControlError", err)
	}

	recoverableRow := publishedScenarioByPurpose(t, FixturePurposeFailedRecoverable)
	startPublishedScenario(t, service, recoverableRow)
	_, err = service.RetryDispatch(context.Background(), recoverableRow.SessionID, RetryDispatchRequest{
		ControlRequest: ControlRequest{},
		DispatchID:     "disp-js-interrupted-002",
	})
	if !errors.As(err, &terminalErr) || terminalErr.Outcome != LifecycleControlOutcomeTerminalSession {
		t.Fatalf("retry on interrupted = %v, want TERMINAL_SESSION ControlError", err)
	}

	startFailedPartialSession(t, service)
	retry, err := service.RetryDispatch(context.Background(), "dur-sess-js-failed-partial-001", RetryDispatchRequest{
		ControlRequest: ControlRequest{RequestID: "ctrl-retry-fail-001"},
		DispatchID:     "disp-js-fail-002",
	})
	if err != nil {
		t.Fatalf("RetryDispatch failed partial: %v", err)
	}
	if retry.Outcome != LifecycleControlOutcomeAccepted || retry.Status != LifecycleStatusRunning {
		t.Fatalf("retry = %#v, want ACCEPTED/RUNNING", retry)
	}
	if retry.RetryDispatchID != "disp-js-fail-002" {
		t.Fatalf("retryDispatchId = %q", retry.RetryDispatchID)
	}
	retryHash, err := LifecycleControlResultHash(retry)
	if err != nil {
		t.Fatalf("LifecycleControlResultHash retry: %v", err)
	}
	if retryHash != "sha256:ff4b53b67a11b90eeb9a667c68dd206cb2156265067325eb150b31877882852b" {
		t.Fatalf("retry hash = %q", retryHash)
	}

	dispatches, err := service.ListDispatches(context.Background(), "dur-sess-js-failed-partial-001")
	if err != nil {
		t.Fatalf("ListDispatches: %v", err)
	}
	for _, dispatch := range dispatches.Dispatches {
		if dispatch.ID == "disp-js-fail-002" && dispatch.Status != DispatchStatusQueued {
			t.Fatalf("retried dispatch = %#v, want QUEUED", dispatch)
		}
	}
}

func TestFakeService_PublishedScenarios_LifecycleControlAcceptedInspectionLinks(t *testing.T) {
	service := newContractFakeService(t)
	row := publishedScenarioByPurpose(t, FixturePurposeAsyncRunning)
	startPublishedScenario(t, service, row)

	paused, err := service.Pause(context.Background(), row.SessionID, ControlRequest{})
	if err != nil {
		t.Fatalf("Pause: %v", err)
	}
	want := LifecycleControlLinksForSession(row.SessionID, true)
	if paused.Links != want {
		t.Fatalf("links = %#v, want %#v", paused.Links, want)
	}
	if paused.Session == nil || paused.Session.Status != LifecycleStatusPaused {
		t.Fatalf("session projection = %#v", paused.Session)
	}
}

func TestFakeService_PublishedScenarios_LifecycleControlIdempotentReplayAndConflict(t *testing.T) {
	service := newContractFakeService(t)
	row := publishedScenarioByPurpose(t, FixturePurposeAsyncRunning)
	startPublishedScenario(t, service, row)

	first, err := service.Pause(context.Background(), row.SessionID, ControlRequest{
		RequestID: "ctrl-lifecycle-replay-001",
	})
	if err != nil {
		t.Fatalf("first Pause: %v", err)
	}
	second, err := service.Pause(context.Background(), row.SessionID, ControlRequest{
		RequestID: "ctrl-lifecycle-replay-001",
	})
	if err != nil {
		t.Fatalf("replay Pause: %v", err)
	}
	firstHash, err := LifecycleControlResultHash(first)
	if err != nil {
		t.Fatalf("first hash: %v", err)
	}
	secondHash, err := LifecycleControlResultHash(second)
	if err != nil {
		t.Fatalf("second hash: %v", err)
	}
	if firstHash != secondHash {
		t.Fatalf("replay hash drift: %q vs %q", firstHash, secondHash)
	}

	_, err = service.Resume(context.Background(), row.SessionID, ControlRequest{
		RequestID: "ctrl-lifecycle-replay-001",
	})
	var controlErr *ControlError
	if !errors.As(err, &controlErr) || controlErr.Outcome != LifecycleControlOutcomeConflict {
		t.Fatalf("conflict error = %v, want CONFLICT ControlError", err)
	}
	if controlErr.Status != LifecycleStatusPaused {
		t.Fatalf("conflict status = %q, want PAUSED", controlErr.Status)
	}
}

func TestFakeService_PublishedScenarios_LifecycleControlIsolationAcrossSessions(t *testing.T) {
	service := newContractFakeService(t)
	pausedRow := publishedScenarioByPurpose(t, FixturePurposeLifecycleControl)
	runningRow := publishedScenarioByPurpose(t, FixturePurposeAsyncRunning)
	startPublishedScenario(t, service, pausedRow)
	startPublishedScenario(t, service, runningRow)

	beforePaused, err := service.GetSession(context.Background(), pausedRow.SessionID)
	if err != nil {
		t.Fatalf("GetSession paused before: %v", err)
	}
	if beforePaused.Status != LifecycleStatusPaused {
		t.Fatalf("paused status before = %q", beforePaused.Status)
	}

	if _, err := service.Terminate(context.Background(), runningRow.SessionID, ControlRequest{}); err != nil {
		t.Fatalf("Terminate running: %v", err)
	}

	afterPaused, err := service.GetSession(context.Background(), pausedRow.SessionID)
	if err != nil {
		t.Fatalf("GetSession paused after: %v", err)
	}
	if afterPaused.Status != LifecycleStatusPaused {
		t.Fatalf("paused status after = %q, want PAUSED unchanged", afterPaused.Status)
	}
	pausedHash, err := LifecycleControlResultHash(LifecycleControlResult{
		SessionID: afterPaused.SessionID,
		Operation: LifecycleControlPause,
		Outcome:   LifecycleControlOutcomeNoOp,
		Status:    afterPaused.Status,
		Links:     LifecycleControlLinksForSession(afterPaused.SessionID, true),
	})
	if err != nil {
		t.Fatalf("LifecycleControlResultHash: %v", err)
	}
	if pausedHash != "sha256:dff882b64856e2bf56d03e29643fc65abe7129532ff38615e1033ae39873df7c" {
		t.Fatalf("isolation hash = %q", pausedHash)
	}
}

func TestFakeService_PublishedScenarios_LifecycleControlDeterministicAcrossServiceReload(t *testing.T) {
	row := publishedScenarioByPurpose(t, FixturePurposeLifecycleControl)

	runControl := func(t *testing.T) string {
		t.Helper()
		service := newContractFakeService(t)
		startPublishedScenario(t, service, row)
		resumed, err := service.Resume(context.Background(), row.SessionID, ControlRequest{})
		if err != nil {
			t.Fatalf("Resume: %v", err)
		}
		hash, err := LifecycleControlResultHash(resumed)
		if err != nil {
			t.Fatalf("LifecycleControlResultHash: %v", err)
		}
		return hash
	}

	first := runControl(t)
	second := runControl(t)
	if first != second {
		t.Fatalf("reload hash drift: %q vs %q", first, second)
	}
	if first != "sha256:c12be84234b44996999436577f3967f4bccfc9b5be1d9ad179146b064d56df5a" {
		t.Fatalf("reload resume hash = %q", first)
	}
}
func liveSessionCount(t *testing.T, service *FakeService) int {
	t.Helper()
	result, err := service.ListSessions(context.Background(), ListSessionsRequest{
		Scope: SessionListScopeLive,
	})
	if err != nil {
		t.Fatalf("ListSessions live: %v", err)
	}
	return len(result.LiveSessions)
}

func assertTypedFailureHash(t *testing.T, err error, wantHash string) TypedFailureIdentity {
	t.Helper()
	if err == nil {
		t.Fatal("error = nil, want typed failure")
	}
	identity, ok := TypedFailureIdentityFromError(err)
	if !ok {
		t.Fatalf("error = %v, want mappable typed failure identity", err)
	}
	hash, err := TypedFailureHash(identity)
	if err != nil {
		t.Fatalf("TypedFailureHash: %v", err)
	}
	if hash != wantHash {
		t.Fatalf("typed failure hash = %q, want %q (identity=%#v)", hash, wantHash, identity)
	}
	return identity
}

func TestFakeService_PublishedTypedFailures_StableErrorIdentities(t *testing.T) {
	service := newContractFakeService(t)
	runningRow := publishedScenarioByPurpose(t, FixturePurposeAsyncRunning)
	if _, err := service.StartAsync(context.Background(), startRequestForPublished(runningRow)); err != nil {
		t.Fatalf("seed running session: %v", err)
	}
	successRow := publishedScenarioByPurpose(t, FixturePurposeSyncSuccess)
	if _, err := service.StartSync(context.Background(), startRequestForPublished(successRow)); err != nil {
		t.Fatalf("seed terminal session: %v", err)
	}
	reconnectRow := publishedScenarioByPurpose(t, FixturePurposeEventReconnect)
	if _, err := service.StartAsync(context.Background(), startRequestForPublished(reconnectRow)); err != nil {
		t.Fatalf("seed reconnect session: %v", err)
	}
	artifactRow := publishedScenarioByPurpose(t, FixturePurposeArtifactInspection)
	if _, err := service.StartAsync(context.Background(), startRequestForPublished(artifactRow)); err != nil {
		t.Fatalf("seed artifact session: %v", err)
	}

	cases := []struct {
		name     string
		run      func() error
		wantHash string
		assert   func(t *testing.T, identity TypedFailureIdentity)
	}{
		{
			name: "unknown scenario",
			run: func() error {
				_, err := service.StartAsync(context.Background(), StartRequest{
					RequestID: "req-unknown-scenario-999",
					Source: Source{
						Kind:      workflowsource.KindFactoryID,
						FactoryID: "customer-support-triage",
					},
				})
				return err
			},
			wantHash: "sha256:3ca4d4c3c59dd387d3192c61359f957311940532afde4b54f6567e9324f60025",
			assert: func(t *testing.T, identity TypedFailureIdentity) {
				if identity.Kind != TypedFailureUnknownScenario || identity.Field != "requestId" {
					t.Fatalf("identity = %#v, want UNKNOWN_SCENARIO requestId", identity)
				}
			},
		},
		{
			name: "malformed start missing requestId",
			run: func() error {
				_, err := service.StartAsync(context.Background(), StartRequest{
					Source: Source{
						Kind:      workflowsource.KindFactoryID,
						FactoryID: "customer-support-triage",
					},
				})
				return err
			},
			wantHash: "sha256:5fd53056c4c9ebd6139c42b2f9ac8c41369e13dc9fa33c2bf2b945f3ddd64a66",
			assert: func(t *testing.T, identity TypedFailureIdentity) {
				if identity.Kind != TypedFailureMalformedRequest || identity.Field != "requestId" {
					t.Fatalf("identity = %#v, want MALFORMED_REQUEST requestId", identity)
				}
			},
		},
		{
			name: "malformed start missing factoryId",
			run: func() error {
				_, err := service.StartAsync(context.Background(), StartRequest{
					RequestID: "req-malformed-factory-001",
					Source:    Source{Kind: workflowsource.KindFactoryID},
				})
				return err
			},
			wantHash: "sha256:40e7ece145ff99044ce9136eba49059e6631831169655f3fe640ab937a1a7a4c",
			assert: func(t *testing.T, identity TypedFailureIdentity) {
				if identity.Kind != TypedFailureMalformedRequest || identity.Field != "source.factoryId" {
					t.Fatalf("identity = %#v, want MALFORMED_REQUEST source.factoryId", identity)
				}
			},
		},
		{
			name: "unknown session",
			run: func() error {
				_, err := service.GetSession(context.Background(), "dur-sess-missing-999")
				return err
			},
			wantHash: "sha256:4e8710020fa29e5e1a71572d34d95b31eac0585c58b0b969375722e2080df427",
			assert: func(t *testing.T, identity TypedFailureIdentity) {
				if identity.Kind != TypedFailureSessionNotFound {
					t.Fatalf("identity = %#v, want SESSION_NOT_FOUND", identity)
				}
			},
		},
		{
			name: "unknown dispatch",
			run: func() error {
				_, err := service.GetDispatch(context.Background(), successRow.SessionID, "disp-missing-999")
				return err
			},
			wantHash: "sha256:be4a698e8381fb189f8835458d6010cdcb2bd0d12340ab3f14d4d738722291c9",
			assert: func(t *testing.T, identity TypedFailureIdentity) {
				if identity.Kind != TypedFailureDispatchNotFound {
					t.Fatalf("identity = %#v, want DISPATCH_NOT_FOUND", identity)
				}
			},
		},
		{
			name: "unknown artifact",
			run: func() error {
				_, err := service.GetArtifact(context.Background(), artifactRow.SessionID, "art-missing-999")
				return err
			},
			wantHash: "sha256:60eb8812cd1420353e45889092cd8621f08975dc06fda7b82e2fb1f0e6878af6",
			assert: func(t *testing.T, identity TypedFailureIdentity) {
				if identity.Kind != TypedFailureArtifactNotFound {
					t.Fatalf("identity = %#v, want ARTIFACT_NOT_FOUND", identity)
				}
			},
		},
		{
			name: "reconnect cursor miss",
			run: func() error {
				_, err := service.ReadEvents(context.Background(), reconnectRow.SessionID, EventReconnectRequest{
					AfterEventID: "missing-event-id",
				})
				return err
			},
			wantHash: "sha256:825721e8c0269ef7775e6a498f94cf303a7e7f8eb34605443a27e7d47b89003f",
			assert: func(t *testing.T, identity TypedFailureIdentity) {
				if identity.Kind != TypedFailureReconnectCursorNotFound {
					t.Fatalf("identity = %#v, want RECONNECT_CURSOR_NOT_FOUND", identity)
				}
			},
		},
		{
			name: "execution request id conflict",
			run: func() error {
				req := startRequestForPublished(PublishedFixtureScenario{
					RequestID: FixtureScenarioIdempotentReplay,
				})
				req.RequestID = "req-idempotent-replay-001"
				if _, err := service.StartAsync(context.Background(), req); err != nil {
					return err
				}
				conflict := req
				conflict.Args = map[string]any{"task": "different"}
				_, err := service.StartAsync(context.Background(), conflict)
				return err
			},
			wantHash: "sha256:4f23c1535bd281b8c86838d72e23aa678de499ff6f7cec2c74e6e86327f1355d",
			assert: func(t *testing.T, identity TypedFailureIdentity) {
				if identity.Kind != TypedFailureExecutionRequestConflict {
					t.Fatalf("identity = %#v, want EXECUTION_REQUEST_ID_CONFLICT", identity)
				}
			},
		},
		{
			name: "lifecycle control conflict",
			run: func() error {
				if _, err := service.Pause(context.Background(), runningRow.SessionID, ControlRequest{
					RequestID: "ctrl-conflict-typed-001",
				}); err != nil {
					return err
				}
				_, err := service.Resume(context.Background(), runningRow.SessionID, ControlRequest{
					RequestID: "ctrl-conflict-typed-001",
				})
				return err
			},
			wantHash: "sha256:1d628d2ea99e52f916a3c74b240c368ac3d496eed822cfd6bd65f6a32d4e1941",
			assert: func(t *testing.T, identity TypedFailureIdentity) {
				if identity.Kind != TypedFailureLifecycleConflict ||
					identity.Outcome != LifecycleControlOutcomeConflict ||
					identity.Operation != LifecycleControlResume {
					t.Fatalf("identity = %#v, want RESUME CONFLICT", identity)
				}
				if identity.Status != LifecycleStatusPaused {
					t.Fatalf("status = %q, want PAUSED", identity.Status)
				}
			},
		},
		{
			name: "lifecycle invalid state",
			run: func() error {
				if _, err := service.StartAsync(context.Background(), StartRequest{
					RequestID: "req-js-awaiting-001",
					Source: Source{
						Kind:      workflowsource.KindFactoryID,
						FactoryID: "customer-support-triage",
					},
				}); err != nil {
					return err
				}
				_, err := service.Pause(context.Background(), "dur-sess-js-awaiting-001", ControlRequest{})
				return err
			},
			wantHash: "sha256:6c511c017a9ef0d179fe25f803795b3aad27dd469c6f186e80c69c68f7e6b987",
			assert: func(t *testing.T, identity TypedFailureIdentity) {
				if identity.Kind != TypedFailureLifecycleInvalidState ||
					identity.Operation != LifecycleControlPause {
					t.Fatalf("identity = %#v, want PAUSE INVALID_STATE", identity)
				}
			},
		},
		{
			name: "lifecycle terminal session",
			run: func() error {
				_, err := service.Cancel(context.Background(), successRow.SessionID, ControlRequest{})
				return err
			},
			wantHash: "sha256:5521c0202e46e84f30a205891e529b616710d755cefe2f95b37912e45550283d",
			assert: func(t *testing.T, identity TypedFailureIdentity) {
				if identity.Kind != TypedFailureLifecycleTerminalSession ||
					identity.Operation != LifecycleControlCancel {
					t.Fatalf("identity = %#v, want CANCEL TERMINAL_SESSION", identity)
				}
			},
		},
		{
			name: "malformed control missing dispatchId",
			run: func() error {
				_, err := service.RetryDispatch(context.Background(), runningRow.SessionID, RetryDispatchRequest{})
				return err
			},
			wantHash: "sha256:f4ea5eb8291cb4fa851df8c7eeaacffa219dea2140e9abe6d5c7239f570a60e0",
			assert: func(t *testing.T, identity TypedFailureIdentity) {
				if identity.Kind != TypedFailureMalformedRequest || identity.Field != "dispatchId" {
					t.Fatalf("identity = %#v, want MALFORMED_REQUEST dispatchId", identity)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.run()
			identity := assertTypedFailureHash(t, err, tc.wantHash)
			tc.assert(t, identity)
		})
	}
}

func TestFakeService_MalformedRequests_DoNotMutateFixtureState(t *testing.T) {
	service := newContractFakeService(t)
	before := liveSessionCount(t, service)

	_, err := service.StartAsync(context.Background(), StartRequest{
		RequestID: "",
		Source: Source{
			Kind:      workflowsource.KindFactoryID,
			FactoryID: "customer-support-triage",
		},
	})
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) || validationErr.Field != "requestId" {
		t.Fatalf("StartAsync empty requestId error = %v", err)
	}
	if after := liveSessionCount(t, service); after != before {
		t.Fatalf("live sessions after malformed start = %d, want %d", after, before)
	}

	_, err = service.StartAsync(context.Background(), StartRequest{
		RequestID: "req-unknown-scenario-typed-001",
		Source: Source{
			Kind:      workflowsource.KindFactoryID,
			FactoryID: "customer-support-triage",
		},
	})
	if !errors.As(err, &validationErr) || validationErr.Field != "requestId" {
		t.Fatalf("unknown scenario error = %v", err)
	}
	if after := liveSessionCount(t, service); after != before {
		t.Fatalf("live sessions after unknown scenario = %d, want %d", after, before)
	}

	_, err = service.RetryDispatch(context.Background(), "dur-sess-missing-typed-001", RetryDispatchRequest{})
	if !errors.As(err, &validationErr) || validationErr.Field != "dispatchId" {
		t.Fatalf("malformed retry error = %v", err)
	}
	if after := liveSessionCount(t, service); after != before {
		t.Fatalf("live sessions after malformed control = %d, want %d", after, before)
	}

	_, err = service.GetSession(context.Background(), "dur-sess-missing-typed-001")
	if !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("unknown session error = %v", err)
	}
	if after := liveSessionCount(t, service); after != before {
		t.Fatalf("live sessions after unknown session read = %d, want %d", after, before)
	}
}

func TestNormalizeListSessionsRequest_DefaultsToLiveAndRejectsUnsupportedScope(t *testing.T) {
	normalized, err := NormalizeListSessionsRequest(ListSessionsRequest{})
	if err != nil {
		t.Fatalf("NormalizeListSessionsRequest: %v", err)
	}
	if normalized.Scope != SessionListScopeLive {
		t.Fatalf("scope = %q, want live", normalized.Scope)
	}

	_, err = NormalizeListSessionsRequest(ListSessionsRequest{Scope: SessionListScope("workspace")})
	if err == nil {
		t.Fatal("error = nil, want validation error")
	}
}

func TestApplySessionListScope_LivePersistedAndAllDedup(t *testing.T) {
	startedAt := time.Date(2026, 6, 8, 10, 0, 1, 0, time.UTC)
	finishedAt := time.Date(2026, 6, 8, 10, 5, 0, 0, time.UTC)
	base := ListSessionsResult{
		LiveSessions: []LiveSessionSummary{
			{ID: "live-alpha", Project: "alpha"},
			{ID: "dur-sess-petri-success-001", Project: "alpha"},
			{ID: "live-zeta", Project: "zeta"},
		},
		DurableSessions: []DurableSessionListSummary{
			{
				SessionID:        "dur-sess-petri-success-001",
				Status:           LifecycleStatusSucceeded,
				OrchestratorKind: "PETRI",
				ResolvedSource: ResolvedSource{
					Kind:       workflowsource.KindFactoryID,
					SourceRef:  "factory/customer-support-triage",
					SourceHash: "sha256:petri-factory-001",
				},
				ResultSummary: &ResultSummary{ResultStatus: string(ResultStatusFinal)},
				Progress:      &ProgressCounts{TotalDispatches: 1, CompletedDispatches: 1},
				ArtifactCount: 1,
				Lifecycle:     &LifecycleTimestamps{StartedAt: &startedAt, FinishedAt: &finishedAt},
				Actions:       DeriveSessionActionAvailability(LifecycleStatusSucceeded),
			},
			{
				SessionID:        "dur-sess-petri-cancel-001",
				Status:           LifecycleStatusCanceled,
				OrchestratorKind: "PETRI",
				ResolvedSource: ResolvedSource{
					Kind:       workflowsource.KindFactoryID,
					SourceRef:  "factory/customer-support-triage",
					SourceHash: "sha256:petri-factory-001",
				},
				ResultSummary: &ResultSummary{ResultStatus: string(ResultStatusUnavailable)},
				Actions:       DeriveSessionActionAvailability(LifecycleStatusCanceled),
			},
			{
				SessionID:        "dur-sess-petri-run-001",
				Status:           LifecycleStatusRunning,
				OrchestratorKind: "PETRI",
				ResolvedSource: ResolvedSource{
					Kind:       workflowsource.KindFactoryID,
					SourceRef:  "factory/customer-support-triage",
					SourceHash: "sha256:petri-factory-001",
				},
				ResultSummary: &ResultSummary{ResultStatus: string(ResultStatusNotReady)},
				Progress:      &ProgressCounts{TotalDispatches: 1, InFlightDispatches: 1},
				Actions:       DeriveSessionActionAvailability(LifecycleStatusRunning),
			},
		},
	}

	liveOnly := ApplySessionListScope(base, ListSessionsRequest{Scope: SessionListScopeLive})
	if len(liveOnly.LiveSessions) != 3 || len(liveOnly.DurableSessions) != 0 {
		t.Fatalf("live scope = %#v, want live rows only", liveOnly)
	}
	if liveOnly.LiveSessions[0].ID != "dur-sess-petri-success-001" ||
		liveOnly.LiveSessions[1].ID != "live-alpha" ||
		liveOnly.LiveSessions[2].ID != "live-zeta" {
		t.Fatalf("live sort = %#v, want stable id order", liveOnly.LiveSessions)
	}

	persistedOnly := ApplySessionListScope(base, ListSessionsRequest{Scope: SessionListScopePersisted})
	if len(persistedOnly.LiveSessions) != 0 {
		t.Fatalf("persisted live rows = %#v, want none", persistedOnly.LiveSessions)
	}
	if len(persistedOnly.DurableSessions) != 2 {
		t.Fatalf("persisted durable rows = %#v, want terminal/interrupted only", persistedOnly.DurableSessions)
	}
	if persistedOnly.DurableSessions[0].SessionID != "dur-sess-petri-cancel-001" ||
		persistedOnly.DurableSessions[1].SessionID != "dur-sess-petri-success-001" {
		t.Fatalf("persisted sort = %#v, want stable session-id order", persistedOnly.DurableSessions)
	}

	allScope := ApplySessionListScope(base, ListSessionsRequest{Scope: SessionListScopeAll})
	if len(allScope.LiveSessions) != 2 {
		t.Fatalf("all live rows = %#v, want deduped live rows", allScope.LiveSessions)
	}
	for _, session := range allScope.LiveSessions {
		if session.ID == "dur-sess-petri-success-001" {
			t.Fatalf("all scope still contains deduped live id %q", session.ID)
		}
	}
	if len(allScope.DurableSessions) != 3 {
		t.Fatalf("all durable rows = %#v, want full durable list", allScope.DurableSessions)
	}
}

func TestMatchesDurableSessionListFilters_StatusOrchestratorAndRecoverability(t *testing.T) {
	summary := DurableSessionListSummary{
		SessionID:        "dur-sess-js-interrupted-001",
		Status:           LifecycleStatusInterrupted,
		OrchestratorKind: "JAVASCRIPT",
		ResolvedSource: ResolvedSource{
			Kind:      workflowsource.KindWorkflowFile,
			SourceRef: "workflow/.claude/workflows/docs-refresh.yaml",
		},
		Recoverable: true,
		StaleLease:  true,
	}

	if !MatchesDurableSessionListFilters(summary, SessionListFilters{
		Statuses:          []LifecycleStatus{LifecycleStatusInterrupted},
		OrchestratorKinds: []string{"javascript"},
		SourceKind:        workflowsource.KindWorkflowFile,
		SourceRef:         "docs-refresh",
		Recoverable:       boolPtr(true),
		StaleLease:        boolPtr(true),
	}) {
		t.Fatal("expected summary to match recoverable durable filters")
	}

	if MatchesDurableSessionListFilters(summary, SessionListFilters{
		Statuses: []LifecycleStatus{LifecycleStatusSucceeded},
	}) {
		t.Fatal("expected status filter mismatch")
	}
}

func TestDurableListSummaryFromSessionRead_ProjectsActionAvailability(t *testing.T) {
	read := SessionReadResult{
		SessionID:        "dur-sess-petri-run-001",
		Status:           LifecycleStatusRunning,
		OrchestratorKind: "PETRI",
		Progress:         &ProgressCounts{TotalDispatches: 1, InFlightDispatches: 1},
		ResultSummary:    &ResultSummary{ResultStatus: string(ResultStatusNotReady)},
		ArtifactRefs: []ArtifactRefSummary{
			{ID: "art-001"},
		},
	}
	summary := DurableListSummaryFromSessionRead(read)
	if summary.ArtifactCount != 1 {
		t.Fatalf("artifactCount = %d, want 1", summary.ArtifactCount)
	}
	if !summary.Actions.CanPause || summary.Actions.CanRetryDispatch {
		t.Fatalf("actions = %#v, want pause without retry", summary.Actions)
	}
}

func boolPtr(value bool) *bool {
	return &value
}
