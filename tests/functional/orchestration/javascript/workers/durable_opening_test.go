package workers_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// The public identity-selecting Start contract is needed here because HTTP
// opening cannot select an identity. Invocation still enters through the CLI.
// Retry and replacement are ordered: reuse of the selected identity and paths
// is the customer invariant, while the peer remains admitted at its own edge.
func TestJavaScriptDurableOpeningLifecycle(t *testing.T) {
	t.Parallel()
	router := newJavaScriptSharedCommandRouter()
	api := support.NewProcessAPIServer()
	peer := newDurableOpeningScenario(t, "opening peer", factorysessions.PersistencePolicyDisabled)
	candidate := newDurableOpeningScenario(t, "opening candidate", factorysessions.PersistencePolicyEnabled)
	files := &durableOpeningFiles{
		failedPath: filepath.Join(candidate.dir, ".you-agent-factory", "durable-sessions", candidate.id+".json"),
		peerPath:   filepath.Join(peer.dir, ".you-agent-factory", "durable-sessions", peer.id+".json"),
	}
	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{
		ProviderCommandRunner: router, APIServerStarter: api.Start,
		FactorySessionRuntimePersistenceFileSystem: files,
	})
	if err != nil {
		t.Fatal(err)
	}
	support.CleanupProcess(t, process)
	host := support.ScaffoldFactory(t, permissionMatrixFactoryConfig("return 'host';"))
	home := t.TempDir()
	inputs := support.FakeInputs(t.Context(), []string{"you", "run", "--dir", host, "--continuously", "--with-server", "--quiet", "--no-record"})
	inputs.Input.Env = javascriptSharedCustomerEnvironment(home)
	inputs.Input.WorkingDirectory = host
	command := support.StartProcessCommand(t, process, inputs.Input)
	t.Cleanup(func() { command.Stop(t) })
	baseURL := api.WaitForURL(t)
	sessions := process.FactorySessions().FactorySessions().(factorysessions.Service)
	peer.open(t, sessions)
	peerRunner := peer.runner(t, router)
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	peerResult := peer.invoke(t, ctx, process, baseURL)
	awaitDurableOpeningChild(t, ctx, peerRunner, peerResult)
	before, err := sessions.SubscribeFactoryEventsForSession(ctx, peer.id, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = sessions.Start(ctx, candidate.request())
	var resumeErr *factorysessions.ResumeError
	if !errors.As(err, &resumeErr) || resumeErr.Outcome != factorysessions.ResumeOutcomeCorruptedPersistence || files.failures.Load() != 1 {
		t.Fatalf("selected persistence opening = %v, faults=%d; want existing corrupted-persistence classification after one-shot read fault", err, files.failures.Load())
	}
	_, err = sessions.Get(ctx, factorysessions.SessionGetRequest{SessionID: candidate.id, Mode: factorysessions.SessionOperationModeLive})
	if !errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("failed candidate was published: %v", err)
	}
	verifyDurableOpeningReplacement(t, ctx, process, sessions, baseURL, router, peer, candidate, peerRunner, peerResult)
	after, err := sessions.SubscribeFactoryEventsForSession(ctx, peer.id, nil)
	if err != nil {
		t.Fatal(err)
	}
	if before.StreamGenerationID != after.StreamGenerationID || len(after.History) < len(before.History) || !reflect.DeepEqual(before.History, after.History[:len(before.History)]) {
		t.Fatal("candidate failure/replacement changed the peer's retained generation/history")
	}
	if files.peerReads.Load() != 0 {
		t.Fatal("disabled peer accessed durable persistence")
	}
	assertDurableOpeningSelections(t, router, peer, candidate)
	t.Log("F-RETRY/F-REPLACE: one-shot selected persistence read failure, unpublished candidate, same-ID retry/replacement and retained live peer proved")
}

func assertDurableOpeningSelections(t *testing.T, router *javascriptSharedCommandRouter, scenarios ...durableOpeningScenario) {
	t.Helper()
	for _, scenario := range scenarios {
		observed := 0
		for _, request := range router.requestRecords() {
			if !strings.Contains(string(request.Stdin), scenario.marker) {
				continue
			}
			observed++
			model := strings.ReplaceAll(scenario.marker, " ", "-")
			if request.Command != "codex" || !strings.Contains(strings.Join(request.Args, " "), model) || filepath.Clean(request.WorkDir) != filepath.Clean(scenario.dir) {
				t.Fatalf("selected preset/project was lost: %+v", request)
			}
		}
		if observed == 0 {
			t.Fatalf("no selected child command for %q", scenario.marker)
		}
	}
}

