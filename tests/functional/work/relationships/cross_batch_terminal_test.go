package relationships

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	crossBatchMixedCompleteName  = "mixed-complete"
	crossBatchMixedCompleteID    = "work-mixed-complete"
	crossBatchMixedFailedName    = "mixed-failed"
	crossBatchMixedFailedID      = "work-mixed-failed"
	crossBatchMixedDependentName = "mixed-dependent"
	crossBatchMixedDependentID   = "work-mixed-dependent"
)

// TestCrossBatchDependsOnCompletedTargetReleasesAtAdmission proves that a
// target which already reached its required state releases a later batch
// immediately. It also checks that the admitted relation keeps the target ID.
func TestCrossBatchDependsOnCompletedTargetReleasesAtAdmission(t *testing.T) {
	requireCrossBatchGit(t)
	run := newCrossBatchFunctionalRun(t)

	executeCrossBatchSubmit(t, run.submitProcess, run.baseURL, crossBatchPrerequisiteBatchJSON())
	run.runner.WaitForFinishDispatch(t, 15*time.Second)
	run.runner.Release()
	support.WaitForSessionTerminalStatus(t, run.baseURL, run.session.Id, 15*time.Second)
	if got := run.runner.CallCount(); got != 2 {
		t.Fatalf("provider calls before completed-target admission = %d, want two prerequisite dispatches", got)
	}

	executeCrossBatchSubmit(t, run.submitProcess, run.baseURL, crossBatchDependentBatchByIDJSON())
	support.WaitForSessionTerminalStatus(t, run.baseURL, run.session.Id, 15*time.Second)

	listed := support.ListDefaultSessionWork(t, run.baseURL)
	assertCrossBatchTerminalState(t, listed, crossBatchPrerequisiteID, "complete")
	assertCrossBatchTerminalState(t, listed, crossBatchDependentID, "complete")
	assertCrossBatchCanonicalDependency(t, listed, crossBatchDependentID, crossBatchPrerequisiteID)
	if got := run.runner.CallCount(); got != 4 {
		t.Fatalf("provider calls after completed-target admission = %d, want four total dispatches", got)
	}

	prerequisiteSequence, dependentSequence := crossBatchDispatchOrdering(
		t,
		support.GetFactoryEventsAt(t, run.baseURL),
	)
	if dependentSequence <= prerequisiteSequence {
		t.Fatalf("dependent dispatch sequence = %d, want after completed target sequence %d", dependentSequence, prerequisiteSequence)
	}
}

// TestCrossBatchDependsOnFailedTargetCascadesAtAdmission proves that a later
// batch submitted against a failed target is admitted, cascades to failed, and
// never receives a worker dispatch.
func TestCrossBatchDependsOnFailedTargetCascadesAtAdmission(t *testing.T) {
	requireCrossBatchGit(t)
	run := newTerminalCrossBatchFunctionalRun(t, crossBatchPrerequisiteName)

	executeCrossBatchSubmit(t, run.submitProcess, run.baseURL, crossBatchPrerequisiteBatchJSON())
	support.WaitForSessionTerminalStatus(t, run.baseURL, run.session.Id, 15*time.Second)
	assertCrossBatchTerminalState(t, support.ListDefaultSessionWork(t, run.baseURL), crossBatchPrerequisiteID, "failed")
	if got := run.runner.CallCount(); got != 2 {
		t.Fatalf("provider calls before failed-target admission = %d, want two prerequisite dispatches", got)
	}

	executeCrossBatchSubmit(t, run.submitProcess, run.baseURL, crossBatchDependentBatchJSON())
	support.WaitForSessionTerminalStatus(t, run.baseURL, run.session.Id, 15*time.Second)

	listed := support.ListDefaultSessionWork(t, run.baseURL)
	assertCrossBatchTerminalState(t, listed, crossBatchPrerequisiteID, "failed")
	assertCrossBatchTerminalState(t, listed, crossBatchDependentID, "failed")
	assertCrossBatchCanonicalDependency(t, listed, crossBatchDependentID, crossBatchPrerequisiteID)
	assertCrossBatchNoDispatchForWork(t, support.GetFactoryEventsAt(t, run.baseURL), crossBatchDependentID)
	if got := run.runner.CallCount(); got != 2 {
		t.Fatalf("provider calls after failed-target admission = %d, want no dependent dispatch", got)
	}
}

