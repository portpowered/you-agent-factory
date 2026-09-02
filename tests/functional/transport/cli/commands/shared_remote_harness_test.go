package commands_test

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/internal/functionalevidence"
)

// sharedRemoteCLI keeps one service-mode host and one reusable CLI root alive
// while each scenario selects its own explicit Factory Session. Failure cases
// still provide their own server URL when they need an unavailable or synthetic
// HTTP boundary.
type sharedRemoteCLI struct {
	process        *builtcliacceptance.Harness
	baseURL        string
	hostFactoryDir string
}

// TestCLISharedRemoteScenarios keeps the eligible remote command witnesses on
// one production service-mode host. Scenarios that select invocation-owned or
// explicit-session state overlap on one concurrent root process; cases that
// characterize or mutate the named default session remain serialized.
func TestCLISharedRemoteScenarios(t *testing.T) {
	if testing.Short() {
		t.Skip("slow shared remote CLI wiring")
	}

	hostFactoryDir := support.ScaffoldFactory(t, sharedRemoteHostFactoryConfig())
	recordingHome := t.TempDir()
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                hostFactoryDir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{
			// Keep the session-list all-scope witness independent of any
			// operator recording artifacts on the test host.
			FactorySessionResolveHomeDirectory: func() (string, error) { return recordingHome, nil },
		},
	})
	t.Cleanup(func() { server.Stop(t) })

	remote := &sharedRemoteCLI{
		process:        newLocalConcurrentProcessHarness(t),
		baseURL:        server.URL(),
		hostFactoryDir: hostFactoryDir,
	}

	serialized := []struct {
		name string
		run  func(*testing.T, *sharedRemoteCLI)
	}{
		{name: "TestCLIFactoryInitValidateAndShow", run: testCLIFactoryInitValidateAndShow},
		{name: "TestCLIFactoryReplaceCurrentChangesSessionFactory", run: testCLIFactoryReplaceCurrentChangesSessionFactory},
		{name: "TestCLISessionListUsesIsolatedRecordingHome", run: testCLISessionListUsesIsolatedRecordingHome},
	}
	for _, test := range serialized {
		t.Run(test.name, func(t *testing.T) {
			test.run(t, remote)
		})
	}

	parallel := []struct {
		name string
		run  func(*testing.T, *sharedRemoteCLI)
	}{
		{name: "TestCLISubmitBatchInlineJSON", run: testCLISubmitBatchInlineJSON},
		{name: "TestCLISubmitBatchFile", run: testCLISubmitBatchFile},
		{name: "TestCLISubmitUnavailableServer", run: testCLISubmitUnavailableServer},
		{name: "TestCLISubmitBackendErrorPreservesPublicMessage", run: testCLISubmitBackendErrorPreservesPublicMessage},
		{name: "TestCLIWorkListAndShowReflectSubmittedWork", run: testCLIWorkListAndShowReflectSubmittedWork},
		{name: "TestCLIWorkMoveChangesState", run: testCLIWorkMoveChangesState},
		{name: "TestCLIWorkShowMissingReturnsNotFound", run: testCLIWorkShowMissingReturnsNotFound},
		{name: "TestCLISessionCreateListShowDelete", run: testCLISessionCreateListShowDelete},
		{name: "TestCLISessionPauseBuffersAndResumeDispatches", run: testCLISessionPauseBuffersAndResumeDispatches},
		{name: "TestCLISessionMissingIDReturnsNotFound", run: testCLISessionMissingIDReturnsNotFound},
		{name: "TestCLIWorkApprovalListAndShowExposePendingApprovalAndSafeEmptyErrors", run: testCLIWorkApprovalListAndShowExposePendingApprovalAndSafeEmptyErrors},
		{name: "TestCLIExplicitSessionIsolation", run: testCLIExplicitSessionIsolation},
	}
	for _, test := range parallel {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			test.run(t, remote)
		})
	}
	functionalevidence.Covers(
		t,
		"cli/you.submit.batch",
		"cli/you.work.approval.list",
		"cli/you.work.approval.show",
		"cli/you.work.move",
	)
}

func (r *sharedRemoteCLI) run(
	ctx context.Context,
	workingDir string,
	sessionID string,
	args ...string,
) ([]byte, error) {
	return r.runAt(ctx, workingDir, r.baseURL, sessionID, args...)
}

func (r *sharedRemoteCLI) runAt(
	ctx context.Context,
	workingDir string,
	serverURL string,
	sessionID string,
	args ...string,
) ([]byte, error) {
	if strings.TrimSpace(sessionID) != "" {
		args = append(args, "--session", sessionID)
	}
	return runYouCLI(ctx, r.process, workingDir, serverURL, args...)
}

