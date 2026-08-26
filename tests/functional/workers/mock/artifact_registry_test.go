package mock

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	artifactRegistryWorkType    = "artifact-task"
	artifactRegistryWorker      = "artifact-worker"
	artifactRegistryWorkstation = "produce-artifacts"
	artifactRegistryWorkID      = "artifact-registry-work"
	artifactRegistryWorkName    = "artifact-registry-report"
	artifactRegistryScript      = "artifact-registry-script"
	artifactRegistrySummary     = "summary"
	artifactRegistryDetails     = "details"
)

// TestExpectedArtifactsEnforceThroughRootBuildProcess proves the customer
// process turns a successful mock-worker completion into a typed failure when
// declared files are absent, while a runner that materializes the same literal
// and templated glob reaches the normal terminal state.
func TestExpectedArtifactsEnforceThroughRootBuildProcess(t *testing.T) {
	t.Parallel()
	t.Run("under-production routes to failure", func(t *testing.T) {
		scenario := runArtifactRegistryScenario(t, false)
		assertArtifactRegistryFailure(t, scenario)
	})

	t.Run("complete production routes to success", func(t *testing.T) {
		scenario := runArtifactRegistryScenario(t, true)
		assertArtifactRegistrySuccess(t, scenario)
	})
}

type artifactRegistryScenario struct {
	listed  factoryapi.ListWorkResponse
	events  []factoryapi.FactoryEvent
	session factoryapi.FactorySession
	runner  *artifactRegistryCommandRunner
}

type artifactRegistryProjectionObservation struct {
	listed  factoryapi.ListWorkResponse
	session factoryapi.FactorySession
}

func runArtifactRegistryScenario(t *testing.T, produceArtifacts bool) artifactRegistryScenario {
	t.Helper()

	dir := scaffoldArtifactRegistryFactory(t)
	runner := &artifactRegistryCommandRunner{
		produceArtifacts: produceArtifacts,
		workName:         artifactRegistryWorkName,
	}
	mockWorkersPath := support.WriteMockWorkersConfig(t, &workers.MockWorkersConfig{
		MockWorkers: []workers.MockWorkerConfig{{
			WorkerName:      artifactRegistryWorker,
			WorkstationName: artifactRegistryWorkstation,
			RunType:         workers.MockWorkerRunTypeScript,
			ScriptConfig: &workers.MockWorkerScriptConfig{
				Command: artifactRegistryScript,
			},
		}},
	})

	api := support.NewProcessAPIServer()
	process := support.BuildProcess(t, serviceedges.Edges{
		APIServerStarter:    api.Start,
		ScriptCommandRunner: runner,
	})
	support.CleanupProcess(t, process)
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run",
		"--dir", dir,
		"--continuously",
		"--with-server",
		"--server", "http://127.0.0.1:1",
		"--quiet",
		"--no-record",
		"--with-mock-workers", mockWorkersPath,
	})
	home := t.TempDir()
	inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	inputs.Input.WorkingDirectory = dir
	command := support.StartProcessCommand(t, process, inputs.Input)
	baseURL := api.WaitForURL(t)
	stream := support.OpenFactoryEventStreamAt(t, support.DefaultSessionEventsURL(baseURL))
	if !waitForArtifactRegistryCompletion(stream) {
		command.Stop(t)
		events := support.GetFactoryEventsAt(t, baseURL)
		t.Fatalf(
			"timed out waiting for terminal Factory Event; event types=%v stdout=%q stderr=%q processErr=%v",
			artifactRegistryEventTypes(events),
			inputs.Stdout(),
			inputs.Stderr(),
			command.Err(),
		)
	}

	listed, session := waitForArtifactRegistryProjection(t, baseURL, produceArtifacts)
	scenario := artifactRegistryScenario{
		listed:  listed,
		events:  support.GetFactoryEventsAt(t, baseURL),
		session: session,
		runner:  runner,
	}
	command.Stop(t)
	return scenario
}

