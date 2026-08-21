package relationships

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	crossBatchCrossSessionDependentName = "cross-session-dependent"
	crossBatchCrossSessionDependentID   = "work-cross-session-dependent"
	crossBatchCrossSessionSiblingName   = "cross-session-sibling"
	crossBatchCrossSessionSiblingID     = "work-cross-session-sibling"
)

// TestCrossBatchDependsOnRejectsCrossSessionTargetAtomically proves that a
// target visible in one Factory Session cannot satisfy admission in another,
// and that the rejected batch admits none of its Work items.
func TestCrossBatchDependsOnRejectsCrossSessionTargetAtomically(t *testing.T) {
	requireCrossBatchGit(t)
	run := newCrossBatchFunctionalRun(t)

	opened := support.OpenFactorySessionAt(t, run.baseURL, run.session.FolderPath)
	if opened.Session == nil {
		t.Fatalf("cross-session open response missing session: %#v", opened)
	}
	otherSessionID := opened.Session.Id
	baseline := listCrossBatchSessionWork(t, run.baseURL, otherSessionID)

	executeCrossBatchSubmit(t, run.submitProcess, run.baseURL, crossBatchPrerequisiteBatchJSON())
	run.runner.WaitForFinishDispatch(t, 15*time.Second)
	run.runner.Release()
	assertCrossBatchWorkState(t, run.baseURL, crossBatchPrerequisiteID, "complete", "completed source-session target")

	stdout, stderr, err := executeCrossBatchSubmitForSessionExpectingError(
		t,
		run.submitProcess,
		run.baseURL,
		otherSessionID,
		crossBatchCrossSessionBatchJSON(),
	)
	if err == nil {
		t.Fatalf("cross-session batch unexpectedly succeeded\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
	}
	diagnostic := strings.Join([]string{err.Error(), stdout, stderr}, "\n")
	for _, marker := range []string{
		"batch submission failed (400)",
		"code=BAD_REQUEST",
		"family=BAD_REQUEST",
		"targetWorkId",
		crossBatchPrerequisiteID,
		"does not identify a Work on this Factory Session board",
	} {
		if !strings.Contains(diagnostic, marker) {
			t.Fatalf("cross-session rejection diagnostic missing %q:\n%s", marker, diagnostic)
		}
	}

	listed := listCrossBatchSessionWork(t, run.baseURL, otherSessionID)
	if len(listed.Results) != len(baseline.Results) {
		t.Fatalf("cross-session rejection changed Work count from %d to %d: %#v", len(baseline.Results), len(listed.Results), listed.Results)
	}
	for _, workID := range []string{crossBatchCrossSessionDependentID, crossBatchCrossSessionSiblingID} {
		if crossBatchSessionWorkListed(listed, workID) {
			t.Fatalf("rejected cross-session batch admitted Work %q: %#v", workID, listed.Results)
		}
	}
	if got := run.runner.CallCount(); got != 2 {
		t.Fatalf("provider calls after cross-session rejection = %d, want only the source session's two dispatches", got)
	}
}

func crossBatchCrossSessionBatchJSON() string {
	return fmt.Sprintf(`{
		"requestId": "cross-batch-cross-session",
		"type": "FACTORY_REQUEST_BATCH",
		"works": [
			{
				"name": %q,
				"workId": %q,
				"workTypeName": "task",
				"payload": {"title": "Cross-session dependent"}
			},
			{
				"name": %q,
				"workId": %q,
				"workTypeName": "task",
				"payload": {"title": "Cross-session sibling"}
			}
		],
		"relations": [{
			"type": "DEPENDS_ON",
			"sourceWorkName": %q,
			"targetWorkId": %q
		}]
	}`, crossBatchCrossSessionDependentName, crossBatchCrossSessionDependentID,
		crossBatchCrossSessionSiblingName, crossBatchCrossSessionSiblingID,
		crossBatchCrossSessionDependentName, crossBatchPrerequisiteID)
}

func executeCrossBatchSubmitForSessionExpectingError(
	t *testing.T,
	process support.Process,
	baseURL string,
	sessionID string,
	batchJSON string,
) (string, string, error) {
	t.Helper()
	homeDir := t.TempDir()
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--server", baseURL, "--session", sessionID, "--json", "submit", "batch", batchJSON,
	})
	inputs.Input.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	inputs.Input.WorkingDirectory = homeDir
	stdinIsTTY := true
	inputs.Input.StdinIsTTY = &stdinIsTTY
	inputs.Input.Stdin = strings.NewReader("")
	err := process.Execute(inputs.Input)
	return inputs.Stdout(), inputs.Stderr(), err
}

func listCrossBatchSessionWork(t *testing.T, baseURL, sessionID string) factoryapi.ListWorkResponse {
	t.Helper()
	endpoint := fmt.Sprintf(
		"%s/factory-sessions/%s/work",
		strings.TrimSuffix(baseURL, "/"),
		url.PathEscape(sessionID),
	)
	return support.GetJSON[factoryapi.ListWorkResponse](t, endpoint)
}

func crossBatchSessionWorkListed(listed factoryapi.ListWorkResponse, workID string) bool {
	for _, item := range listed.Results {
		if support.StringPointerValue(item.WorkId) == workID {
			return true
		}
	}
	return false
}
