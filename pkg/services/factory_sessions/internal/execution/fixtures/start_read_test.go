package fixtures_test

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	fse "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/fixtures"
)

func TestFakeService_PublishedScenarios_SyncStartTerminalAndTimeout(t *testing.T) {
	service := newContractFakeService(t)

	successRow := publishedScenarioByPurpose(t, fixtures.FixturePurposeSyncSuccess)
	terminal, err := service.StartSync(context.Background(), startRequestForPublished(successRow))
	if err != nil {
		t.Fatalf("fse.StartSync success: %v", err)
	}
	if terminal.SyncOutcome != fse.SyncOutcomeCompleted {
		t.Fatalf("syncOutcome = %q, want COMPLETED", terminal.SyncOutcome)
	}
	if terminal.Status != string(fse.LifecycleStatusSucceeded) {
		t.Fatalf("status = %q, want SUCCEEDED", terminal.Status)
	}
	terminalHash, err := fixtures.SyncStartResultHash(terminal)
	if err != nil {
		t.Fatalf("fixtures.SyncStartResultHash: %v", err)
	}
	if terminalHash != "sha256:89b3a278be3192017c6fcd9fbd4ca57154fb84ab6154ce961e4a597ba5fa6c05" {
		t.Fatalf("sync success hash = %q, want sha256:89b3a278be3192017c6fcd9fbd4ca57154fb84ab6154ce961e4a597ba5fa6c05", terminalHash)
	}

	timeoutRow := publishedScenarioByPurpose(t, fixtures.FixturePurposeSyncTimeout)
	timedOut, err := service.StartSync(context.Background(), startRequestForPublished(timeoutRow))
	if err != nil {
		t.Fatalf("fse.StartSync timeout: %v", err)
	}
	if timedOut.SyncOutcome != fse.SyncOutcomeTimedOut || !timedOut.TimedOut {
		t.Fatalf("timeout response = %#v", timedOut)
	}
	if timedOut.SessionID != timeoutRow.SessionID {
		t.Fatalf("sessionId = %q, want %q", timedOut.SessionID, timeoutRow.SessionID)
	}
	timeoutHash, err := fixtures.SyncStartResultHash(timedOut)
	if err != nil {
		t.Fatalf("fixtures.SyncStartResultHash: %v", err)
	}
	if timeoutHash != "sha256:58f35deb8326d37ab4048ebf7f1d7d6f6a994e04b82de300a61f52f7f72e5378" {
		t.Fatalf("sync timeout hash = %q, want sha256:58f35deb8326d37ab4048ebf7f1d7d6f6a994e04b82de300a61f52f7f72e5378", timeoutHash)
	}
}