func verifyDurableOpeningReplacement(t *testing.T, ctx context.Context, process support.Process, sessions factorysessions.Service, baseURL string, router *javascriptSharedCommandRouter, peer, candidate durableOpeningScenario, peerRunner *runtimeJavaScriptCommandRunner, peerResult <-chan durableOpeningCommandResult) {
	t.Helper()
	candidate.open(t, sessions)
	candidateRunner := candidate.runner(t, router)
	candidateResult := candidate.invoke(t, ctx, process, baseURL)
	awaitDurableOpeningChild(t, ctx, candidateRunner, candidateResult)
	candidateRunner.unblock()
	assertDurableOpeningOutput(t, ctx, candidateResult, candidate.marker, peer.marker)
	// Closing the selected live scope must release its acquired owner, so the
	// exact same identity/root can open again while the peer runs. This witness
	// selects a new recording for the new generation; retained recording reuse
	// is a separate recovery edge, not proved by this resource-retirement check.
	closeDurableOpening(t, sessions, candidate.id)
	candidate.record = filepath.Join(filepath.Dir(candidate.record), "replacement.jsonl")
	candidate.open(t, sessions)
	// A new request on the replacement executes again; the reusable provider
	// edge returns the same selected output without enforcing a single call.
	if err := router.unregister(candidate.marker); err != nil {
		t.Fatal(err)
	}
	replacement := support.NewRecordingCommandRunner(candidate.marker + " output")
	if err := router.register(candidate.marker, replacement); err != nil {
		t.Fatal(err)
	}
	assertDurableOpeningOutput(t, ctx, candidate.invoke(t, ctx, process, baseURL), candidate.marker, peer.marker)
	if replacement.CallCount() != 1 {
		t.Fatalf("replacement calls=%d", replacement.CallCount())
	}
	select {
	case result := <-peerResult:
		t.Fatalf("candidate retired gated peer: %+v", result)
	default:
	}
	peerRunner.unblock()
	assertDurableOpeningOutput(t, ctx, peerResult, peer.marker, candidate.marker)
	if peerRunner.calls.Load() != 1 {
		t.Fatalf("peer calls=%d", peerRunner.calls.Load())
	}
}

var errDurableOpeningRead = errors.New("selected opening persistence unavailable")

type durableOpeningFiles struct {
	platformfilesystem.Local
	failedPath string
	peerPath   string
	failed     atomic.Bool
	failures   atomic.Int32
	peerReads  atomic.Int32
}

func (files *durableOpeningFiles) ReadFile(path string) ([]byte, error) {
	return files.ReadFileBounded(path, 64<<20)
}

func (files *durableOpeningFiles) ReadFileBounded(path string, limit int64) ([]byte, error) {
	if filepath.Clean(path) == filepath.Clean(files.peerPath) {
		files.peerReads.Add(1)
	}
	if filepath.Clean(path) == filepath.Clean(files.failedPath) && files.failed.CompareAndSwap(false, true) {
		files.failures.Add(1)
		return nil, errDurableOpeningRead
	}
	return platformfilesystem.NewRecovery(files.Local, files.Local).ReadFileBounded(path, limit)
}

func (files *durableOpeningFiles) RenameNoReplace(source, destination string) error {
	return platformfilesystem.NewRecovery(files.Local, files.Local).RenameNoReplace(source, destination)
}

type durableOpeningScenario struct {
	id, dir, home, marker, record string
	policy                        factorysessions.PersistencePolicy
}

func newDurableOpeningScenario(t *testing.T, marker string, policy factorysessions.PersistencePolicy) durableOpeningScenario {
	t.Helper()
	home := t.TempDir()
	configDir := filepath.Join(home, ".you-agent-factory")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatal(err)
	}
	config := fmt.Sprintf(`{"workerPresets":[{"id":"opening-worker","modelProvider":"codex","model":%q}]}`, strings.ReplaceAll(marker, " ", "-"))
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	workflow := fmt.Sprintf(`return (async function(){return await agent.run({preset:"opening-worker",prompt:%q});})();`, marker)
	dir := support.ScaffoldFactory(t, permissionMatrixFactoryConfig(workflow))
	return durableOpeningScenario{id: uuid.NewString(), dir: dir, home: home, marker: marker, policy: policy, record: filepath.Join(t.TempDir(), "opening.jsonl")}
}

