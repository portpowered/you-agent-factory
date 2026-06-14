package fixtures_test

import (
	"context"
	"testing"

	fse "github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution/fixtures"
)

func newFakeExecutionServiceFromContractFixtures(t *testing.T) fse.Service {
	t.Helper()
	scenarios, err := fse.LoadFakeScenariosFromContractFixtures(contractFixtureCatalogPath(t))
	if err != nil {
		t.Fatalf("LoadFakeScenariosFromContractFixtures: %v", err)
	}
	service, err := fse.NewExecutionService(
		fse.ExecutionProviderFake,
		fse.ServiceConfig{
			FakeOptions: []fse.FakeServiceOption{
				fse.WithFakeScenarios(scenarios...),
			},
		},
	)
	if err != nil {
		t.Fatalf("NewExecutionService(fake): %v", err)
	}
	return service
}

func TestNewExecutionService_FakeProvider_PublishedScenarios_StillDeterministic(t *testing.T) {
	service := newFakeExecutionServiceFromContractFixtures(t)

	successRow := publishedScenarioByPurpose(t, fixtures.FixturePurposeSyncSuccess)
	terminal, err := service.StartSync(context.Background(), startRequestForPublished(successRow))
	if err != nil {
		t.Fatalf("StartSync success: %v", err)
	}
	if terminal.SyncOutcome != fse.SyncOutcomeCompleted {
		t.Fatalf("syncOutcome = %q, want COMPLETED", terminal.SyncOutcome)
	}
	if terminal.SessionID != successRow.SessionID {
		t.Fatalf("sessionId = %q, want %q", terminal.SessionID, successRow.SessionID)
	}
	terminalHash, err := fixtures.SyncStartResultHash(terminal)
	if err != nil {
		t.Fatalf("SyncStartResultHash: %v", err)
	}
	if terminalHash != "sha256:89b3a278be3192017c6fcd9fbd4ca57154fb84ab6154ce961e4a597ba5fa6c05" {
		t.Fatalf("sync success hash = %q, want sha256:89b3a278be3192017c6fcd9fbd4ca57154fb84ab6154ce961e4a597ba5fa6c05", terminalHash)
	}

	runningRow := publishedScenarioByPurpose(t, fixtures.FixturePurposeAsyncRunning)
	if _, err := service.StartAsync(context.Background(), startRequestForPublished(runningRow)); err != nil {
		t.Fatalf("StartAsync running: %v", err)
	}
	session, err := service.GetSession(context.Background(), runningRow.SessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if session.Status != fse.LifecycleStatusRunning {
		t.Fatalf("status = %q, want RUNNING", session.Status)
	}

	result, err := service.GetResult(context.Background(), runningRow.SessionID, fse.ResultRequest{
		Mode: fse.ResultModePartial,
	})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if result.ResultStatus != fse.ResultStatusPartial {
		t.Fatalf("resultStatus = %q, want PARTIAL", result.ResultStatus)
	}
	resultHash, err := fixtures.ProjectedResultReadHash(result)
	if err != nil {
		t.Fatalf("ProjectedResultReadHash: %v", err)
	}
	if resultHash != "sha256:f4830cd3534f5de6491b04dd4c05b2b1e01cf73844877ad922ea7d6547ae07f6" {
		t.Fatalf("result hash = %q, want sha256:f4830cd3534f5de6491b04dd4c05b2b1e01cf73844877ad922ea7d6547ae07f6", resultHash)
	}
}

func TestJavaScriptRuntimeService_UsesExistingFactorySessionReadSurfaces(t *testing.T) {
	service := newJavaScriptRuntimeService(t)
	req := inlineWorkflowStartRequest(
		"req-runtime-session-surfaces-001",
		simpleFinalWorkflowSource,
		map[string]any{"subject": "workflows", "count": 1, "prefix": "you"},
		nil,
	)

	started, err := service.StartSync(context.Background(), req)
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}
	if started.SessionID == "" || started.SyncOutcome != fse.SyncOutcomeCompleted {
		t.Fatalf("sync start = %#v, want completed FactorySession execution response", started)
	}

	session, err := service.GetSession(context.Background(), started.SessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if session.SessionID != started.SessionID {
		t.Fatalf("sessionId = %q, want %q", session.SessionID, started.SessionID)
	}

	result, err := service.GetResult(context.Background(), started.SessionID, fse.ResultRequest{
		Mode: fse.ResultModeFinal,
	})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if result.SessionID != started.SessionID || result.ResultStatus != fse.ResultStatusFinal {
		t.Fatalf("result = %#v, want final FactorySession result read", result)
	}
}
