package wire_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	factorydefinitioncomposition "github.com/portpowered/infinite-you/internal/testutil/factorydefinitionfixtures"
	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	automationswire "github.com/portpowered/infinite-you/pkg/services/automations/wire"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

// Fold-behavior preservation tests construct Automations exclusively through
// automations/wire and exercise reconcile, scheduler sidecars, hosted pollers,
// script pollers, and filesystem watchers through the published Service/Root
// surface after the internal composed-root relocation.

func TestWireFoldPreservesReconcileAdmissionThroughPublishedRoot(t *testing.T) {
	t.Parallel()

	service := validConstructionPorts(t).newService(t)
	peer, ok := service.(rootPeer)
	if !ok {
		t.Fatal("wire-constructed service does not expose Root()")
	}

	result, err := peer.Root().Reconcile(context.Background(), automations.ReconcileRequest{
		Desired: []automations.DesiredSpec{{
			AutomationID: "fold-preservation",
			SourceID:     "cron-source",
			Kind:         "schedule",
			State:        automations.DesiredLifecycleRunning,
		}},
		Observed: []automations.ObservedInstance{{
			AutomationID: "fold-preservation",
			SourceID:     "cron-source",
			InstanceID:   "instance-fold",
			State:        automations.ObservedLifecycleRunning,
		}},
	})
	if err != nil {
		t.Fatalf("Root().Reconcile() = %v", err)
	}
	if len(result.Outcomes) != 1 {
		t.Fatalf("Reconcile() outcomes = %+v, want one admission outcome", result.Outcomes)
	}
	if result.Outcomes[0].Convergence != automations.ConvergenceStatusConverged {
		t.Fatalf(
			"Reconcile() convergence = %q, want %q",
			result.Outcomes[0].Convergence,
			automations.ConvergenceStatusConverged,
		)
	}
}

func TestWireFoldPreservesSchedulerSidecarSupervision(t *testing.T) {
	start := time.Date(2026, time.April, 18, 12, 30, 0, 0, time.UTC)
	fakeClock := clockwork.NewFakeClockAt(start)
	factoryDir := t.TempDir()

	cronWS := cronWorkstationForFoldTest("scheduled-task")
	cronWS.Cron.TriggerAtStart = true
	scriptPoller := scriptPollerWorkstationForFoldTest()
	scriptWorker := scriptPollerWorkerForFoldTest()

	factoryCfg := &factorydefinitions.FactoryConfig{
		WorkTypes: []factorydefinitions.WorkTypeConfig{{Name: "task"}},
		Workers:   []factorydefinitions.FactoryWorkerConfig{{Name: scriptWorker.Name}},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{
			cronWS,
			scriptPoller,
		},
	}
	loaded, err := factorydefinitioncomposition.NewLoadedSource(
		factoryDir,
		factoryCfg,
		runtimefixtures.RuntimeDefinitionLookupFixture{
			Workers: map[string]*factorydefinitions.FactoryWorkerConfig{
				scriptWorker.Name: scriptWorker,
			},
			Workstations: map[string]*factorydefinitions.FactoryWorkstationConfig{
				cronWS.Name:       &cronWS,
				scriptPoller.Name: &scriptPoller,
			},
		},
		nil,
	)
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig: %v", err)
	}

	workRequestJSON := []byte(`{
		"requestId":"script-batch-fold",
		"type":"FACTORY_REQUEST_BATCH",
		"works":[{"name":"issue-script","workTypeName":"task","payload":{"id":"SCRIPT-FOLD"}}]
	}`)
	runner := &foldPollerCommandRunner{
		outcomes: []foldPollerOutcome{{result: workers.CommandResult{Stdout: workRequestJSON}}},
	}
	submitted := &foldRecordingSubmitter{}

	ports := validConstructionPorts(t)
	ports.clock = fakeClock
	ports.commandRunner = runner
	service := ports.newService(t)

	sidecarCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var sidecars sync.WaitGroup
	if err := service.StartSchedulerSidecarsForRuntime(
		sidecarCtx,
		&sidecars,
		factoryDir,
		factoryCfg,
		loaded,
		submitted.submit,
	); err != nil {
		t.Fatalf("StartSchedulerSidecarsForRuntime() = %v", err)
	}

	waitForFoldPollerSubmissions(t, submitted, 1, 2*time.Second)

	var cronRequest work.WorkRequest
	var foundCron bool
	_, submissions := submitted.snapshot()
	for _, request := range submissions {
		if len(request.Works) > 0 &&
			request.Works[0].Tags[factorydefinitions.TimeWorkTagKeyCronWorkstation] == cronWS.Name {
			cronRequest = request
			foundCron = true
			break
		}
	}
	if !foundCron {
		t.Fatal("expected cron trigger-at-start submission through wire-constructed service")
	}
	assertFoldCronWorkRequest(t, cronRequest, start, cronWS.Name)

	cancel()
	sidecars.Wait()
}