func (r *sharedRemoteCLI) openSession(t *testing.T, factoryDir string) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	output, err := r.run(ctx, factoryDir, "", "--json", "session", "create", "--dir", factoryDir)
	if err != nil {
		t.Fatalf("you session create for %q: %v\noutput:\n%s", factoryDir, err, output)
	}
	var opened struct {
		Session *struct {
			ID         string `json:"id"`
			FolderPath string `json:"folderPath"`
		} `json:"session"`
	}
	if err := json.Unmarshal(bytesTrimSpace(output), &opened); err != nil {
		t.Fatalf("decode session create JSON for %q: %v\noutput:\n%s", factoryDir, err, output)
	}
	if opened.Session == nil || strings.TrimSpace(opened.Session.ID) == "" {
		t.Fatalf("session create response for %q missing session id: %#v", factoryDir, opened)
	}
	if opened.Session.FolderPath != factoryDir {
		t.Fatalf("session folder path for %q = %q, want %q", factoryDir, opened.Session.FolderPath, factoryDir)
	}
	sessionID := opened.Session.ID
	t.Cleanup(func() {
		r.closeSession(t, factoryDir, sessionID)
	})
	return sessionID
}

func (r *sharedRemoteCLI) closeSession(t *testing.T, workingDir, sessionID string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	terminateOut, err := r.run(ctx, workingDir, "", "--remote", "--json", "session", "terminate", sessionID)
	if err != nil {
		t.Errorf("you session terminate %q during cleanup: %v\noutput:\n%s", sessionID, err, terminateOut)
	}
	deleteOut, err := r.run(ctx, workingDir, "", "--json", "session", "delete", sessionID)
	if err != nil {
		t.Errorf("you session delete %q during cleanup: %v\noutput:\n%s", sessionID, err, deleteOut)
	}
}

func (r *sharedRemoteCLI) assertHealthy(t *testing.T, workingDir string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	output, err := r.run(ctx, workingDir, "", "--json", "factory", "show")
	if err != nil {
		t.Fatalf("shared remote server did not recover: %v\noutput:\n%s", err, output)
	}
	var factory factoryapi.Factory
	if err := json.Unmarshal(bytesTrimSpace(output), &factory); err != nil {
		t.Fatalf("decode shared remote health response: %v\noutput:\n%s", err, output)
	}
	if strings.TrimSpace(factory.Name) == "" {
		t.Fatalf("shared remote health response omitted Factory name: %#v", factory)
	}
}

func testCLIExplicitSessionIsolation(t *testing.T, r *sharedRemoteCLI) {
	factoryADir := support.ScaffoldFactory(t, sharedRemoteIsolationFactoryConfig(
		"cli-remote-isolation-a",
		"remote-session-a-work",
	))
	factoryBDir := support.ScaffoldFactory(t, sharedRemoteIsolationFactoryConfig(
		"cli-remote-isolation-b",
		"remote-session-b-work",
	))
	sessionA := r.openSession(t, factoryADir)
	sessionB := r.openSession(t, factoryBDir)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	workA := submitSharedIsolationWork(t, r, ctx, factoryADir, sessionA, "cli-remote-isolation-request-a", "remote-session-a-work")
	workB := submitSharedIsolationWork(t, r, ctx, factoryBDir, sessionB, "cli-remote-isolation-request-b", "remote-session-b-work")
	if workA == workB {
		t.Fatalf("isolated sessions received the same work id %q", workA)
	}

	listA := runWorkListCLIJSON(t, ctx, r.process, factoryADir, r.baseURL, sessionA, "")
	assertIsolatedWorkList(t, listA, sessionA, workA, "remote-session-a-work", workB, "remote-session-b-work")
	listB := runWorkListCLIJSON(t, ctx, r.process, factoryBDir, r.baseURL, sessionB, "")
	assertIsolatedWorkList(t, listB, sessionB, workB, "remote-session-b-work", workA, "remote-session-a-work")
	waitForWorkStateViaCLI(t, ctx, r.process, factoryADir, r.baseURL, sessionA, workA, "complete", 30*time.Second)
	waitForWorkStateViaCLI(t, ctx, r.process, factoryBDir, r.baseURL, sessionB, workB, "complete", 30*time.Second)

	factoryA := replaceFactoryViaCLIJSON(t, ctx, r, factoryADir, sessionA)
	assertFactoryDirectory(t, "session A", factoryA, factoryADir)
	factoryB := replaceFactoryViaCLIJSON(t, ctx, r, factoryBDir, sessionB)
	assertFactoryDirectory(t, "session B", factoryB, factoryBDir)

	shownA := runSessionShowCLIJSON(t, ctx, r.process, factoryADir, r.baseURL, sessionA)
	shownB := runSessionShowCLIJSON(t, ctx, r.process, factoryBDir, r.baseURL, sessionB)
	if shownA.Id != sessionA || shownB.Id != sessionB || shownA.Id == shownB.Id {
		t.Fatalf("session show identities are not disjoint: A=%q B=%q", shownA.Id, shownB.Id)
	}
	if !samePath(shownA.FactoryDir, factoryADir) || !samePath(shownB.FactoryDir, factoryBDir) || samePath(shownA.FactoryDir, shownB.FactoryDir) {
		t.Fatalf(
			"session show factory directories are not disjoint: A=%q B=%q, want A=%q B=%q",
			shownA.FactoryDir,
			shownB.FactoryDir,
			factoryADir,
			factoryBDir,
		)
	}
}

