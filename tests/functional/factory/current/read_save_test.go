package current

import (
	"net/http"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestSharedCurrentFactoryAPI keeps one root-built server alive while the
// compatible read/save scenarios use unique explicit Factory Sessions. The
// named Factory files are created once before the server starts and are never
// rewritten by the child scenarios.
func TestSharedCurrentFactoryAPI(t *testing.T) {
	t.Parallel()
	fixture := startSharedCurrentFactoryAPI(t)

	t.Run("GetAndSave", func(t *testing.T) {
		t.Parallel()
		testSharedCurrentFactoryGetAndSave(t, fixture)
	})
	t.Run("SaveValidates", func(t *testing.T) {
		t.Parallel()
		testSharedCurrentFactorySaveValidates(t, fixture)
	})
	t.Run("FactoriesRemainSessionScoped", func(t *testing.T) {
		t.Parallel()
		testSharedCurrentFactoriesRemainSessionScoped(t, fixture)
	})
	t.Run("PromptTemplateContractAndValidation", func(t *testing.T) {
		t.Parallel()
		testSharedPromptTemplateContractAndValidation(t, fixture)
	})
	t.Run("InvalidPromptTemplate", func(t *testing.T) {
		t.Parallel()
		testSharedInvalidPromptTemplate(t, fixture)
	})
	t.Run("TemplateValidationDoesNotMutate", func(t *testing.T) {
		t.Parallel()
		testSharedTemplateValidationDoesNotMutate(t, fixture)
	})

}

// testSharedCurrentFactoryGetAndSave proves that one explicit Factory Session
// can read its Current Factory, save a valid updated definition through the
// public session API, and read back the saved customer-visible topology within
// the same session.
func testSharedCurrentFactoryGetAndSave(t *testing.T, fixture *sharedCurrentFactoryAPI) {
	fixture.requireServerRunning(t)
	session := fixture.openSession(t, "alpha-valid")
	current := getCurrentFactoryForSession(t, session.serverURL, session.id)
	if current.Name != factoryapi.FactoryName("alpha-valid") {
		t.Fatalf("current factory name = %q, want alpha-valid", current.Name)
	}
	assertFactoryWorkType(t, current, "alpha-valid-task", "initial current factory")

	advanced := advancedFactoryVersion(t, current.Version)
	saved := saveCurrentFactoryForSession(
		t,
		session.serverURL,
		session.id,
		functionalNamedFactoryBody("alpha-valid", "story", advanced),
	)
	assertFactoryVersionAtLeast(t, saved.Version, advanced, "saved")
	assertFactoryWorkType(t, saved, "story", "save response")

	reloaded := getCurrentFactoryForSession(t, session.serverURL, session.id)
	if reloaded.Name != factoryapi.FactoryName("alpha-valid") {
		t.Fatalf("reloaded current factory name = %q, want alpha-valid", reloaded.Name)
	}
	assertFactoryWorkType(t, reloaded, "story", "subsequent get within session")
	assertFactoryVersionAtLeast(t, reloaded.Version, advanced, "reloaded")
	fixture.requireServerRunning(t)
}

// testSharedCurrentFactorySaveValidates proves that an invalid Current
// Factory save is rejected through the public session API before persistence, returns
// a structured validation error, and leaves the prior Current Factory unchanged on
// subsequent readback within the same session.
func testSharedCurrentFactorySaveValidates(t *testing.T, fixture *sharedCurrentFactoryAPI) {
	fixture.requireServerRunning(t)
	session := fixture.openSession(t, "alpha-invalid")
	current := getCurrentFactoryForSession(t, session.serverURL, session.id)
	if current.Version == nil {
		t.Fatal("current factory version = nil, want version metadata for save")
	}
	assertFactoryWorkType(t, current, "alpha-invalid-task", "initial current factory")

	advanced := advancedFactoryVersion(t, current.Version)
	body := `{
		"name":"alpha-invalid",
		"version":{"physical":"` + advanced.Physical.UTC().Format(time.RFC3339Nano) + `","logical":"` + strconv.FormatInt(advanced.Logical.Int64(), 10) + `"},
		"workTypes":[{"name":"story","states":[{"name":"queued","type":"INITIAL"}]}],
		"workers":[{"name":"worker-a","type":"MODEL_WORKER","modelProvider":"CLAUDE","executorProvider":"SCRIPT_WRAP","model":"claude-sonnet-4-20250514"}],
		"workstations":[{"name":"process","behavior":"STANDARD","type":"MODEL_WORKSTATION","worker":"worker-a","inputs":[{"workType":"story","state":"queued"}],"outputs":[{"workType":"story","state":"missing-state"}]}]
	}`

	resp := saveCurrentFactoryForSessionExpectStatus(t, session.serverURL, session.id, body, http.StatusBadRequest)
	var errResp factoryapi.ErrorResponse
	decodeJSONResponse(t, resp, &errResp, "decode invalid current factory save response")
	if errResp.Code != factoryapi.ErrorResponseCodeINVALIDFACTORY {
		t.Fatalf("error code = %q, want INVALID_FACTORY", errResp.Code)
	}
	if errResp.Family != factoryapi.ErrorFamilyBadRequest {
		t.Fatalf("error family = %q, want BAD_REQUEST", errResp.Family)
	}
	if errResp.Targets == nil || len(*errResp.Targets) == 0 {
		t.Fatalf("error targets = %#v, want topology validation targets", errResp.Targets)
	}
	if !hasValidationTargetCode(*errResp.Targets, factoryValidationCodeDanglingPlaceReference) {
		t.Fatalf("error targets = %#v, want dangling place reference target", errResp.Targets)
	}

	reloaded := getCurrentFactoryForSession(t, session.serverURL, session.id)
	if !reflect.DeepEqual(reloaded, current) {
		t.Fatalf("reloaded current factory = %#v, want unchanged %#v", reloaded, current)
	}
	assertFactoryWorkType(t, reloaded, "alpha-invalid-task", "current factory after rejected save")
	fixture.requireServerRunning(t)
}

// testSharedCurrentFactoriesRemainSessionScoped proves that Current Factory saves stay
// isolated across Factory Sessions so a valid save in one session updates that
// session's readback while another session's Current Factory remains unchanged.
func testSharedCurrentFactoriesRemainSessionScoped(t *testing.T, fixture *sharedCurrentFactoryAPI) {
	fixture.requireServerRunning(t)
	alphaSession := fixture.openSession(t, "alpha-isolation")
	betaSession := fixture.openSession(t, "beta-isolation")
	if alphaSession.id == betaSession.id {
		t.Fatalf("alpha and beta sessions share identity %q", alphaSession.id)
	}

	alphaBefore := getCurrentFactoryForSession(t, alphaSession.serverURL, alphaSession.id)
	sessionCurrent := getCurrentFactoryForSession(t, betaSession.serverURL, betaSession.id)
	if alphaBefore.Name != factoryapi.FactoryName("alpha-isolation") {
		t.Fatalf("alpha session current factory name = %q, want alpha-isolation", alphaBefore.Name)
	}
	assertFactoryWorkType(t, alphaBefore, "alpha-isolation-task", "alpha session current factory before save")
	if sessionCurrent.Name != factoryapi.FactoryName("beta-isolation") {
		t.Fatalf("session current factory name = %q, want beta-isolation", sessionCurrent.Name)
	}
	assertFactoryWorkType(t, sessionCurrent, "beta-isolation-task", "beta session current factory before save")

	advanced := advancedFactoryVersion(t, sessionCurrent.Version)
	saved := saveCurrentFactoryForSession(
		t,
		betaSession.serverURL,
		betaSession.id,
		functionalNamedFactoryBody("beta-isolation", "story", advanced),
	)
	assertFactoryWorkType(t, saved, "story", "beta session save response")
	assertFactoryVersionAtLeast(t, saved.Version, advanced, "beta saved")

	reloaded := getCurrentFactoryForSession(t, betaSession.serverURL, betaSession.id)
	assertFactoryWorkType(t, reloaded, "story", "beta session current factory after save")
	assertFactoryVersionAtLeast(t, reloaded.Version, advanced, "beta reloaded")

	alphaAfter := getCurrentFactoryForSession(t, alphaSession.serverURL, alphaSession.id)
	if !reflect.DeepEqual(alphaAfter, alphaBefore) {
		t.Fatalf("alpha session current factory after beta save = %#v, want unchanged %#v", alphaAfter, alphaBefore)
	}
	defaultCurrent := getCurrentFactory(t, fixture.server.URL())
	if defaultCurrent.Name != factoryapi.FactoryName("alpha") {
		t.Fatalf("default current factory name = %q, want alpha", defaultCurrent.Name)
	}
	assertFactoryWorkType(t, defaultCurrent, "alpha-task", "default session current factory after beta session save")

	unknown := getCurrentFactoryForSessionExpectStatus(t, fixture.server.URL(), "unknown-shared-current-factory-session", http.StatusNotFound)
	assertNotFoundFactorySessionError(t, unknown, "unknown session")

	closedSession := fixture.openSession(t, "alpha")
	closedSession.close(t)
	closed := getCurrentFactoryForSessionExpectStatus(t, fixture.server.URL(), closedSession.id, http.StatusNotFound)
	assertNotFoundFactorySessionError(t, closed, "closed session")
	fixture.requireServerRunning(t)
}

func assertNotFoundFactorySessionError(
	t *testing.T,
	response factoryapi.ErrorResponse,
	contextLabel string,
) {
	t.Helper()
	if response.Code != factoryapi.ErrorResponseCodeNOTFOUND {
		t.Fatalf("%s code = %q, want NOT_FOUND", contextLabel, response.Code)
	}
	if response.Family != factoryapi.ErrorFamilyNotFound {
		t.Fatalf("%s family = %q, want NOT_FOUND", contextLabel, response.Family)
	}
	if response.Message == "" {
		t.Fatalf("%s message is empty, want safe session diagnostic", contextLabel)
	}
}

// TestProcessFactoryConfigVersionChangesObservableRouting proves that distinct
// on-disk factory configs route Work through their authored workstation graphs.
func TestProcessFactoryConfigVersionChangesObservableRouting(t *testing.T) {
	t.Parallel()
	v1Dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "workflow_v1_dir"))
	testutil.WriteSeedFile(t, v1Dir, "task", []byte("v1 work item"))

	providerV1 := testutil.NewMockWorkerMapProvider(map[string][]workerexecution.InferenceResponse{
		"processor": {{Content: "Processed. COMPLETE"}},
		"finalizer": {{Content: "Finalized. COMPLETE"}},
	})
	_, listedV1 := support.RunFactoryToCompletionWithEdgesAndWork(
		t,
		v1Dir,
		serviceedges.Edges{ProviderOverride: providerV1},
		10*time.Second,
	)
	assertCurrentFactoryWorkCustomerStates(t, listedV1, map[string]int{
		support.WorkCustomerLocation("task", "complete"):   1,
		support.WorkCustomerLocation("task", "init"):       0,
		support.WorkCustomerLocation("task", "processing"): 0,
	})
	if providerV1.CallCount("processor") != 1 {
		t.Errorf("v1 processor call count = %d, want 1", providerV1.CallCount("processor"))
	}
	if providerV1.CallCount("finalizer") != 1 {
		t.Errorf("v1 finalizer call count = %d, want 1", providerV1.CallCount("finalizer"))
	}

	v2Dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "workflow_v2_dir"))
	testutil.WriteSeedFile(t, v2Dir, "task", []byte("v2 work item"))

	providerV2 := testutil.NewMockWorkerMapProvider(map[string][]workerexecution.InferenceResponse{
		"processor": {{Content: "Processed. COMPLETE"}},
		"reviewer":  {{Content: "Reviewed. COMPLETE"}},
		"finalizer": {{Content: "Finalized. COMPLETE"}},
	})
	_, listedV2 := support.RunFactoryToCompletionWithEdgesAndWork(
		t,
		v2Dir,
		serviceedges.Edges{ProviderOverride: providerV2},
		10*time.Second,
	)
	assertCurrentFactoryWorkCustomerStates(t, listedV2, map[string]int{
		support.WorkCustomerLocation("task", "complete"):   1,
		support.WorkCustomerLocation("task", "init"):       0,
		support.WorkCustomerLocation("task", "processing"): 0,
		support.WorkCustomerLocation("task", "in-review"):  0,
	})
	if providerV2.CallCount("processor") != 1 {
		t.Errorf("v2 processor call count = %d, want 1", providerV2.CallCount("processor"))
	}
	if providerV2.CallCount("reviewer") != 1 {
		t.Errorf("v2 reviewer call count = %d, want 1", providerV2.CallCount("reviewer"))
	}
	if providerV2.CallCount("finalizer") != 1 {
		t.Errorf("v2 finalizer call count = %d, want 1", providerV2.CallCount("finalizer"))
	}
}