// TestCrossBatchDependsOnMixedTerminalFanInCascades proves that a later batch
// with one complete and one failed target does not dispatch its dependent.
func TestCrossBatchDependsOnMixedTerminalFanInCascades(t *testing.T) {
	requireCrossBatchGit(t)
	run := newTerminalCrossBatchFunctionalRun(t, crossBatchMixedFailedName)

	executeCrossBatchSubmit(t, run.submitProcess, run.baseURL, crossBatchMixedTargetsBatchJSON())
	support.WaitForSessionTerminalStatus(t, run.baseURL, run.session.Id, 15*time.Second)
	listed := support.ListDefaultSessionWork(t, run.baseURL)
	assertCrossBatchTerminalState(t, listed, crossBatchMixedCompleteID, "complete")
	assertCrossBatchTerminalState(t, listed, crossBatchMixedFailedID, "failed")
	if got := run.runner.CallCount(); got != 4 {
		t.Fatalf("provider calls before mixed fan-in admission = %d, want four target dispatches", got)
	}

	executeCrossBatchSubmit(t, run.submitProcess, run.baseURL, crossBatchMixedDependentBatchJSON())
	support.WaitForSessionTerminalStatus(t, run.baseURL, run.session.Id, 15*time.Second)

	listed = support.ListDefaultSessionWork(t, run.baseURL)
	assertCrossBatchTerminalState(t, listed, crossBatchMixedDependentID, "failed")
	assertCrossBatchCanonicalDependency(t, listed, crossBatchMixedDependentID, crossBatchMixedCompleteID)
	assertCrossBatchCanonicalDependency(t, listed, crossBatchMixedDependentID, crossBatchMixedFailedID)
	assertCrossBatchNoDispatchForWork(t, support.GetFactoryEventsAt(t, run.baseURL), crossBatchMixedDependentID)
	if got := run.runner.CallCount(); got != 4 {
		t.Fatalf("provider calls after mixed fan-in admission = %d, want no dependent dispatch", got)
	}
}

func crossBatchDependentBatchByIDJSON() string {
	return fmt.Sprintf(`{
		"requestId": "cross-batch-dependent-by-id",
		"type": "FACTORY_REQUEST_BATCH",
		"works": [{
			"name": %q,
			"workId": %q,
			"workTypeName": "task",
			"payload": {"title": "Cross-batch dependent by ID"}
		}],
		"relations": [{
			"type": "DEPENDS_ON",
			"sourceWorkName": %q,
			"targetWorkId": %q
		}]
	}`, crossBatchDependentName, crossBatchDependentID, crossBatchDependentName, crossBatchPrerequisiteID)
}

func crossBatchMixedTargetsBatchJSON() string {
	return fmt.Sprintf(`{
		"requestId": "cross-batch-mixed-targets",
		"type": "FACTORY_REQUEST_BATCH",
		"works": [
			{
				"name": %q,
				"workId": %q,
				"workTypeName": "task",
				"payload": {"title": "Mixed fan-in complete target"}
			},
			{
				"name": %q,
				"workId": %q,
				"workTypeName": "task",
				"payload": {"title": "Mixed fan-in failed target"}
			}
		]
	}`, crossBatchMixedCompleteName, crossBatchMixedCompleteID, crossBatchMixedFailedName, crossBatchMixedFailedID)
}

func crossBatchMixedDependentBatchJSON() string {
	return fmt.Sprintf(`{
		"requestId": "cross-batch-mixed-dependent",
		"type": "FACTORY_REQUEST_BATCH",
		"works": [{
			"name": %q,
			"workId": %q,
			"workTypeName": "task",
			"payload": {"title": "Mixed fan-in dependent"}
		}],
		"relations": [
			{
				"type": "DEPENDS_ON",
				"sourceWorkName": %q,
				"targetWorkName": %q
			},
			{
				"type": "DEPENDS_ON",
				"sourceWorkName": %q,
				"targetWorkName": %q
			}
		]
	}`, crossBatchMixedDependentName, crossBatchMixedDependentID,
		crossBatchMixedDependentName, crossBatchMixedCompleteName,
		crossBatchMixedDependentName, crossBatchMixedFailedName)
}