func assertFactoryDirectory(t *testing.T, label string, factory factoryapi.Factory, wantDir string) {
	t.Helper()
	if factory.FactoryDirectory == nil || !samePath(*factory.FactoryDirectory, wantDir) {
		gotDir := "<missing>"
		if factory.FactoryDirectory != nil {
			gotDir = *factory.FactoryDirectory
		}
		t.Fatalf("%s factory directory = %q, want %q", label, gotDir, wantDir)
	}
}

func samePath(left, right string) bool {
	leftAbs, leftErr := filepath.Abs(filepath.Clean(left))
	rightAbs, rightErr := filepath.Abs(filepath.Clean(right))
	if leftErr != nil || rightErr != nil {
		return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
	}
	return strings.EqualFold(leftAbs, rightAbs)
}

func replaceFactoryViaCLIJSON(
	t *testing.T,
	ctx context.Context,
	r *sharedRemoteCLI,
	workingDir, sessionID string,
) factoryapi.Factory {
	t.Helper()

	output, err := r.run(ctx, workingDir, sessionID, "--json", "factory", "replace-current")
	if err != nil {
		t.Fatalf("you factory replace-current for session %q: %v\noutput:\n%s", sessionID, err, output)
	}
	var current factoryapi.Factory
	if err := json.Unmarshal(bytesTrimSpace(output), &current); err != nil {
		t.Fatalf("decode factory replace-current JSON for session %q: %v\noutput:\n%s", sessionID, err, output)
	}
	return current
}

func submitSharedIsolationWork(
	t *testing.T,
	r *sharedRemoteCLI,
	ctx context.Context,
	workingDir, sessionID, requestID, workName string,
) string {
	t.Helper()
	batch := fmt.Sprintf(
		`{"requestId":%q,"type":"FACTORY_REQUEST_BATCH","works":[{"name":%q,"workTypeName":"task","payload":{"title":"isolated remote session"}}]}`,
		requestID,
		workName,
	)
	output, err := r.run(ctx, workingDir, sessionID, "--json", "submit", "batch", batch)
	if err != nil {
		t.Fatalf("submit isolated work for session %q: %v\noutput:\n%s", sessionID, err, output)
	}
	var submitted workWiringBatchSubmitJSON
	if err := json.Unmarshal(bytesTrimSpace(output), &submitted); err != nil {
		t.Fatalf("decode isolated submit JSON for session %q: %v\noutput:\n%s", sessionID, err, output)
	}
	if submitted.WorkCount != 1 || len(submitted.Works) != 1 || strings.TrimSpace(submitted.Works[0].WorkID) == "" {
		t.Fatalf("isolated submit response missing work identity for session %q: %#v", sessionID, submitted)
	}
	return submitted.Works[0].WorkID
}

func assertIsolatedWorkList(
	t *testing.T,
	listed factoryapi.ListWorkResponse,
	sessionID, ownWorkID, ownWorkName, foreignWorkID, foreignWorkName string,
) {
	t.Helper()
	foundOwn := false
	for _, work := range listed.Results {
		workID := ""
		if work.WorkId != nil {
			workID = strings.TrimSpace(*work.WorkId)
		}
		if workID == foreignWorkID || work.Name == foreignWorkName {
			t.Fatalf("session %q work list leaked foreign work %q/%q: %#v", sessionID, foreignWorkID, foreignWorkName, listed.Results)
		}
		if workID == ownWorkID {
			foundOwn = true
			if work.Name != ownWorkName {
				t.Fatalf("session %q work %q name = %q, want %q", sessionID, ownWorkID, work.Name, ownWorkName)
			}
		}
	}
	if !foundOwn {
		t.Fatalf("session %q work list did not contain own work %q: %#v", sessionID, ownWorkID, listed.Results)
	}
}

func sharedRemoteHostFactoryConfig() map[string]any {
	config := factoryWiringFactoryConfig()
	config["id"] = factoryWiringName
	return config
}

func sessionHistoryOnlyFactoryConfig() map[string]any {
	config := sharedRemoteHostFactoryConfig()
	delete(config, "workers")
	delete(config, "workstations")
	return config
}

func sharedRemoteIsolationFactoryConfig(name, workName string) map[string]any {
	return map[string]any{
		"name": name,
		"id":   name,
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{"name": "mock-worker"}},
		"workstations": []map[string]any{{
			"name":      "process-task-" + workName,
			"worker":    "mock-worker",
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
		}},
	}
}