func waitForArtifactRegistryProjection(
	t *testing.T,
	baseURL string,
	produceArtifacts bool,
) (factoryapi.ListWorkResponse, factoryapi.FactorySession) {
	t.Helper()
	wantState := "failed"
	if produceArtifacts {
		wantState = "complete"
	}
	observation, err := support.WaitForObservation(
		15*time.Second,
		func() (artifactRegistryProjectionObservation, error) {
			return artifactRegistryProjectionObservation{
				listed:  support.ListDefaultSessionWork(t, baseURL),
				session: support.GetDefaultSession(t, baseURL),
			}, nil
		},
		func(observation artifactRegistryProjectionObservation) bool {
			item, ok := findArtifactRegistryWork(observation.listed)
			if !ok || item.State == nil || item.State.Name != wantState {
				return false
			}
			if produceArtifacts {
				return observation.session.Runtime.Progress.Categories.Terminal == 1 &&
					observation.session.Runtime.Progress.Categories.Failed == 0
			}
			return observation.session.Runtime.Progress.Categories.Terminal == 0 &&
				observation.session.Runtime.Progress.Categories.Failed == 1
		},
	)
	if err != nil {
		t.Fatalf("timed out waiting for %q Work projection: %v", wantState, err)
	}
	return observation.listed, observation.session
}

func waitForArtifactRegistryCompletion(stream *support.FactoryEventStream) bool {
	for {
		event, ok := stream.TryNextEvent(15 * time.Second)
		if !ok {
			return false
		}
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err == nil && payload.TransitionId == artifactRegistryWorkstation {
			return true
		}
	}
}

func artifactRegistryEventTypes(events []factoryapi.FactoryEvent) []factoryapi.FactoryEventType {
	types := make([]factoryapi.FactoryEventType, 0, len(events))
	for _, event := range events {
		types = append(types, event.Type)
	}
	return types
}

func assertArtifactRegistryFailure(t *testing.T, scenario artifactRegistryScenario) {
	t.Helper()

	if scenario.runner.CallCount() != 1 {
		t.Fatalf("mock script calls = %d, want one successful worker completion", scenario.runner.CallCount())
	}
	if got := scenario.session.Runtime.Progress.Categories.Failed; got != 1 {
		t.Fatalf("session failed Work count = %d, want 1", got)
	}
	if got := scenario.session.Runtime.Progress.Categories.Terminal; got != 0 {
		t.Fatalf("session terminal Work count = %d, want 0 after under-production", got)
	}
	item := artifactRegistryWork(t, scenario.listed)
	if item.State == nil || item.State.Name != "failed" || item.State.Type != factoryapi.WorkStateTypeFAILED {
		t.Fatalf("Work state = %#v, want failed state", item.State)
	}
	assertArtifactRegistryProjection(t, item, factoryapi.WorkExpectedArtifactVerificationFailed)
	response := artifactRegistryDispatchResponse(t, scenario.events)
	if response.Outcome != factoryapi.WorkOutcomeFailed {
		t.Fatalf("dispatch outcome = %s, want FAILED", response.Outcome)
	}
	if response.ArtifactVerification == nil {
		t.Fatal("dispatch response is missing expected-artifact verification summary")
	}
	if response.ArtifactVerification.Code != factoryapi.WorkFailureTypeExpectedArtifactsUnsatisfied {
		t.Fatalf("artifact verification code = %s, want EXPECTED_ARTIFACTS_UNSATISFIED", response.ArtifactVerification.Code)
	}
	if len(response.ArtifactVerification.Entries) != 2 {
		t.Fatalf("artifact verification entries = %#v, want both unmet declarations", response.ArtifactVerification.Entries)
	}
	for _, entry := range response.ArtifactVerification.Entries {
		if entry.Reason != factoryapi.ExpectedArtifactVerificationReasonMissing {
			t.Errorf("artifact verification entry %q reason = %s, want MISSING", entry.Name, entry.Reason)
		}
	}
	if response.Output == nil || !strings.Contains(*response.Output, "artifact worker completed") {
		t.Fatalf("dispatch output = %#v, want original successful worker output preserved", response.Output)
	}
	if response.Error == nil || !strings.Contains(*response.Error, artifactRegistrySummary) || !strings.Contains(*response.Error, artifactRegistryDetails) {
		t.Fatalf("dispatch error = %#v, want every unmet declaration named", response.Error)
	}
}

