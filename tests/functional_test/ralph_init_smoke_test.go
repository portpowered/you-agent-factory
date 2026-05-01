package functional_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	initcmd "github.com/portpowered/infinite-you/pkg/cli/init"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/pkg/workers"
)

const ralphInitSmokeRequest = `Create a minimal release-planning loop for a document processing service.
Generate a human-readable PRD, a matching Ralph JSON plan, and an execution loop
that completes one story per iteration until the work is done.
Keep the plan product-neutral unless the customer request names a specific product.
`

type ralphInitSmokeMode string

const (
	ralphInitSmokeModeComplete ralphInitSmokeMode = "complete"
	ralphInitSmokeModeExhaust  ralphInitSmokeMode = "exhaust"
)

type ralphInitSmokeRunner struct {
	mu                   sync.Mutex
	rootDir              string
	mode                 ralphInitSmokeMode
	plannerCalls         int
	executorCalls        int
	executorStoryWorkID  string
	executorSawArtifacts bool
	executorPRDHistory   []ralphInitSmokePRD
	executorResponses    []string
	workstationSequence  []string
	internalErrors       []string
}

type ralphInitSmokePRD struct {
	Project          string                    `json:"project"`
	BranchName       string                    `json:"branchName"`
	Description      string                    `json:"description"`
	RequestedChanges []string                  `json:"requestedChanges"`
	CustomerIntent   string                    `json:"customerIntent"`
	UserStories      []ralphInitSmokeUserStory `json:"userStories"`
}

type ralphInitSmokeUserStory struct {
	ID                 string   `json:"id"`
	Title              string   `json:"title"`
	Priority           int      `json:"priority"`
	Passes             bool     `json:"passes"`
	Description        string   `json:"description"`
	AcceptanceCriteria []string `json:"acceptanceCriteria"`
	Notes              []string `json:"notes"`
}

func TestIntegrationSmoke_RalphInitScaffoldCompletesFromGeneratedLoop(t *testing.T) {
	dir := initRalphSmokeScaffold(t)
	setWorkingDirectory(t, dir)
	writeRalphSmokeRequest(t, dir, "release-planning-loop.md")

	runner := newRalphInitSmokeRunner(dir, ralphInitSmokeModeComplete)
	harness := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProviderCommandRunner(runner),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	harness.RunUntilComplete(t, 15*time.Second)
	runner.Assert(t)

	harness.Assert().
		PlaceTokenCount("request:planned", 1).
		PlaceTokenCount("story:complete", 1).
		HasNoTokenInPlace("request:failed").
		HasNoTokenInPlace("story:init").
		HasNoTokenInPlace("story:failed")

	assertRalphInitSmokeSequence(t, runner.WorkstationSequence(), []string{
		"plan-request",
		"execute-story",
		"execute-story",
	})
	if got := runner.PlannerCalls(); got != 1 {
		t.Fatalf("planner calls = %d, want 1", got)
	}
	if got := runner.ExecutorCalls(); got != 2 {
		t.Fatalf("executor calls = %d, want 2", got)
	}
	history := runner.ExecutorPRDHistory()
	if len(history) != 2 {
		t.Fatalf("executor PRD history length = %d, want 2", len(history))
	}
	if got := highestPriorityIncompleteStoryID(history[0]); got != "US-001" {
		t.Fatalf("first execute call highest-priority incomplete story = %q, want %q", got, "US-001")
	}
	if got := highestPriorityIncompleteStoryID(history[1]); got != "US-002" {
		t.Fatalf("second execute call highest-priority incomplete story = %q, want %q", got, "US-002")
	}
	if history[0].UserStories[0].Passes || history[0].UserStories[1].Passes {
		t.Fatalf("first execute call should start with both stories incomplete: %#v", history[0].UserStories)
	}
	if !history[1].UserStories[0].Passes || history[1].UserStories[1].Passes {
		t.Fatalf("second execute call should see only the first story completed: %#v", history[1].UserStories)
	}
	responses := runner.ExecutorResponses()
	if len(responses) != 2 {
		t.Fatalf("executor response count = %d, want 2", len(responses))
	}
	if responses[0] != "completed top priority story\n<CONTINUE>" {
		t.Fatalf("first execute response = %q, want continue after one story", responses[0])
	}
	if responses[1] != "all stories complete\n<COMPLETE>" {
		t.Fatalf("second execute response = %q, want complete after final story", responses[1])
	}

	prd := loadRalphInitSmokePRD(t, dir)
	if prd.BranchName != "ralph/document-processing-service" {
		t.Fatalf("branchName = %q, want %q", prd.BranchName, "ralph/document-processing-service")
	}
	if len(prd.RequestedChanges) == 0 {
		t.Fatal("requestedChanges should not be empty")
	}
	if prd.CustomerIntent == "" {
		t.Fatal("customerIntent should not be empty")
	}
	for _, story := range prd.UserStories {
		if !story.Passes {
			t.Fatalf("story %s remains incomplete in final prd.json: %#v", story.ID, prd.UserStories)
		}
		if len(story.AcceptanceCriteria) == 0 {
			t.Fatalf("story %s missing acceptance criteria: %#v", story.ID, story)
		}
	}
}