func TestFakeService_PublishedScenarios_GetSessionReadModels(t *testing.T) {
	service := newContractFakeService(t)
	cases := []struct {
		purpose fixtures.FixtureScenarioPurpose
		sync    bool
	}{
		{fixtures.FixturePurposeAsyncRunning, false},
		{fixtures.FixturePurposeSyncSuccess, true},
		{fixtures.FixturePurposeSyncTimeout, true},
		{fixtures.FixturePurposeFailedRecoverable, false},
	}
	for _, tc := range cases {
		row := publishedScenarioByPurpose(t, tc.purpose)
		t.Run(string(tc.purpose), func(t *testing.T) {
			req := startRequestForPublished(row)
			if tc.sync {
				if _, err := service.StartSync(context.Background(), req); err != nil {
					t.Fatalf("fse.StartSync: %v", err)
				}
			} else if _, err := service.StartAsync(context.Background(), req); err != nil {
				t.Fatalf("fse.StartAsync: %v", err)
			}

			read, err := service.GetSession(context.Background(), row.SessionID)
			if err != nil {
				t.Fatalf("fse.GetSession: %v", err)
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
			result, err := service.GetResult(context.Background(), row.SessionID, fse.ResultRequest{Mode: fse.ResultModePartial})
			if tc.purpose == fixtures.FixturePurposeSyncSuccess || tc.purpose == fixtures.FixturePurposeSyncTimeout {
				result, err = service.GetResult(context.Background(), row.SessionID, fse.ResultRequest{Mode: fse.ResultModeFinal})
			}
			if err != nil {
				t.Fatalf("fse.GetResult: %v", err)
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
		purpose          fixtures.FixtureScenarioPurpose
		sync             bool
		resultRequest    fse.ResultRequest
		wantHash         string
		wantAvailability string
	}{
		{
			purpose:       fixtures.FixturePurposeAsyncRunning,
			resultRequest: fse.ResultRequest{Mode: fse.ResultModePartial},
			wantHash:      "sha256:f4830cd3534f5de6491b04dd4c05b2b1e01cf73844877ad922ea7d6547ae07f6",
		},
		{
			purpose:       fixtures.FixturePurposeSyncSuccess,
			sync:          true,
			resultRequest: fse.ResultRequest{Mode: fse.ResultModeFinal, IncludeArtifacts: false},
			wantHash:      "sha256:977772c884f0ec53b9292ca8fa0374fec1673fec8d0d481e358b3dd4ae65fb95",
		},
		{
			purpose:          fixtures.FixturePurposeSyncTimeout,
			sync:             true,
			resultRequest:    fse.ResultRequest{Mode: fse.ResultModeFinal},
			wantHash:         "sha256:ab30784fe4f173cd457d0fe83d90425eeee0212ce2942869886a31316d70b4ba",
			wantAvailability: "SYNC_WAIT_TIMED_OUT",
		},
		{
			purpose:       fixtures.FixturePurposeFailedRecoverable,
			resultRequest: fse.ResultRequest{Mode: fse.ResultModePartial},
			wantHash:      "sha256:266b2572ecbf4d6e87f9143ac2852866069365beefe329e886a6827ff0de3746",
		},
	}
	for _, tc := range cases {
		row := publishedScenarioByPurpose(t, tc.purpose)
		t.Run(string(tc.purpose), func(t *testing.T) {
			req := startRequestForPublished(row)
			if tc.sync {
				if _, err := service.StartSync(context.Background(), req); err != nil {
					t.Fatalf("fse.StartSync: %v", err)
				}
			} else if _, err := service.StartAsync(context.Background(), req); err != nil {
				t.Fatalf("fse.StartAsync: %v", err)
			}

			result, err := service.GetResult(context.Background(), row.SessionID, tc.resultRequest)
			if err != nil {
				t.Fatalf("fse.GetResult: %v", err)
			}
			if result.ResultStatus != row.ResultStatus {
				t.Fatalf("resultStatus = %q, want %q", result.ResultStatus, row.ResultStatus)
			}
			if tc.wantAvailability != "" {
				if result.Availability == nil || result.Availability.Reason != tc.wantAvailability {
					t.Fatalf("availability = %#v, want reason %q", result.Availability, tc.wantAvailability)
				}
			}
			hash, err := fixtures.ProjectedResultReadHash(result)
			if err != nil {
				t.Fatalf("fixtures.ProjectedResultReadHash: %v", err)
			}
			if hash != tc.wantHash {
				t.Fatalf("result hash = %q, want %q", hash, tc.wantHash)
			}
		})
	}
}

func TestFakeService_PublishedScenarios_StartIdempotentReplay(t *testing.T) {
	service := newContractFakeService(t)
	req := startRequestForPublished(fixtures.PublishedFixtureScenario{
		RequestID: fixtures.FixtureScenarioIdempotentReplay,
	})
	req.RequestID = "req-idempotent-replay-001"

	first, err := service.StartAsync(context.Background(), req)
	if err != nil {
		t.Fatalf("first fse.StartAsync: %v", err)
	}
	second, err := service.StartAsync(context.Background(), req)
	if err != nil {
		t.Fatalf("replay fse.StartAsync: %v", err)
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
	if !errors.Is(err, fse.ErrExecutionRequestIDConflict) {
		t.Fatalf("error = %v, want fse.ErrExecutionRequestIDConflict", err)
	}
}

func TestProjectedResultReadHash_IsStableAcrossEquivalentReads(t *testing.T) {
	service := newContractFakeService(t)
	row := publishedScenarioByPurpose(t, fixtures.FixturePurposeSyncSuccess)
	if _, err := service.StartSync(context.Background(), startRequestForPublished(row)); err != nil {
		t.Fatalf("fse.StartSync: %v", err)
	}
	first, err := service.GetResult(context.Background(), row.SessionID, fse.ResultRequest{Mode: fse.ResultModeFinal})
	if err != nil {
		t.Fatalf("first fse.GetResult: %v", err)
	}
	second, err := service.GetResult(context.Background(), row.SessionID, fse.ResultRequest{Mode: fse.ResultModeFinal})
	if err != nil {
		t.Fatalf("second fse.GetResult: %v", err)
	}
	firstHash, err := fixtures.ProjectedResultReadHash(first)
	if err != nil {
		t.Fatalf("first hash: %v", err)
	}
	secondHash, err := fixtures.ProjectedResultReadHash(second)
	if err != nil {
		t.Fatalf("second hash: %v", err)
	}
	if firstHash != secondHash {
		t.Fatalf("hash drift: %q vs %q", firstHash, secondHash)
	}
}
func TestMatchesDurableSessionListFilters_StatusOrchestratorAndRecoverability(t *testing.T) {
	summary := fse.DurableSessionListSummary{
		SessionID:        "dur-sess-js-interrupted-001",
		Status:           fse.LifecycleStatusInterrupted,
		OrchestratorKind: "JAVASCRIPT",
		ResolvedSource: fse.ResolvedSource{
			Kind:      factory.WorkflowSourceKindWorkflowFile,
			SourceRef: "workflow/.claude/workflows/docs-refresh.yaml",
		},
		Recoverable: true,
		StaleLease:  true,
	}

	if !fse.MatchesDurableSessionListFilters(summary, fse.SessionListFilters{
		Statuses:          []fse.LifecycleStatus{fse.LifecycleStatusInterrupted},
		OrchestratorKinds: []string{"javascript"},
		SourceKind:        factory.WorkflowSourceKindWorkflowFile,
		SourceRef:         "docs-refresh",
		Recoverable:       boolPtr(true),
		StaleLease:        boolPtr(true),
	}) {
		t.Fatal("expected summary to match recoverable durable filters")
	}

	if fse.MatchesDurableSessionListFilters(summary, fse.SessionListFilters{
		Statuses: []fse.LifecycleStatus{fse.LifecycleStatusSucceeded},
	}) {
		t.Fatal("expected status filter mismatch")
	}
}

func TestDurableListSummaryFromSessionRead_ProjectsActionAvailability(t *testing.T) {
	read := fse.SessionReadResult{
		SessionID:        "dur-sess-petri-run-001",
		Status:           fse.LifecycleStatusRunning,
		OrchestratorKind: "PETRI",
		Progress:         &fse.ProgressCounts{TotalDispatches: 1, InFlightDispatches: 1},
		ResultSummary:    &fse.ResultSummary{ResultStatus: string(fse.ResultStatusNotReady)},
		ArtifactRefs: []fse.ArtifactRefSummary{
			{ID: "art-001"},
		},
	}
	summary := fse.DurableListSummaryFromSessionRead(read)
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