func assertCrossBatchTerminalState(t *testing.T, listed factoryapi.ListWorkResponse, workID, state string) {
	t.Helper()
	if !support.HasWorkAtCustomerState(listed, workID, support.WorkCustomerLocation("task", state)) {
		for _, item := range listed.Results {
			if support.StringPointerValue(item.WorkId) == workID && item.State != nil {
				t.Fatalf("Work %q did not reach %q; public state=%q: %#v", workID, state, item.State.Name, listed)
			}
		}
		t.Fatalf("Work %q did not reach %q: %#v", workID, state, listed)
	}
}

func assertCrossBatchCanonicalDependency(t *testing.T, listed factoryapi.ListWorkResponse, sourceID, targetID string) {
	t.Helper()
	for _, item := range listed.Results {
		if support.StringPointerValue(item.WorkId) != sourceID {
			continue
		}
		if item.Relations == nil {
			t.Fatalf("Work %q has no relations, want dependency on %q", sourceID, targetID)
		}
		for _, relation := range *item.Relations {
			if relation.Type == factoryapi.RelationTypeDependsOn && support.StringPointerValue(relation.TargetWorkId) == targetID {
				return
			}
		}
		t.Fatalf("Work %q relations = %#v, want DEPENDS_ON target %q", sourceID, *item.Relations, targetID)
	}
	t.Fatalf("Work %q is missing from public listing: %#v", sourceID, listed)
}

func assertCrossBatchNoDispatchForWork(t *testing.T, events []factoryapi.FactoryEvent, workID string) {
	t.Helper()
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchRequest {
			continue
		}
		payload, err := event.Payload.AsDispatchRequestEventPayload()
		if err != nil {
			t.Fatalf("decode dispatch event for Work %q: %v", workID, err)
		}
		if dispatchRequestIncludesWork(payload, workID) {
			t.Fatalf("Work %q received dispatch at sequence %d", workID, event.Context.Sequence)
		}
	}
}

type terminalCrossBatchFunctionalRun struct {
	baseURL       string
	session       factoryapi.FactorySession
	submitProcess support.Process
	runner        *terminalCrossBatchCommandRunner
}

func newTerminalCrossBatchFunctionalRun(t *testing.T, failWorkName string) terminalCrossBatchFunctionalRun {
	t.Helper()
	factoryDir := scaffoldCrossBatchGitFactory(t)
	runner := newTerminalCrossBatchCommandRunner(failWorkName)
	api := support.NewProcessAPIServer()
	daemonProcess := support.BuildProcess(t, serviceedges.Edges{
		APIServerStarter:      api.Start,
		ProviderCommandRunner: runner,
	})
	support.CleanupProcess(t, daemonProcess)

	runInputs := support.FakeInputs(t.Context(), []string{
		"you", "run", "--dir", factoryDir, "--continuously", "--with-server",
		"--server", "http://127.0.0.1:1", "--quiet", "--no-record",
	})
	homeDir := t.TempDir()
	runInputs.Input.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	runInputs.Input.WorkingDirectory = factoryDir
	support.StartProcessCommand(t, daemonProcess, runInputs.Input)

	baseURL := api.WaitForURL(t)
	submitProcess := support.BuildProcess(t, serviceedges.Edges{})
	support.CleanupProcess(t, submitProcess)
	return terminalCrossBatchFunctionalRun{
		baseURL:       baseURL,
		session:       support.GetDefaultSession(t, baseURL),
		submitProcess: submitProcess,
		runner:        runner,
	}
}

type terminalCrossBatchCommandRunner struct {
	failWorkName string

	mu    sync.Mutex
	calls int
}

func newTerminalCrossBatchCommandRunner(failWorkName string) *terminalCrossBatchCommandRunner {
	return &terminalCrossBatchCommandRunner{
		failWorkName: failWorkName,
	}
}

func (r *terminalCrossBatchCommandRunner) Run(_ context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	r.mu.Lock()
	r.calls++
	r.mu.Unlock()
	requestPrompt := string(request.Stdin)
	shouldFail := strings.Contains(requestPrompt, "Complete cross-batch Work "+r.failWorkName)
	if shouldFail {
		return platformprocess.CommandResult{}, errors.New("controlled cross-batch prerequisite failure")
	}
	return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout("COMPLETE")}, nil
}

func (r *terminalCrossBatchCommandRunner) CallCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

var _ platformprocess.CommandRunner = (*terminalCrossBatchCommandRunner)(nil)