func assertArtifactRegistrySuccess(t *testing.T, scenario artifactRegistryScenario) {
	t.Helper()

	if scenario.runner.CallCount() != 1 {
		t.Fatalf("mock script calls = %d, want one successful worker completion", scenario.runner.CallCount())
	}
	if got := scenario.session.Runtime.Progress.Categories.Terminal; got != 1 {
		t.Fatalf("session terminal Work count = %d, want 1", got)
	}
	if got := scenario.session.Runtime.Progress.Categories.Failed; got != 0 {
		t.Fatalf("session failed Work count = %d, want 0 after complete production", got)
	}
	item := artifactRegistryWork(t, scenario.listed)
	if item.State == nil || item.State.Name != "complete" || item.State.Type != factoryapi.WorkStateTypeTERMINAL {
		t.Fatalf("Work state = %#v, want complete terminal state", item.State)
	}
	assertArtifactRegistryProjection(t, item, factoryapi.WorkExpectedArtifactVerificationSatisfied)
	response := artifactRegistryDispatchResponse(t, scenario.events)
	if response.Outcome != factoryapi.WorkOutcomeAccepted {
		t.Fatalf("dispatch outcome = %s, want ACCEPTED", response.Outcome)
	}
	if response.ArtifactVerification != nil || response.Error != nil {
		t.Fatalf("successful dispatch response = %#v, want no artifact failure", response)
	}
}

func artifactRegistryWork(t *testing.T, listed factoryapi.ListWorkResponse) factoryapi.Work {
	t.Helper()
	if item, ok := findArtifactRegistryWork(listed); ok {
		return item
	}
	t.Fatalf("public Work list is missing %q: %#v", artifactRegistryWorkID, listed.Results)
	return factoryapi.Work{}
}

func findArtifactRegistryWork(listed factoryapi.ListWorkResponse) (factoryapi.Work, bool) {
	for _, item := range listed.Results {
		if support.StringPointerValue(item.WorkId) == artifactRegistryWorkID {
			return item, true
		}
	}
	return factoryapi.Work{}, false
}

func assertArtifactRegistryProjection(
	t *testing.T,
	item factoryapi.Work,
	wantVerification factoryapi.WorkExpectedArtifactVerification,
) {
	t.Helper()
	if item.ExpectedArtifacts == nil || len(*item.ExpectedArtifacts) != 2 {
		t.Fatalf("Work expectedArtifacts = %#v, want two effective declarations", item.ExpectedArtifacts)
	}
	wantPatterns := []string{
		"reports/" + artifactRegistryWorkName + "/summary.md",
		"reports/" + artifactRegistryWorkName + "/*.json",
	}
	wantNames := []string{artifactRegistrySummary, artifactRegistryDetails}
	for index, artifact := range *item.ExpectedArtifacts {
		if artifact.Name != wantNames[index] || filepath.ToSlash(artifact.Pattern) != wantPatterns[index] {
			t.Errorf("expected artifact %d = %#v, want name=%q pattern=%q", index, artifact, wantNames[index], wantPatterns[index])
		}
		if !artifact.NonEmpty {
			t.Errorf("expected artifact %q nonEmpty = false, want true", artifact.Name)
		}
		if artifact.Verification != wantVerification {
			t.Errorf("expected artifact %q verification = %s, want %s", artifact.Name, artifact.Verification, wantVerification)
		}
	}
}