func TestIntegrationSmoke_RalphInitScaffoldExhaustsNonConvergingLoop(t *testing.T) {
	dir := initRalphSmokeScaffold(t)
	setWorkingDirectory(t, dir)
	writeRalphSmokeRequest(t, dir, "never-converges.md")

	runner := newRalphInitSmokeRunner(dir, ralphInitSmokeModeExhaust)
	harness := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProviderCommandRunner(runner),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	harness.RunUntilComplete(t, 15*time.Second)
	runner.Assert(t)

	harness.Assert().
		PlaceTokenCount("request:planned", 1).
		PlaceTokenCount("story:failed", 1).
		HasNoTokenInPlace("request:failed").
		HasNoTokenInPlace("story:init").
		HasNoTokenInPlace("story:complete")

	if got := runner.PlannerCalls(); got != 1 {
		t.Fatalf("planner calls = %d, want 1", got)
	}
	if got := runner.ExecutorCalls(); got != 8 {
		t.Fatalf("executor calls = %d, want 8 before guarded loop breaker", got)
	}
	if runner.ExecutorStoryWorkID() == "" {
		t.Fatal("executor story work ID was not captured")
	}

	snapshot, err := harness.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	assertDispatchHistoryContainsWorkstation(
		t,
		snapshot.DispatchHistory,
		"execute-story-loop-breaker",
		"story:failed",
		runner.ExecutorStoryWorkID(),
	)

	prd := loadRalphInitSmokePRD(t, dir)
	if len(prd.UserStories) == 0 || prd.UserStories[0].Passes {
		t.Fatalf("failed-loop prd.json unexpectedly marked stories complete: %#v", prd.UserStories)
	}
}

func initRalphSmokeScaffold(t *testing.T) string {
	t.Helper()

	dir := filepath.Join(t.TempDir(), "ralph-factory")
	if err := initcmd.Init(initcmd.InitConfig{Dir: dir, Type: string(initcmd.RalphScaffoldType)}); err != nil {
		t.Fatalf("Init ralph scaffold: %v", err)
	}
	return dir
}

func writeRalphSmokeRequest(t *testing.T, dir string, name string) {
	t.Helper()

	path := filepath.Join(dir, "inputs", initcmd.RalphFactoryInputType, "default", name)
	if err := os.WriteFile(path, []byte(ralphInitSmokeRequest), 0o644); err != nil {
		t.Fatalf("write request %s: %v", path, err)
	}
}

func newRalphInitSmokeRunner(rootDir string, mode ralphInitSmokeMode) *ralphInitSmokeRunner {
	return &ralphInitSmokeRunner{
		rootDir: rootDir,
		mode:    mode,
	}
}

func (r *ralphInitSmokeRunner) Run(_ context.Context, req workers.CommandRequest) (workers.CommandResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.workstationSequence = append(r.workstationSequence, req.WorkstationName)

	switch req.WorkstationName {
	case "plan-request":
		r.plannerCalls++
		workDir := r.requireWorkDir(req)
		if err := writeRalphInitSmokeArtifacts(workDir, plannedRalphInitSmokePRD()); err != nil {
			r.recordError("write plan artifacts: %v", err)
		}
		return workers.CommandResult{Stdout: []byte("planned artifacts ready\n<COMPLETE>")}, nil
	case "execute-story":
		r.executorCalls++
		r.captureStoryWorkID(req)
		workDir := r.requireWorkDir(req)
		prd, err := loadRalphInitSmokePRDFromPath(filepath.Join(workDir, "prd.json"))
		if err != nil {
			r.recordError("load prd before execute call %d: %v", r.executorCalls, err)
		}
		if err := r.verifyExecutorArtifacts(workDir); err != nil {
			r.recordError("verify executor artifacts before call %d: %v", r.executorCalls, err)
		} else {
			r.executorSawArtifacts = true
		}
		r.executorPRDHistory = append(r.executorPRDHistory, cloneRalphInitSmokePRD(prd))
		result := r.executeResult(workDir, prd)
		r.executorResponses = append(r.executorResponses, string(result.Stdout))
		return result, nil
	default:
		r.recordError("unexpected workstation %q", req.WorkstationName)
		return workers.CommandResult{Stdout: []byte("<COMPLETE>")}, nil
	}
}