func TestWireFoldPreservesHostedPollerSupervisionFailure(t *testing.T) {
	startErr := errors.New("hosted poller unavailable")
	hostedPollers := foldProgrammableHostedPollers{
		Start: func(
			context.Context,
			*sync.WaitGroup,
			factorydefinitions.RuntimeConfigLookup,
			factorydefinitions.FactoryWorkstationConfig,
			*factorydefinitions.FactoryWorkerConfig,
			automations.HostedWorkSubmitter,
		) error {
			return startErr
		},
	}
	poller := hostedLinearWorkstationForFoldTest()
	worker := hostedLinearWorkerForFoldTest()
	factoryConfig := &factorydefinitions.FactoryConfig{
		Workers:      []factorydefinitions.FactoryWorkerConfig{*worker},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{poller},
	}
	runtimeConfig := runtimefixtures.RuntimeConfigLookupFixture{
		Factory:      factoryConfig,
		FactoryPath:  t.TempDir(),
		Workers:      map[string]*factorydefinitions.FactoryWorkerConfig{worker.Name: worker},
		Workstations: map[string]*factorydefinitions.FactoryWorkstationConfig{poller.Name: &poller},
	}

	ports := validConstructionPorts(t)
	ports.hostedSources = func(
		*zap.Logger,
		automations.HostedLinearClock,
		automations.HostedLinearHTTPDoer,
		automations.HostedLinearSecretResolver,
		string,
	) automations.HostedPollers {
		return hostedPollers
	}
	service := ports.newService(t)
	submitted := &foldRecordingSubmitter{}

	ctx, cancel := context.WithCancel(context.Background())
	var sidecars sync.WaitGroup
	err := service.StartSchedulerSidecarsForRuntime(
		ctx,
		&sidecars,
		runtimeConfig.FactoryDir(),
		factoryConfig,
		runtimeConfig,
		submitted.submit,
	)
	if !errors.Is(err, automations.ErrSupervisionFailed) || !errors.Is(err, startErr) {
		t.Fatalf("scheduler start error = %v, want typed supervision failure wrapping %v", err, startErr)
	}
	if calls, _ := submitted.snapshot(); calls != 0 {
		t.Fatalf("canonical Work submissions after failed start = %d, want 0", calls)
	}

	cancel()
	sidecars.Wait()
}