// TestProcessRejectionLoopCompletesAfterRetry proves that a factory config with
// rejection routing completes after one reject-then-accept cycle.
func TestProcessRejectionLoopCompletesAfterRetry(t *testing.T) {
	t.Parallel()
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "workflow_v2_rejection_dir"))
	testutil.WriteSeedFile(t, dir, "doc", []byte("needs-revision draft"))
	provider := testutil.NewMockWorkerMapProvider(map[string][]workerexecution.InferenceResponse{
		"drafter":  {{Content: "draft COMPLETE"}, {Content: "revised draft COMPLETE"}},
		"approver": {{Content: "needs revision"}, {Content: "approved COMPLETE"}},
	})
	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(
		t,
		dir,
		serviceedges.Edges{ProviderOverride: provider},
		10*time.Second,
	)
	assertCurrentFactoryWorkCustomerStates(t, listed, map[string]int{
		support.WorkCustomerLocation("doc", "complete"):   1,
		support.WorkCustomerLocation("doc", "init"):       0,
		support.WorkCustomerLocation("doc", "processing"): 0,
	})
	if got := provider.CallCount("drafter"); got != 2 {
		t.Errorf("drafter call count = %d, want 2", got)
	}
	if got := provider.CallCount("approver"); got != 2 {
		t.Errorf("approver call count = %d, want 2", got)
	}
}