func (scenario durableOpeningScenario) request() factorysessions.SessionStartRequest {
	return factorysessions.SessionStartRequest{SessionID: scenario.id, Mode: factorysessions.SessionOperationModeLive, FolderPath: scenario.dir, ActivationOnly: true, Persistence: scenario.policy,
		RuntimeSelection: &factorysessions.SessionRuntimeSelection{Mode: factorysessions.SessionRuntimeModeService, SystemConfigHome: scenario.home, DefinitionSourcePath: filepath.Join(scenario.dir, "factory.json"), ExecutionBaseDir: scenario.dir, RuntimeInstanceID: uuid.NewString(), Recording: factorysessions.SessionRecordingSelection{RecordPath: scenario.record}},
	}
}

func (scenario durableOpeningScenario) open(t *testing.T, sessions factorysessions.Service) {
	t.Helper()
	result, err := sessions.Start(t.Context(), scenario.request())
	if err != nil || result.SessionID != scenario.id {
		t.Fatalf("open selected session: %+v %v", result, err)
	}
	t.Cleanup(func() {
		_, err := sessions.Get(context.Background(), factorysessions.SessionGetRequest{SessionID: scenario.id, Mode: factorysessions.SessionOperationModeLive})
		if errors.Is(err, factorysessions.ErrSessionNotFound) {
			return
		}
		closeDurableOpening(t, sessions, scenario.id)
	})
}

func closeDurableOpening(t *testing.T, sessions factorysessions.Service, id string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	result, err := sessions.Control(ctx, factorysessions.SessionControlRequest{SessionID: id, Mode: factorysessions.SessionOperationModeLive, Operation: factorysessions.SessionControlClose})
	if err != nil || !result.Closed {
		t.Fatalf("close selected session: %+v %v", result, err)
	}
}

func (scenario durableOpeningScenario) runner(t *testing.T, router *javascriptSharedCommandRouter) *runtimeJavaScriptCommandRunner {
	t.Helper()
	runner := &runtimeJavaScriptCommandRunner{marker: scenario.marker, started: make(chan struct{}), release: make(chan struct{})}
	if err := router.register(scenario.marker, runner); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		runner.unblock()
		if err := router.unregister(scenario.marker); err != nil {
			t.Error(err)
		}
	})
	return runner
}

type durableOpeningCommandResult struct {
	stdout, stderr string
	err            error
}

func (scenario durableOpeningScenario) invoke(t *testing.T, ctx context.Context, process support.Process, baseURL string) <-chan durableOpeningCommandResult {
	t.Helper()
	inputs := support.FakeInputs(ctx, []string{"you", "--json", "--remote", "--server", baseURL, "--session", scenario.id, "run", "--factory", filepath.Join(scenario.dir, "factory.json"), "--output", "primary", "--no-record", scenario.marker})
	inputs.Input.Env = javascriptSharedCustomerEnvironment(scenario.home)
	inputs.Input.WorkingDirectory = scenario.dir
	results := make(chan durableOpeningCommandResult, 1)
	go func() {
		err := process.Execute(inputs.Input)
		results <- durableOpeningCommandResult{stdout: inputs.Stdout(), stderr: inputs.Stderr(), err: err}
	}()
	return results
}

func awaitDurableOpeningChild(t *testing.T, ctx context.Context, runner *runtimeJavaScriptCommandRunner, results <-chan durableOpeningCommandResult) {
	t.Helper()
	select {
	case <-runner.started:
	case result := <-results:
		t.Fatalf("invocation returned before child admission: %+v", result)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

func assertDurableOpeningOutput(t *testing.T, ctx context.Context, results <-chan durableOpeningCommandResult, own, foreign string) {
	t.Helper()
	select {
	case result := <-results:
		if result.err != nil || !strings.Contains(result.stdout, own+" output") || strings.Contains(result.stdout, foreign) {
			t.Fatalf("selected output: %+v", result)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}