func TestWireFoldPreservesFilesystemWatcherFactoryAndPreseed(t *testing.T) {
	t.Parallel()

	dir := setupFoldFilesystemWatcherDir(t)
	batch := work.WorkRequest{
		RequestID: "fold-watcher-batch",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{
			{Name: "fold-work", WorkTypeID: "request", TraceID: "trace-fold"},
		},
	}
	data, err := json.Marshal(batch)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "request", "default", "batch.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	service := validConstructionPorts(t).newService(t)
	var submitCalls int
	var submitted work.WorkRequest
	watcher := service.NewFilesystemWatcher(automations.FilesystemWatcherConfig{
		Dir:            dir,
		Logger:         zap.NewNop(),
		KnownWorkTypes: []string{"request"},
		Files:          foldDiskFilesystemInputReader{},
		WalkDirectory:  filepath.WalkDir,
		WorkRequestIDs: func() string { return "fold-generated-id" },
		Submitter: func(_ context.Context, request work.WorkRequest) error {
			submitCalls++
			submitted = request
			return nil
		},
	})
	if watcher == nil {
		t.Fatal("NewFilesystemWatcher returned nil watcher")
	}
	if err := watcher.PreseedInputs(context.Background()); err != nil {
		t.Fatalf("PreseedInputs() = %v", err)
	}
	if submitCalls != 1 {
		t.Fatalf("submitter calls = %d, want 1", submitCalls)
	}
	if submitted.RequestID != batch.RequestID {
		t.Fatalf("submitted request ID = %q, want %q", submitted.RequestID, batch.RequestID)
	}
	if len(submitted.Works) != 1 || submitted.Works[0].TraceID != "trace-fold" {
		t.Fatalf("submitted works = %#v, want fold trace preserved", submitted.Works)
	}
}

func TestWireFoldPreservesHostedSourcesFactoryComposition(t *testing.T) {
	store, err := automationswire.NewHostedLinearCheckpointStore(platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("NewHostedLinearCheckpointStore() = %v", err)
	}

	var factoryCalls int
	ports := validConstructionPorts(t)
	ports.hostedSources = func(
		logger *zap.Logger,
		clock automations.HostedLinearClock,
		httpClient automations.HostedLinearHTTPDoer,
		secrets automations.HostedLinearSecretResolver,
		endpoint string,
	) automations.HostedPollers {
		factoryCalls++
		return automationswire.NewHostedSourcesFactory(store)(logger, clock, httpClient, secrets, endpoint)
	}

	service, err := automationswire.NewService(
		ports.logger,
		ports.clock,
		ports.commandRunner,
		"fold-hosted-factory",
		"",
		ports.hostedSources,
		nil,
		ports.hostedClock,
		nil,
		nil,
		"",
		ports.resolveTemplates,
		ports.executionPolicy,
	)
	if err != nil {
		t.Fatalf("NewService() = %v", err)
	}
	if service == nil {
		t.Fatal("NewService() returned nil service")
	}
	if factoryCalls != 1 {
		t.Fatalf("hosted-sources factory calls = %d, want exactly one wire composition call", factoryCalls)
	}
	var published automations.Service = service
	if published == nil {
		t.Fatal("constructed service is not assignable to automations.Service")
	}
}

type foldDiskFilesystemInputReader struct{}

func (foldDiskFilesystemInputReader) ReadDir(name string) ([]fs.DirEntry, error) {
	return os.ReadDir(name)
}
func (foldDiskFilesystemInputReader) ReadFile(name string) ([]byte, error) {
	return os.ReadFile(name)
}
func (foldDiskFilesystemInputReader) Stat(name string) (fs.FileInfo, error) {
	return os.Stat(name)
}

func setupFoldFilesystemWatcherDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "request", "default"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func cronWorkstationForFoldTest(name string) factorydefinitions.FactoryWorkstationConfig {
	return factorydefinitions.FactoryWorkstationConfig{
		Name: name,
		Kind: factorydefinitions.WorkstationKindCron,
		Cron: &factorydefinitions.CronConfig{Schedule: "* * * * *"},
		Outputs: []factorydefinitions.IOConfig{
			{WorkTypeName: "task", StateName: "init"},
		},
	}
}

func scriptPollerWorkstationForFoldTest() factorydefinitions.FactoryWorkstationConfig {
	return factorydefinitions.FactoryWorkstationConfig{
		Name:           "fold-script-ingress",
		Kind:           factorydefinitions.WorkstationKindPoller,
		WorkerTypeName: "fold-script-poller",
	}
}

func scriptPollerWorkerForFoldTest() *factorydefinitions.FactoryWorkerConfig {
	return &factorydefinitions.FactoryWorkerConfig{
		Name:    "fold-script-poller",
		Type:    factorydefinitions.WorkerTypeScript,
		Command: "factory/scripts/poller.sh",
	}
}