// TestProcessIndependentFactoryRootsRemainIsolated proves that separate factory
// roots run to completion without cross-contaminating each other's Work state.
func TestProcessIndependentFactoryRootsRemainIsolated(t *testing.T) {
	t.Parallel()
	dirA := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "simple_pipeline"))
	testutil.WriteSeedFile(t, dirA, "task", []byte("item for A"))

	providerA := testutil.NewMockWorkerMapProvider(map[string][]workerexecution.InferenceResponse{
		"processor": {{Content: "Done. COMPLETE"}},
	})
	_, listedA := support.RunFactoryToCompletionWithEdgesAndWork(
		t,
		dirA,
		serviceedges.Edges{ProviderOverride: providerA},
		10*time.Second,
	)
	assertCurrentFactoryWorkCustomerStates(t, listedA, map[string]int{
		support.WorkCustomerLocation("task", "complete"): 1,
	})

	dirB := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "workflow_v1_dir"))
	testutil.WriteSeedFile(t, dirB, "task", []byte("task for B"))

	providerB := testutil.NewMockWorkerMapProvider(map[string][]workerexecution.InferenceResponse{
		"processor": {{Content: "Processed. COMPLETE"}},
		"finalizer": {{Content: "Finalized. COMPLETE"}},
	})
	_, listedB := support.RunFactoryToCompletionWithEdgesAndWork(
		t,
		dirB,
		serviceedges.Edges{ProviderOverride: providerB},
		10*time.Second,
	)
	assertCurrentFactoryWorkCustomerStates(t, listedB, map[string]int{
		support.WorkCustomerLocation("task", "complete"): 1,
	})
	assertCurrentFactoryWorkCustomerStates(t, listedA, map[string]int{
		support.WorkCustomerLocation("task", "complete"): 1,
		support.WorkCustomerLocation("task", "init"):     0,
	})
}
