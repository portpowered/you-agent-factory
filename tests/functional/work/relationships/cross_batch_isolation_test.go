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

// testCrossBatchDependsOnRejectsCrossSessionTargetAtomically proves that a
// target visible in one Factory Session cannot satisfy admission in another,
// and that the rejected batch admits none of its Work items.
func testCrossBatchDependsOnRejectsCrossSessionTargetAtomically(t *testing.T, host *sharedRelationshipHost) {
	t.Helper()

	factoryDir := scaffoldCrossBatchFactory(t)
	baseURL := host.URL()
	sourceSession, sourceClose := openSharedRelationshipSession(t, baseURL, factoryDir)
	otherSession, otherClose := openSharedRelationshipSession(t, baseURL, factoryDir)
	baseline := listCrossBatchSessionWork(t, baseURL, otherSession.Id)

	executeCrossBatchSubmitForSessionOnServer(t, host.server, sourceSession.Id, crossBatchPrerequisiteBatchJSON())
	support.WaitForSessionTerminalStatus(t, baseURL, sourceSession.Id, 15*time.Second)
	assertCrossBatchWorkStateForSession(t, baseURL, sourceSession.Id, crossBatchPrerequisiteID, "complete", "completed source-session target")

	stdout, stderr, err := executeCrossBatchSubmitForSessionExpectingError(
		t,
		host.server,
		baseURL,
		otherSession.Id,
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

	listed := listCrossBatchSessionWork(t, baseURL, otherSession.Id)
	if len(listed.Results) != len(baseline.Results) {
		t.Fatalf("cross-session rejection changed Work count from %d to %d: %#v", len(baseline.Results), len(listed.Results), listed.Results)
	}
	for _, workID := range []string{crossBatchCrossSessionDependentID, crossBatchCrossSessionSiblingID} {
		if crossBatchSessionWorkListed(listed, workID) {
			t.Fatalf("rejected cross-session batch admitted Work %q: %#v", workID, listed.Results)
		}
	}
	sourceClose()
	otherClose()
	runSharedHostReuseProbe(t, baseURL)
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
	server *support.FunctionalAPIServer,
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
	err := server.Execute(t, inputs.Input)
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