func hostedLinearWorkstationForFoldTest() factorydefinitions.FactoryWorkstationConfig {
	return factorydefinitions.FactoryWorkstationConfig{
		Name:           "fold-linear-ingress",
		Kind:           factorydefinitions.WorkstationKindPoller,
		WorkerTypeName: "fold-linear-poller",
	}
}

func hostedLinearWorkerForFoldTest() *factorydefinitions.FactoryWorkerConfig {
	return &factorydefinitions.FactoryWorkerConfig{
		Name:     "fold-linear-poller",
		Type:     factorydefinitions.WorkerTypeHosted,
		Provider: factorydefinitions.HostedWorkerProviderLinear,
		Auth:     &factorydefinitions.HostedWorkerAuthConfig{SecretRef: "secrets/linear-api-key"},
		Linear: &factorydefinitions.HostedLinearWorkerConfig{
			PollInterval: "1h",
			Mapping: factorydefinitions.HostedLinearWorkerMappingConfig{
				WorkType: "story",
				State:    "init",
			},
		},
	}
}

func TestWireFoldPreservesScriptPollerCursorRecoveryThroughPublishedRoot(t *testing.T) {
	t.Parallel()

	factoryDir := t.TempDir()
	poller := scriptPollerWorkstationForFoldTest()
	worker := scriptPollerWorkerForFoldTest()
	factoryConfig := &factorydefinitions.FactoryConfig{
		WorkTypes:    []factorydefinitions.WorkTypeConfig{{Name: "task"}},
		Workers:      []factorydefinitions.FactoryWorkerConfig{*worker},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{poller},
	}
	loaded, err := factorydefinitioncomposition.NewLoadedSource(
		factoryDir,
		factoryConfig,
		runtimefixtures.RuntimeDefinitionLookupFixture{
			Workers:      map[string]*factorydefinitions.FactoryWorkerConfig{worker.Name: worker},
			Workstations: map[string]*factorydefinitions.FactoryWorkstationConfig{poller.Name: &poller},
		},
		nil,
	)
	if err != nil {
		t.Fatalf("NewLoadedSource() = %v", err)
	}

	runner := &foldPollerCommandRunner{outcomes: []foldPollerOutcome{{
		result: workers.CommandResult{Stdout: []byte(`{
			"requestId":"script-recovery-fold",
			"type":"FACTORY_REQUEST_BATCH",
			"works":[{"name":"issue-recovery","workTypeName":"task"}],
			"cursor":"cursor-fold-2",
			"checkpoint":"checkpoint-fold-2"
		}`)},
	}}}
	submitted := &foldRecordingSubmitter{}
	ports := validConstructionPorts(t)
	ports.commandRunner = runner
	service := ports.newService(t)

	err = service.RunScriptPoller(
		context.Background(),
		runner,
		loaded,
		poller,
		worker,
		submitted.submit,
	)
	if err == nil || !strings.Contains(err.Error(), "exited unexpectedly") {
		t.Fatalf("RunScriptPoller() = %v, want unexpected exit after committed recovery", err)
	}
	if calls, _ := submitted.snapshot(); calls != 1 {
		t.Fatalf("script poller submissions = %d, want 1", calls)
	}

	instanceID := foldScriptPollerInstanceID("automations-wire", poller.Name)
	root := service.Root()
	cursor, err := root.GetCursor(context.Background(), automations.GetCursorRequest{
		InstanceID: instanceID,
	})
	if err != nil {
		t.Fatalf("published Root().GetCursor() = %v", err)
	}
	if cursor.AutomationID != "automations-wire" ||
		cursor.InstanceID != instanceID ||
		cursor.Cursor != "cursor-fold-2" ||
		cursor.Checkpoint != "checkpoint-fold-2" {
		t.Fatalf("published Root().GetCursor() = %+v, want committed recovery facts", cursor)
	}

	_, err = root.GetCursor(context.Background(), automations.GetCursorRequest{
		InstanceID:     instanceID,
		ExpectedCursor: "stale-cursor",
	})
	var typed *automations.Error
	if !errors.As(err, &typed) || typed.Code != automations.ErrorCodeConflict {
		t.Fatalf("published Root().GetCursor(stale) = %v, want typed conflict", err)
	}
	if !errors.Is(err, automations.ErrConflict) {
		t.Fatalf("published Root().GetCursor(stale) = %v, want ErrConflict", err)
	}
}