func artifactRegistryDispatchResponse(t *testing.T, events []factoryapi.FactoryEvent) factoryapi.DispatchResponseEventPayload {
	t.Helper()
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			t.Fatalf("decode artifact dispatch response: %v", err)
		}
		if payload.TransitionId == artifactRegistryWorkstation {
			return payload
		}
	}
	t.Fatalf("Factory Events are missing dispatch response for %q", artifactRegistryWorkstation)
	return factoryapi.DispatchResponseEventPayload{}
}

func scaffoldArtifactRegistryFactory(t *testing.T) string {
	t.Helper()
	dir := support.ScaffoldFactory(t, map[string]any{
		"name": "artifact-registry-functional",
		"workTypes": []map[string]any{{
			"name": artifactRegistryWorkType,
			"expectedArtifacts": []map[string]any{{
				"name":     artifactRegistrySummary,
				"pattern":  "reports/{{ (index .Inputs 0).Name }}/summary.md",
				"nonEmpty": true,
			}},
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{"name": artifactRegistryWorker}},
		"workstations": []map[string]any{{
			"name":   artifactRegistryWorkstation,
			"worker": artifactRegistryWorker,
			"inputs": []map[string]string{{"workType": artifactRegistryWorkType, "state": "init"}},
			"expectedArtifacts": []map[string]any{{
				"name":     artifactRegistryDetails,
				"pattern":  "reports/{{ (index .Inputs 0).Name }}/*.json",
				"nonEmpty": true,
			}},
			"outputs":   []map[string]string{{"workType": artifactRegistryWorkType, "state": "complete"}},
			"onFailure": []map[string]string{{"workType": artifactRegistryWorkType, "state": "failed"}},
		}},
	})
	support.WriteAgentConfig(t, dir, artifactRegistryWorker, "---\n"+
		"type: SCRIPT_WORKER\n"+
		"command: authored-artifact-command\n"+
		"---\n"+
		"Produce the declared artifacts.\n")
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkID:     artifactRegistryWorkID,
		Name:       artifactRegistryWorkName,
		WorkTypeID: artifactRegistryWorkType,
		TraceID:    "artifact-registry-trace",
		Payload:    []byte("artifact registry functional payload"),
	})
	return dir
}

type artifactRegistryCommandRunner struct {
	mu               sync.Mutex
	produceArtifacts bool
	workName         string
	requests         []platformprocess.CommandRequest
}

func (runner *artifactRegistryCommandRunner) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	if err := ctx.Err(); err != nil {
		return platformprocess.CommandResult{}, err
	}
	runner.mu.Lock()
	runner.requests = append(runner.requests, request)
	produceArtifacts := runner.produceArtifacts
	workName := runner.workName
	runner.mu.Unlock()
	if !produceArtifacts {
		return platformprocess.CommandResult{Stdout: []byte("artifact worker completed")}, nil
	}
	if strings.TrimSpace(request.WorkDir) == "" {
		return platformprocess.CommandResult{}, fmt.Errorf("artifact worker received empty workspace")
	}
	artifactDir := filepath.Join(request.WorkDir, "reports", workName)
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		return platformprocess.CommandResult{}, fmt.Errorf("create artifact workspace: %w", err)
	}
	if err := os.WriteFile(filepath.Join(artifactDir, "summary.md"), []byte("complete summary"), 0o644); err != nil {
		return platformprocess.CommandResult{}, fmt.Errorf("write summary artifact: %w", err)
	}
	if err := os.WriteFile(filepath.Join(artifactDir, "detail.json"), []byte(`{"status":"complete"}`), 0o644); err != nil {
		return platformprocess.CommandResult{}, fmt.Errorf("write detail artifact: %w", err)
	}
	return platformprocess.CommandResult{Stdout: []byte("artifact worker completed")}, nil
}

func (runner *artifactRegistryCommandRunner) CallCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return len(runner.requests)
}

var _ platformprocess.CommandRunner = (*artifactRegistryCommandRunner)(nil)