func (r *ralphInitSmokeRunner) requireWorkDir(req workers.CommandRequest) string {
	if req.WorkDir == "" {
		r.recordError("%s request missing work dir", req.WorkstationName)
		return r.rootDir
	}
	if req.WorkDir != r.rootDir {
		r.recordError("%s work dir = %q, want %q", req.WorkstationName, req.WorkDir, r.rootDir)
	}
	return req.WorkDir
}

func (r *ralphInitSmokeRunner) executeResult(workDir string, prd ralphInitSmokePRD) workers.CommandResult {
	switch r.mode {
	case ralphInitSmokeModeComplete:
		return r.completeLoopResult(workDir, prd)
	case ralphInitSmokeModeExhaust:
		return workers.CommandResult{Stdout: []byte("still iterating\n<CONTINUE>")}
	default:
		r.recordError("unexpected smoke mode %q", r.mode)
		return workers.CommandResult{Stdout: []byte("<COMPLETE>")}
	}
}

func (r *ralphInitSmokeRunner) completeLoopResult(workDir string, prd ralphInitSmokePRD) workers.CommandResult {
	switch r.executorCalls {
	case 1:
		if len(prd.UserStories) < 2 {
			r.recordError("first execute call saw %d stories, want at least 2", len(prd.UserStories))
			return workers.CommandResult{Stdout: []byte("<CONTINUE>")}
		}
		prd.UserStories[0].Passes = true
		if err := writeRalphInitSmokeArtifacts(workDir, prd); err != nil {
			r.recordError("write prd after first execute call: %v", err)
		}
		return workers.CommandResult{Stdout: []byte("completed top priority story\n<CONTINUE>")}
	case 2:
		for i := range prd.UserStories {
			prd.UserStories[i].Passes = true
		}
		if err := writeRalphInitSmokeArtifacts(workDir, prd); err != nil {
			r.recordError("write prd after second execute call: %v", err)
		}
		return workers.CommandResult{Stdout: []byte("all stories complete\n<COMPLETE>")}
	default:
		r.recordError("complete-mode execute call %d exceeded planned iterations", r.executorCalls)
		return workers.CommandResult{Stdout: []byte("<COMPLETE>")}
	}
}

func (r *ralphInitSmokeRunner) verifyExecutorArtifacts(workDir string) error {
	for _, name := range []string{"prd.md", "prd.json", "progress.txt"} {
		if _, err := os.Stat(filepath.Join(workDir, name)); err != nil {
			return fmt.Errorf("missing %s: %w", name, err)
		}
	}
	return nil
}

func (r *ralphInitSmokeRunner) captureStoryWorkID(req workers.CommandRequest) {
	if r.executorStoryWorkID != "" {
		return
	}
	if len(req.Execution.WorkIDs) > 0 {
		r.executorStoryWorkID = req.Execution.WorkIDs[0]
		return
	}
	r.recordError("execute-story request missing execution work IDs")
}

func (r *ralphInitSmokeRunner) recordError(format string, args ...any) {
	r.internalErrors = append(r.internalErrors, fmt.Sprintf(format, args...))
}

func (r *ralphInitSmokeRunner) Assert(t *testing.T) {
	t.Helper()

	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.executorSawArtifacts {
		t.Fatal("executor never observed planned artifacts before running")
	}
	if len(r.internalErrors) > 0 {
		t.Fatalf("smoke runner errors: %v", r.internalErrors)
	}
}

func (r *ralphInitSmokeRunner) PlannerCalls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.plannerCalls
}

func (r *ralphInitSmokeRunner) ExecutorCalls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.executorCalls
}

func (r *ralphInitSmokeRunner) ExecutorStoryWorkID() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.executorStoryWorkID
}

func (r *ralphInitSmokeRunner) WorkstationSequence() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	sequence := make([]string, len(r.workstationSequence))
	copy(sequence, r.workstationSequence)
	return sequence
}

func (r *ralphInitSmokeRunner) ExecutorPRDHistory() []ralphInitSmokePRD {
	r.mu.Lock()
	defer r.mu.Unlock()

	history := make([]ralphInitSmokePRD, len(r.executorPRDHistory))
	for i, prd := range r.executorPRDHistory {
		history[i] = cloneRalphInitSmokePRD(prd)
	}
	return history
}

func (r *ralphInitSmokeRunner) ExecutorResponses() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	responses := make([]string, len(r.executorResponses))
	copy(responses, r.executorResponses)
	return responses
}

