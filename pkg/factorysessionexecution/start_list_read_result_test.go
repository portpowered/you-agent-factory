package factorysessionexecution

import (
	"context"
	"errors"
	"testing"

	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

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