func foldScriptPollerInstanceID(automationID, workstationName string) string {
	sourceID := "script-poller:" + strings.TrimSpace(workstationName)
	identity := fmt.Sprintf(
		"%d:%s:%d:%s",
		len(automationID),
		automationID,
		len(sourceID),
		sourceID,
	)
	sum := sha256.Sum256([]byte("automations-script-poller-instance:" + identity))
	return "script-poller-instance:" + hex.EncodeToString(sum[:16])
}

type foldRecordingSubmitter struct {
	mu          sync.Mutex
	calls       int
	submissions []work.WorkRequest
}

func (r *foldRecordingSubmitter) submit(_ context.Context, request work.WorkRequest) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	r.submissions = append(r.submissions, request)
	return nil
}

func (r *foldRecordingSubmitter) snapshot() (int, []work.WorkRequest) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls, append([]work.WorkRequest(nil), r.submissions...)
}

type foldProgrammableHostedPollers struct {
	Start func(
		context.Context,
		*sync.WaitGroup,
		factorydefinitions.RuntimeConfigLookup,
		factorydefinitions.FactoryWorkstationConfig,
		*factorydefinitions.FactoryWorkerConfig,
		automations.HostedWorkSubmitter,
	) error
}

func (p foldProgrammableHostedPollers) StartLinearPoller(
	ctx context.Context,
	sidecars *sync.WaitGroup,
	runtimeConfig factorydefinitions.RuntimeConfigLookup,
	workstation factorydefinitions.FactoryWorkstationConfig,
	worker *factorydefinitions.FactoryWorkerConfig,
	submitter automations.HostedWorkSubmitter,
) error {
	if p.Start == nil {
		return nil
	}
	return p.Start(ctx, sidecars, runtimeConfig, workstation, worker, submitter)
}

func (p foldProgrammableHostedPollers) ValidateLinearPoller(
	factorydefinitions.RuntimeConfigLookup,
	factorydefinitions.FactoryWorkstationConfig,
	*factorydefinitions.FactoryWorkerConfig,
	automations.HostedWorkSubmitter,
) error {
	return nil
}

type foldPollerOutcome struct {
	result workers.CommandResult
	err    error
}

type foldPollerCommandRunner struct {
	mu       sync.Mutex
	calls    int
	outcomes []foldPollerOutcome
}

func (r *foldPollerCommandRunner) Run(ctx context.Context, _ workers.CommandRequest) (workers.CommandResult, error) {
	r.mu.Lock()
	index := r.calls
	r.calls++
	var outcome foldPollerOutcome
	if index < len(r.outcomes) {
		outcome = r.outcomes[index]
	} else if len(r.outcomes) > 0 {
		outcome = r.outcomes[len(r.outcomes)-1]
	}
	r.mu.Unlock()

	return outcome.result, outcome.err
}

func waitForFoldPollerSubmissions(t *testing.T, submitted *foldRecordingSubmitter, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		calls, _ := submitted.snapshot()
		if calls >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	calls, _ := submitted.snapshot()
	t.Fatalf("timed out waiting for %d poller submission(s); got %d", want, calls)
}

func assertFoldCronWorkRequest(
	t *testing.T,
	request work.WorkRequest,
	want time.Time,
	workstation string,
) {
	t.Helper()
	got := request.Works[0].Tags[factorydefinitions.TimeWorkTagKeyNominalAt]
	wantTag := want.Format(time.RFC3339Nano)
	if got != wantTag {
		t.Fatalf("cron nominal_at tag = %q, want %q", got, wantTag)
	}
	if got := request.Works[0].Tags[factorydefinitions.TimeWorkTagKeyCronWorkstation]; got != workstation {
		t.Fatalf("cron workstation tag = %q, want %q", got, workstation)
	}
}