func plannedRalphInitSmokePRD() ralphInitSmokePRD {
	return ralphInitSmokePRD{
		Project:          "Document Processing Service",
		BranchName:       "ralph/document-processing-service",
		Description:      "Minimal PRD-to-execution loop smoke fixture.",
		RequestedChanges: []string{"Turn incoming requests into planning artifacts.", "Iterate one story at a time until the plan is complete."},
		CustomerIntent:   "Create a minimal, product-neutral release-planning loop that converts a request into aligned execution artifacts.",
		UserStories: []ralphInitSmokeUserStory{
			{
				ID:          "US-001",
				Title:       "Add request intake",
				Priority:    1,
				Passes:      false,
				Description: "Turn incoming requests into planning artifacts.",
				AcceptanceCriteria: []string{
					"plan-request writes prd.md, prd.json, and progress.txt in the working directory.",
					"prd.json includes branchName, requestedChanges, customerIntent, and the initial incomplete story list.",
				},
				Notes: []string{
					"Keep the generated plan product-neutral unless the request names a specific product.",
				},
			},
			{
				ID:          "US-002",
				Title:       "Execute one story at a time",
				Priority:    2,
				Passes:      false,
				Description: "Iterate the executor until the PRD completes.",
				AcceptanceCriteria: []string{
					"execute-story completes only the highest-priority incomplete story in each iteration.",
					"The executor returns <COMPLETE> only after every story in prd.json passes.",
				},
				Notes: []string{
					"Keep prd.md, prd.json, and progress.txt aligned with completed work.",
				},
			},
		},
	}
}

func writeRalphInitSmokeArtifacts(rootDir string, prd ralphInitSmokePRD) error {
	prdJSON, err := json.MarshalIndent(prd, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal prd.json: %w", err)
	}

	prdMD := fmt.Sprintf(
		"# %s\n\nBranch: `%s`\n\n%s\n\n## Requested Changes\n- %s\n- %s\n\n## Customer Intent\n%s\n\n## User Stories\n- [%s] %s\n- [%s] %s\n",
		prd.Project,
		prd.BranchName,
		prd.Description,
		prd.RequestedChanges[0],
		prd.RequestedChanges[1],
		prd.CustomerIntent,
		passMarker(prd.UserStories[0].Passes),
		prd.UserStories[0].Title,
		passMarker(prd.UserStories[1].Passes),
		prd.UserStories[1].Title,
	)
	progress := "## Codebase Patterns\n- Smoke runner-generated artifacts should exist before execute-story runs.\n"

	for path, data := range map[string][]byte{
		filepath.Join(rootDir, "prd.json"):     prdJSON,
		filepath.Join(rootDir, "prd.md"):       []byte(prdMD),
		filepath.Join(rootDir, "progress.txt"): []byte(progress),
	} {
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", filepath.Base(path), err)
		}
	}
	return nil
}

func loadRalphInitSmokePRD(t *testing.T, dir string) ralphInitSmokePRD {
	t.Helper()

	prd, err := loadRalphInitSmokePRDFromPath(filepath.Join(dir, "prd.json"))
	if err != nil {
		t.Fatalf("load prd.json: %v", err)
	}
	return prd
}

func loadRalphInitSmokePRDFromPath(path string) (ralphInitSmokePRD, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ralphInitSmokePRD{}, err
	}

	var prd ralphInitSmokePRD
	if err := json.Unmarshal(data, &prd); err != nil {
		return ralphInitSmokePRD{}, fmt.Errorf("unmarshal %s: %w", path, err)
	}
	return prd, nil
}

func cloneRalphInitSmokePRD(prd ralphInitSmokePRD) ralphInitSmokePRD {
	cloned := ralphInitSmokePRD{
		Project:          prd.Project,
		BranchName:       prd.BranchName,
		Description:      prd.Description,
		RequestedChanges: append([]string(nil), prd.RequestedChanges...),
		CustomerIntent:   prd.CustomerIntent,
		UserStories:      make([]ralphInitSmokeUserStory, len(prd.UserStories)),
	}
	for i, story := range prd.UserStories {
		cloned.UserStories[i] = ralphInitSmokeUserStory{
			ID:                 story.ID,
			Title:              story.Title,
			Priority:           story.Priority,
			Passes:             story.Passes,
			Description:        story.Description,
			AcceptanceCriteria: append([]string(nil), story.AcceptanceCriteria...),
			Notes:              append([]string(nil), story.Notes...),
		}
	}
	return cloned
}

func highestPriorityIncompleteStoryID(prd ralphInitSmokePRD) string {
	bestPriority := int(^uint(0) >> 1)
	bestID := ""
	for _, story := range prd.UserStories {
		if story.Passes {
			continue
		}
		if story.Priority < bestPriority {
			bestPriority = story.Priority
			bestID = story.ID
		}
	}
	return bestID
}

func passMarker(passes bool) string {
	if passes {
		return "x"
	}
	return " "
}

func assertRalphInitSmokeSequence(t *testing.T, got []string, want []string) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("workstation sequence = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("workstation sequence = %v, want %v", got, want)
		}
	}
}

var _ workers.CommandRunner = (*ralphInitSmokeRunner)(nil)
