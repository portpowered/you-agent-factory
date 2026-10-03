package internal

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	factorydefinitioncomposition "github.com/portpowered/infinite-you/internal/testutil/factorydefinitionfixtures"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	cron "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/cron"
	cronwire "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/cron/wire"
	filesystemwatchers "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/filesystem_watchers"
	fswire "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/filesystem_watchers/wire"
	scriptpollers "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/script_pollers"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestRuntimeLifecycle_IsolatesOwnersAndClassifiesDuplicates(t *testing.T) {
	service := New(zap.NewNop(), nil, &internalScriptPollerRunner{}, "", "", nil, nil, nil, cronwire.NewService(), fswire.NewService())
	root := service.Root()
	ctx := context.Background()

	alpha := runtimeActivationRequestForTest("runtime-alpha", "alpha")
	beta := runtimeActivationRequestForTest("runtime-beta", "beta")
	if _, err := root.ActivateRuntime(ctx, alpha); err != nil {
		t.Fatalf("ActivateRuntime(alpha) error = %v", err)
	}
	if _, err := root.ActivateRuntime(ctx, beta); err != nil {
		t.Fatalf("ActivateRuntime(beta) error = %v", err)
	}

	if service.runtimes[alpha.RuntimeID] == service.runtimes[beta.RuntimeID] {
		t.Fatal("runtime activations share an Automations owner")
	}
	alpha.Snapshot.EffectiveFactory.Name = "mutated-after-activation"
	duplicate, err := root.ActivateRuntime(ctx, runtimeActivationRequestForTest("runtime-alpha", "alpha"))
	if err != nil {
		t.Fatalf("ActivateRuntime(duplicate) error = %v", err)
	}
	if !duplicate.Idempotent || duplicate.State != automations.RuntimeLifecycleActivated {
		t.Fatalf("duplicate result = %#v, want idempotent activated result", duplicate)
	}

	_, err = root.ActivateRuntime(ctx, runtimeActivationRequestForTest("runtime-alpha", "changed"))
	if !errors.Is(err, automations.ErrConflict) {
		t.Fatalf("ActivateRuntime(conflict) error = %v, want conflict", err)
	}

	stopped, err := root.DeactivateRuntime(ctx, automations.RuntimeDeactivationRequest{RuntimeID: alpha.RuntimeID})
	if err != nil || stopped.State != automations.RuntimeLifecycleStopped {
		t.Fatalf("DeactivateRuntime(alpha) = %#v, %v", stopped, err)
	}
	if _, ok := service.runtimes[beta.RuntimeID]; !ok {
		t.Fatal("deactivating alpha removed beta runtime state")
	}
	idempotent, err := root.DeactivateRuntime(ctx, automations.RuntimeDeactivationRequest{RuntimeID: alpha.RuntimeID})
	if err != nil || !idempotent.Idempotent {
		t.Fatalf("DeactivateRuntime(alpha again) = %#v, %v, want idempotent", idempotent, err)
	}
	if _, err := root.DeactivateRuntime(ctx, automations.RuntimeDeactivationRequest{RuntimeID: beta.RuntimeID}); err != nil {
		t.Fatalf("DeactivateRuntime(beta) error = %v", err)
	}
}

func TestRuntimeLifecycle_RejectsMissingIdentityWithTypedError(t *testing.T) {
	service := New(zap.NewNop(), nil, &internalScriptPollerRunner{}, "", "", nil, nil, nil, cronwire.NewService(), fswire.NewService())
	_, err := service.Root().ActivateRuntime(context.Background(), automations.RuntimeActivationRequest{})
	var typed *automations.Error
	if !errors.As(err, &typed) || typed.Code != automations.ErrorCodeInvalid {
		t.Fatalf("ActivateRuntime(empty) error = %v, want typed invalid error", err)
	}
}

func TestRuntimeLifecycle_RejectsBehavioralInputConflictsWithSameSnapshot(t *testing.T) {
	service := New(zap.NewNop(), nil, &internalScriptPollerRunner{}, "", "", nil, nil, nil, cronwire.NewService(), fswire.NewService())
	base := runtimeActivationRequestForTest("runtime-input-conflict", "same")
	if _, err := service.ActivateRuntime(context.Background(), base); err != nil {
		t.Fatalf("ActivateRuntime(base) error = %v", err)
	}

	schedulerConflict := base
	schedulerConflict.Inputs.StartSchedulers = true
	if _, err := service.ActivateRuntime(context.Background(), schedulerConflict); !errors.Is(err, automations.ErrConflict) {
		t.Fatalf("ActivateRuntime(scheduler conflict) error = %v, want conflict", err)
	}

	filesystemConflict := base
	filesystemConflict.Inputs.Filesystem.KnownWorkTypes = []string{"different-work-type"}
	if _, err := service.ActivateRuntime(context.Background(), filesystemConflict); !errors.Is(err, automations.ErrConflict) {
		t.Fatalf("ActivateRuntime(filesystem conflict) error = %v, want conflict", err)
	}

	if _, err := service.ActivateRuntime(context.Background(), base); err != nil {
		t.Fatalf("ActivateRuntime(unchanged duplicate) error = %v, want idempotent success", err)
	}
}

func TestRuntimeLifecycle_TreatsEquivalentOpaqueEffectsAsIdempotent(t *testing.T) {
	service := New(zap.NewNop(), nil, &internalScriptPollerRunner{}, "", "", nil, nil, nil, cronwire.NewService(), fswire.NewService())
	base := runtimeActivationRequestForTest("runtime-opaque-effects", "same")
	base.Inputs.Submitter = func(context.Context, work.WorkRequest) error { return nil }
	if _, err := service.ActivateRuntime(context.Background(), base); err != nil {
		t.Fatalf("ActivateRuntime(base) error = %v", err)
	}

	duplicate := runtimeActivationRequestForTest("runtime-opaque-effects", "same")
	duplicate.Inputs.Submitter = func(context.Context, work.WorkRequest) error { return nil }
	result, err := service.ActivateRuntime(context.Background(), duplicate)
	if err != nil {
		t.Fatalf("ActivateRuntime(equivalent opaque effects) error = %v", err)
	}
	if !result.Idempotent || result.State != automations.RuntimeLifecycleActivated {
		t.Fatalf("ActivateRuntime(equivalent opaque effects) = %#v, want idempotent activated result", result)
	}
}

func TestRuntimeLifecycle_StartsAndStopsSchedulerOwnership(t *testing.T) {
	service := New(zap.NewNop(), nil, &internalScriptPollerRunner{}, "", "", nil, nil, nil, cronwire.NewService(), fswire.NewService())
	request := runtimeActivationRequestForTest("runtime-scheduler", "scheduler")
	request.Inputs.StartSchedulers = true
	request.Inputs.Submitter = func(context.Context, work.WorkRequest) error { return nil }
	if _, err := service.ActivateRuntime(context.Background(), request); err != nil {
		t.Fatalf("ActivateRuntime() error = %v", err)
	}
	if err := service.StartRuntime(context.Background(), request.RuntimeID); err != nil {
		t.Fatalf("StartRuntime() error = %v", err)
	}
	if err := service.StartRuntime(context.Background(), request.RuntimeID); err != nil {
		t.Fatalf("StartRuntime() duplicate error = %v", err)
	}
	if _, err := service.DeactivateRuntime(context.Background(), automations.RuntimeDeactivationRequest{RuntimeID: request.RuntimeID}); err != nil {
		t.Fatalf("DeactivateRuntime() error = %v", err)
	}
}

func TestRuntimeLifecycle_ReportsFilesystemWatcherTermination(t *testing.T) {
	watchErr := errors.New("watch sidecar failed with distinctive detail")
	tests := []struct {
		name       string
		watch      func(context.Context) error
		message    string
		level      zapcore.Level
		reason     string
		errorValue string
	}{
		{
			name:       "error",
			watch:      func(context.Context) error { return watchErr },
			message:    "filesystem watcher stopped with error",
			level:      zapcore.ErrorLevel,
			errorValue: watchErr.Error(),
		},
		{
			name:    "event stream closure",
			watch:   func(context.Context) error { return nil },
			message: "filesystem watcher stopped unexpectedly",
			level:   zapcore.WarnLevel,
			reason:  "event stream closed",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			logCore, observedLogs := observer.New(zap.InfoLevel)
			instance, cancel := runtimeInstanceForWatcherTest(zap.New(logCore), test.watch)
			defer cancel()

			if err := instance.start(context.Background()); err != nil {
				t.Fatalf("runtimeInstance.start() error = %v", err)
			}
			instance.sidecars.Wait()

			entries := observedLogs.FilterMessage(test.message).All()
			if len(entries) != 1 {
				t.Fatalf("%s logs = %d, want 1", test.message, len(entries))
			}
			entry := entries[0]
			if entry.Level != test.level {
				t.Fatalf("termination log level = %v, want %v", entry.Level, test.level)
			}
			contextMap := entry.ContextMap()
			if got := contextMap["runtime_id"]; got != "runtime-watcher-diagnostic" {
				t.Fatalf("runtime_id = %#v, want runtime-watcher-diagnostic", got)
			}
			if got := contextMap["watch_root"]; got != "/factories/example/inputs" {
				t.Fatalf("watch_root = %#v, want /factories/example/inputs", got)
			}
			if test.reason != "" && contextMap["reason"] != test.reason {
				t.Fatalf("termination reason = %#v, want %s", contextMap["reason"], test.reason)
			}
			if test.errorValue != "" && contextMap["error"] != test.errorValue {
				t.Fatalf("termination error = %#v, want %s", contextMap["error"], test.errorValue)
			}
		})
	}
}

func TestRuntimeLifecycle_DoesNotReportFilesystemWatcherCancellation(t *testing.T) {
	logCore, observedLogs := observer.New(zap.InfoLevel)
	instance, cancel := runtimeInstanceForWatcherTest(zap.New(logCore), func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})

	if err := instance.start(context.Background()); err != nil {
		t.Fatalf("runtimeInstance.start() error = %v", err)
	}
	cancel()
	instance.sidecars.Wait()

	if got := observedLogs.FilterMessage("filesystem watcher stopped with error").Len(); got != 0 {
		t.Fatalf("watcher error logs after context cancellation = %d, want 0", got)
	}
	if got := observedLogs.FilterMessage("filesystem watcher stopped unexpectedly").Len(); got != 0 {
		t.Fatalf("watcher terminal logs after context cancellation = %d, want 0", got)
	}
}

type runtimeLifecycleWatcher struct {
	watch func(context.Context) error
}

func (w runtimeLifecycleWatcher) PreseedInputs(context.Context) error { return nil }

func (w runtimeLifecycleWatcher) Watch(ctx context.Context) error {
	if w.watch == nil {
		return nil
	}
	return w.watch(ctx)
}

func runtimeInstanceForWatcherTest(
	logger *zap.Logger,
	watch func(context.Context) error,
) (*runtimeInstance, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	return &runtimeInstance{
		runtimeID:   "runtime-watcher-diagnostic",
		watcherRoot: "/factories/example/inputs",
		owner:       New(logger, nil, &internalScriptPollerRunner{}, "", "", nil, nil, nil, cronwire.NewService(), fswire.NewService()),
		watcher:     runtimeLifecycleWatcher{watch: watch},
		ctx:         ctx,
		cancel:      cancel,
	}, cancel
}

func TestRuntimeLifecycle_SnapshotConfigAndInputHelpers(t *testing.T) {
	config := newRuntimeSnapshotConfigForTest()
	assertRuntimeSnapshotIdentity(t, config)
	assertRuntimeSnapshotLookups(t, config)
	assertNilRuntimeSnapshotConfig(t)
	validStates := map[string]map[string]bool{"task": {"ready": true}}
	assertValidStatesClone(t, validStates)
	assertRuntimeFilesystemConfig(t, validStates)
}

func newRuntimeSnapshotConfigForTest() *runtimeSnapshotConfig {
	worker := factorydefinitions.FactoryWorkerConfig{Name: "worker-a"}
	workstation := factorydefinitions.FactoryWorkstationConfig{Name: "workstation-a"}
	return &runtimeSnapshotConfig{
		factoryDir:     "/factories/example",
		runtimeBaseDir: "/runtime/example",
		runtimeID:      "runtime-example",
		config: factorydefinitions.FactoryConfig{
			Name:         "example",
			Workers:      []factorydefinitions.FactoryWorkerConfig{worker},
			Workstations: []factorydefinitions.FactoryWorkstationConfig{workstation},
		},
	}
}

func assertRuntimeSnapshotIdentity(t *testing.T, config *runtimeSnapshotConfig) {
	t.Helper()
	if config.FactoryDir() != "/factories/example" || config.RuntimeBaseDir() != "/runtime/example" || config.RuntimeInstanceID() != "runtime-example" {
		t.Fatalf("runtimeSnapshotConfig identity = %q, %q, %q", config.FactoryDir(), config.RuntimeBaseDir(), config.RuntimeInstanceID())
	}
	if got := config.FactoryConfig(); got == nil || got.Name != "example" {
		t.Fatalf("FactoryConfig() = %#v, want example", got)
	}
}

func assertRuntimeSnapshotLookups(t *testing.T, config *runtimeSnapshotConfig) {
	t.Helper()
	if got, ok := config.Worker("worker-a"); !ok || got == nil || got.Name != "worker-a" {
		t.Fatalf("Worker(worker-a) = %#v, %v", got, ok)
	}
	if _, ok := config.Worker("missing"); ok {
		t.Fatal("Worker(missing) unexpectedly resolved")
	}
	if got, ok := config.Workstation("workstation-a"); !ok || got == nil || got.Name != "workstation-a" {
		t.Fatalf("Workstation(workstation-a) = %#v, %v", got, ok)
	}
	if _, ok := config.Workstation("missing"); ok {
		t.Fatal("Workstation(missing) unexpectedly resolved")
	}
}

func assertNilRuntimeSnapshotConfig(t *testing.T) {
	t.Helper()
	var nilConfig *runtimeSnapshotConfig
	if nilConfig.FactoryDir() != "" || nilConfig.RuntimeBaseDir() != "" || nilConfig.RuntimeInstanceID() != "" || nilConfig.FactoryConfig() != nil {
		t.Fatal("nil runtimeSnapshotConfig returned non-empty identity")
	}
	if _, ok := nilConfig.Worker("worker-a"); ok {
		t.Fatal("nil runtimeSnapshotConfig resolved a worker")
	}
	if _, ok := nilConfig.Workstation("workstation-a"); ok {
		t.Fatal("nil runtimeSnapshotConfig resolved a workstation")
	}
}

func assertValidStatesClone(t *testing.T, validStates map[string]map[string]bool) {
	t.Helper()
	clonedStates := cloneValidStates(validStates)
	clonedStates["task"]["ready"] = false
	if validStates["task"]["ready"] != true {
		t.Fatal("cloneValidStates() shared nested state")
	}
	if cloneValidStates(nil) != nil {
		t.Fatal("cloneValidStates(nil) returned a non-nil map")
	}
}

func assertRuntimeFilesystemConfig(t *testing.T, validStates map[string]map[string]bool) {
	t.Helper()
	filesystem := runtimeFilesystemConfig(
		"/factories/example",
		automations.RuntimeFilesystemInputs{
			KnownWorkTypes:    []string{"task"},
			ValidStatesByType: validStates,
		},
		zap.NewNop(),
		func(context.Context, work.WorkRequest) error { return nil },
	)
	if filesystem.Dir != filepath.Join("/factories/example", factorydefinitions.InputsDir) || len(filesystem.KnownWorkTypes) != 1 {
		t.Fatalf("runtimeFilesystemConfig() = %#v", filesystem)
	}
}

func TestRuntimeLifecycle_RejectsMalformedActivationRequests(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*automations.RuntimeActivationRequest)
	}{
		{name: "missing runtime ID", mutate: func(request *automations.RuntimeActivationRequest) { request.RuntimeID = "" }},
		{name: "session mismatch", mutate: func(request *automations.RuntimeActivationRequest) { request.FactorySessionID = "other" }},
		{name: "missing factory directory", mutate: func(request *automations.RuntimeActivationRequest) { request.Snapshot.FactoryDir = "" }},
		{name: "missing factory name", mutate: func(request *automations.RuntimeActivationRequest) { request.Snapshot.EffectiveFactory.Name = "" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := runtimeActivationRequestForTest("runtime-invalid", "example")
			test.mutate(&request)
			if _, err := normalizeRuntimeActivationRequest(request); err == nil {
				t.Fatal("normalizeRuntimeActivationRequest() error = nil, want invalid request")
			}
		})
	}
}

func TestRuntimeLifecycle_OpaqueIdentityUsesPresenceAndType(t *testing.T) {
	var nilPointer *int
	if got := newOpaqueActivationIdentity(nil); got.present {
		t.Fatal("nil interface was marked present")
	}
	if got := newOpaqueActivationIdentity(nilPointer); got.present {
		t.Fatal("nil pointer was marked present")
	}
	first := newOpaqueActivationIdentity(func() {})
	second := newOpaqueActivationIdentity(func() {})
	if !first.matches(second) {
		t.Fatalf("equivalent function identities did not match: %#v, %#v", first, second)
	}
	if newOpaqueActivationIdentity("value").matches(newOpaqueActivationIdentity(42)) {
		t.Fatal("different opaque value types unexpectedly matched")
	}
	if runtimeLifecycleSentinel(automations.ErrorCodeInvalid) != automations.ErrInvalidRequest ||
		runtimeLifecycleSentinel(automations.ErrorCodeNotFound) != automations.ErrNotFound ||
		runtimeLifecycleSentinel(automations.ErrorCodeConflict) != automations.ErrConflict ||
		runtimeLifecycleSentinel(automations.ErrorCodeFailed) != automations.ErrSupervisionFailed ||
		runtimeLifecycleSentinel(automations.ErrorCode("unknown")) != nil {
		t.Fatal("runtimeLifecycleSentinel() returned an unexpected sentinel")
	}
}

func TestRuntimeLifecycle_CleansPendingAndMatchingRegistryEntries(t *testing.T) {
	service := New(zap.NewNop(), nil, &internalScriptPollerRunner{}, "", "", nil, nil, nil, cronwire.NewService(), fswire.NewService())
	service.runtimeActivating["runtime-pending"] = struct{}{}
	service.clearRuntimeActivation("runtime-pending")
	if _, ok := service.runtimeActivating["runtime-pending"]; ok {
		t.Fatal("clearRuntimeActivation() retained the pending activation")
	}

	instance := &runtimeInstance{}
	service.runtimes["runtime-active"] = instance
	service.removeRuntime("runtime-active", &runtimeInstance{})
	if _, ok := service.runtimes["runtime-active"]; !ok {
		t.Fatal("removeRuntime() removed a non-matching instance")
	}
	service.removeRuntime("runtime-active", instance)
	if _, ok := service.runtimes["runtime-active"]; ok {
		t.Fatal("removeRuntime() retained the matching instance")
	}
}

func TestRuntimeLifecycle_ClonesSnapshotConfigCollections(t *testing.T) {
	snapshot := factorydefinitions.RuntimeSnapshot{
		FactoryDir:     "/factories/example",
		RuntimeBaseDir: "/runtime/example",
		EffectiveFactory: factorydefinitions.FactoryConfig{
			Name:         "example",
			Workers:      []factorydefinitions.FactoryWorkerConfig{{Name: "effective-worker"}},
			Workstations: []factorydefinitions.FactoryWorkstationConfig{{Name: "effective-workstation"}},
		},
		Workers:      []factorydefinitions.FactoryWorkerConfig{{Name: "runtime-worker"}},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{{Name: "runtime-workstation"}},
	}
	config, err := newRuntimeSnapshotConfig("runtime-example", snapshot)
	if err != nil {
		t.Fatalf("newRuntimeSnapshotConfig() error = %v", err)
	}
	if config.FactoryDir() != snapshot.FactoryDir || config.RuntimeBaseDir() != snapshot.RuntimeBaseDir || config.RuntimeInstanceID() != "runtime-example" {
		t.Fatalf("newRuntimeSnapshotConfig() identity = %q, %q, %q", config.FactoryDir(), config.RuntimeBaseDir(), config.RuntimeInstanceID())
	}
	if got, ok := config.Worker("runtime-worker"); !ok || got == nil || got.Name != "runtime-worker" {
		t.Fatalf("runtime worker = %#v, %v", got, ok)
	}
	if got, ok := config.Workstation("runtime-workstation"); !ok || got == nil || got.Name != "runtime-workstation" {
		t.Fatalf("runtime workstation = %#v, %v", got, ok)
	}
	config.config.Workers[0].Name = "mutated"
	config.config.Workstations[0].Name = "mutated"
	if snapshot.Workers[0].Name != "runtime-worker" || snapshot.Workstations[0].Name != "runtime-workstation" {
		t.Fatal("newRuntimeSnapshotConfig() shared collection values")
	}
}

func TestNewFilesystemWatcherUsesServiceOwnerAndHandlesNilOwner(t *testing.T) {
	service := New(zap.NewNop(), nil, &internalScriptPollerRunner{}, "workflow", "", nil, nil, nil, cronwire.NewService(), fswire.NewService())
	watcher := service.NewFilesystemWatcher(automations.FilesystemWatcherConfig{
		Dir:            t.TempDir(),
		Files:          watcherInputFilesystem{},
		WalkDirectory:  filepath.WalkDir,
		WorkRequestIDs: func() string { return "watcher-test" },
		Submitter:      func(context.Context, work.WorkRequest) error { return nil },
	})
	if watcher == nil {
		t.Fatal("NewFilesystemWatcher() returned nil for a constructed service")
	}

	var nilService *Service
	if nilService.NewFilesystemWatcher(automations.FilesystemWatcherConfig{}) != nil {
		t.Fatal("NewFilesystemWatcher() returned a watcher for a nil service")
	}
}

func TestNilServiceRootHasNoPublishedCapabilities(t *testing.T) {
	var nilService *Service
	if root := nilService.Root(); root.Operations != nil || root.Lifecycle != nil || root.Runtime != nil {
		t.Fatalf("nil service Root() = %#v, want empty root", root)
	}
}

func TestCloneStringMapDetachesOptionalTags(t *testing.T) {
	if cloneStringMap(nil) != nil {
		t.Fatal("cloneStringMap(nil) returned a non-nil map")
	}
	original := map[string]string{"key": "value"}
	cloned := cloneStringMap(original)
	cloned["key"] = "changed"
	if original["key"] != "value" {
		t.Fatal("cloneStringMap() shared its backing map")
	}
}

type watcherInputFilesystem struct{}

func (watcherInputFilesystem) ReadDir(name string) ([]fs.DirEntry, error) { return os.ReadDir(name) }
func (watcherInputFilesystem) ReadFile(name string) ([]byte, error)       { return os.ReadFile(name) }
func (watcherInputFilesystem) Stat(name string) (fs.FileInfo, error)      { return os.Stat(name) }

func runtimeActivationRequestForTest(runtimeID, factoryName string) automations.RuntimeActivationRequest {
	return automations.RuntimeActivationRequest{
		RuntimeID:        runtimeID,
		FactorySessionID: "session-1",
		Snapshot: factorydefinitions.RuntimeSnapshot{
			FactoryDir:       "/factories/" + factoryName,
			RuntimeBaseDir:   "/factories/" + factoryName,
			Invocation:       factorydefinitions.RuntimeSnapshotInvocationContext{FactorySessionID: "session-1"},
			EffectiveFactory: factorydefinitions.FactoryConfig{Name: factoryName},
		},
	}
}

// These characterize owner-local lifecycle behavior before the per-runtime graph
// is removed. Commands, admission and scheduling are controlled; only the owned
// cursor persistence uses a test-local filesystem.
func TestRuntimeLifecycle_ReactivationStopsPriorAdmissionAndResumesCursor(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	clock := clockwork.NewFakeClock()
	runner := &lifecycleCursorRunner{calls: make(chan lifecycleCursorCall, 8)}
	service := NewWithCursorFileSystem(zap.NewNop(), clock, runner, "", "", nil, nil,
		factorydefinitioncomposition.WorkstationExecutionPolicy{}, platformfilesystem.Local{},
		cronwire.NewService(),
		fswire.NewService(),
	)
	aAdmissions, bAdmissions := make(chan work.WorkRequest, 8), make(chan work.WorkRequest, 8)
	a := cursorRuntimeRequest(t, "A", "workflow-A", aAdmissions)
	b := cursorRuntimeRequest(t, "B", "workflow-B", bAdmissions)
	activateCursorRuntime(t, ctx, service, a)
	activateCursorRuntime(t, ctx, service, b)
	for range 2 {
		call := awaitLifecycleCursorCall(t, ctx, runner)
		if slices.Contains(call.request.Env, scriptpollers.ScriptPollerCursorEnvVar+"=cursor-A") ||
			slices.Contains(call.request.Env, scriptpollers.ScriptPollerCursorEnvVar+"=cursor-B") {
			t.Fatal("fresh command unexpectedly resumed a prior cursor")
		}
		id := "B"
		if call.request.WorkDir == a.Snapshot.FactoryDir {
			id = "A"
		} else if call.request.WorkDir != b.Snapshot.FactoryDir {
			t.Fatalf("unexpected command directory %q", call.request.WorkDir)
		}
		call.complete <- cursorPollerOutput(id)
	}
	awaitCursorAdmission(t, ctx, aAdmissions, "request-A")
	awaitCursorAdmission(t, ctx, bAdmissions, "request-B")
	if err := clock.BlockUntilContext(ctx, 2); err != nil {
		t.Fatalf("wait for committed polls/backoff: %v", err)
	}
	assertLifecycleCursor(t, ctx, service, "workflow-A", "A")
	stopCursorRuntime(t, ctx, service, a.RuntimeID)
	clock.Advance(time.Second)
	bCall := awaitLifecycleCursorCall(t, ctx, runner)
	assertCursorCommand(t, bCall.request, b.Snapshot.FactoryDir, "B")
	bCall.complete <- cursorPollerOutput("B")
	awaitCursorAdmission(t, ctx, bAdmissions, "request-B")
	if err := clock.BlockUntilContext(ctx, 1); err != nil {
		t.Fatalf("wait for peer commit: %v", err)
	}
	select {
	case admission := <-aAdmissions:
		t.Fatalf("A admitted while stopped: %+v", admission)
	default:
	}
	select {
	case call := <-runner.calls:
		t.Fatalf("unexpected command while A stopped and B waiting: %+v", call.request)
	default:
	}
	activateCursorRuntime(t, ctx, service, a)
	assertLifecycleCursor(t, ctx, service, "workflow-A", "A")
	aCall := awaitLifecycleCursorCall(t, ctx, runner)
	assertCursorCommand(t, aCall.request, a.Snapshot.FactoryDir, "A")
	aCall.complete <- cursorPollerOutput("A")
	awaitCursorAdmission(t, ctx, aAdmissions, "request-A")
	stopCursorRuntime(t, ctx, service, a.RuntimeID)
	stopCursorRuntime(t, ctx, service, b.RuntimeID)
}

type lifecycleCursorCall struct {
	request  platformprocess.CommandRequest
	complete chan []byte
}

type lifecycleCursorRunner struct {
	calls chan lifecycleCursorCall
}

func (r *lifecycleCursorRunner) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	call := lifecycleCursorCall{request: request, complete: make(chan []byte, 1)}
	select {
	case r.calls <- call:
	case <-ctx.Done():
		return platformprocess.CommandResult{}, ctx.Err()
	}
	select {
	case stdout := <-call.complete:
		return platformprocess.CommandResult{Stdout: stdout}, nil
	case <-ctx.Done():
		return platformprocess.CommandResult{}, ctx.Err()
	}
}

func cursorPollerOutput(id string) []byte {
	output, _ := json.Marshal(map[string]any{
		"requestId": "request-" + id, "type": "FACTORY_REQUEST_BATCH",
		"works":  []map[string]string{{"name": "work-" + id, "workTypeName": "task"}},
		"cursor": "cursor-" + id, "checkpoint": "checkpoint-" + id,
	})
	return output
}

func cursorRuntimeRequest(t *testing.T, id, workflowID string, admissions chan work.WorkRequest) automations.RuntimeActivationRequest {
	t.Helper()
	request := runtimeActivationRequestForTest("runtime-"+id, id)
	request.FactorySessionID = "session-" + id
	request.Snapshot.Invocation.FactorySessionID = request.FactorySessionID
	request.Snapshot.Invocation.WorkflowID = workflowID
	request.Snapshot.FactoryDir = t.TempDir()
	request.Snapshot.RuntimeBaseDir = request.Snapshot.FactoryDir
	request.Snapshot.EffectiveFactory.Workers = []factorydefinitions.FactoryWorkerConfig{*internalCanonicalScriptPollerWorker()}
	request.Snapshot.EffectiveFactory.Workstations = []factorydefinitions.FactoryWorkstationConfig{internalCanonicalScriptPollerWorkstation()}
	request.Inputs.StartSchedulers = true
	request.Inputs.Submitter = func(ctx context.Context, request work.WorkRequest) error {
		select {
		case admissions <- request:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return request
}

func activateCursorRuntime(t *testing.T, ctx context.Context, service *Service, request automations.RuntimeActivationRequest) {
	t.Helper()
	if _, err := service.ActivateRuntime(ctx, request); err != nil {
		t.Fatalf("ActivateRuntime(%s): %v", request.RuntimeID, err)
	}
	t.Cleanup(func() { stopCursorRuntime(t, context.Background(), service, request.RuntimeID) })
	if err := service.StartRuntime(ctx, request.RuntimeID); err != nil {
		t.Fatalf("StartRuntime(%s): %v", request.RuntimeID, err)
	}
}

func stopCursorRuntime(t *testing.T, ctx context.Context, service *Service, id string) {
	t.Helper()
	result, err := service.DeactivateRuntime(ctx, automations.RuntimeDeactivationRequest{RuntimeID: id})
	if err != nil || result.State != automations.RuntimeLifecycleStopped {
		t.Fatalf("DeactivateRuntime(%s): %+v, %v", id, result, err)
	}
}

func awaitLifecycleCursorCall(t *testing.T, ctx context.Context, runner *lifecycleCursorRunner) lifecycleCursorCall {
	t.Helper()
	select {
	case call := <-runner.calls:
		return call
	case <-ctx.Done():
		t.Fatalf("wait for command: %v", ctx.Err())
		return lifecycleCursorCall{}
	}
}

func awaitCursorAdmission(t *testing.T, ctx context.Context, admissions <-chan work.WorkRequest, id string) {
	t.Helper()
	select {
	case request := <-admissions:
		if request.RequestID != id || request.Type != work.WorkRequestTypeFactoryRequestBatch {
			t.Fatalf("admission = %+v, want batch %s", request, id)
		}
	case <-ctx.Done():
		t.Fatalf("wait for admission %s: %v", id, ctx.Err())
	}
}

func assertCursorCommand(t *testing.T, request platformprocess.CommandRequest, dir, id string) {
	t.Helper()
	if request.WorkDir != dir || !slices.Contains(request.Env, scriptpollers.ScriptPollerCursorEnvVar+"=cursor-"+id) ||
		!slices.Contains(request.Env, scriptpollers.ScriptPollerCheckpointEnvVar+"=checkpoint-"+id) {
		t.Fatalf("resumed command = %+v, want directory %s and %s recovery facts", request, dir, id)
	}
}

func assertLifecycleCursor(t *testing.T, ctx context.Context, service *Service, workflowID, id string) {
	t.Helper()
	instanceID := scriptpollers.SupervisionFor(workflowID, internalCanonicalScriptPollerWorkstation().Name).InstanceID
	got, err := service.Root().GetCursor(ctx, automations.GetCursorRequest{InstanceID: instanceID})
	if err != nil || got.AutomationID != workflowID || got.Cursor != automations.Cursor("cursor-"+id) || got.Checkpoint != "checkpoint-"+id {
		t.Fatalf("GetCursor(%s) = %+v, %v", id, got, err)
	}
}

func TestGetCursorWithTwoRuntimesSharingPollerInstanceID(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	files := &cursorReadFaultFileSystem{faults: make(map[string]error)}
	service := NewWithCursorFileSystem(zap.NewNop(), clockwork.NewFakeClock(), &internalScriptPollerRunner{}, "shared-workflow", "", nil, nil,
		factorydefinitioncomposition.WorkstationExecutionPolicy{}, files,
		cronwire.NewService(),
		fswire.NewService(),
	)
	a := cursorRuntimeRequest(t, "shared-A", "shared-workflow", make(chan work.WorkRequest, 1))
	b := cursorRuntimeRequest(t, "shared-B", "shared-workflow", make(chan work.WorkRequest, 1))
	for _, request := range []automations.RuntimeActivationRequest{a, b} {
		if _, err := service.ActivateRuntime(ctx, request); err != nil {
			t.Fatalf("ActivateRuntime: %v", err)
		}
		t.Cleanup(func() { stopCursorRuntime(t, ctx, service, request.RuntimeID) })
	}
	seedLifecycleCursor(t, service.runtimes[a.RuntimeID].owner, a.Snapshot.FactoryDir, "A")
	seedLifecycleCursor(t, service.runtimes[b.RuntimeID].owner, b.Snapshot.FactoryDir, "B")
	instanceID := scriptpollers.SupervisionFor("shared-workflow", internalCanonicalScriptPollerWorkstation().Name).InstanceID
	assertSharedCursorSelection(t, service, instanceID)
	assertSharedCursorReadFailures(t, service, instanceID, files, a.Snapshot.FactoryDir, b.Snapshot.FactoryDir)
}

func assertSharedCursorSelection(t *testing.T, service *Service, instanceID string) {
	t.Helper()
	ctx := context.Background()
	request := automations.GetCursorRequest{InstanceID: instanceID}
	got, err := service.Root().GetCursor(ctx, request)
	if err != nil || got.AutomationID != "shared-workflow" || got.InstanceID != instanceID ||
		!((got.Cursor == "cursor-A" && got.Checkpoint == "checkpoint-A") || (got.Cursor == "cursor-B" && got.Checkpoint == "checkpoint-B")) {
		t.Fatalf("unscoped read = %+v, %v, want complete A or B facts", got, err)
	}
	// Map order is intentionally unspecified. A conflicting first candidate
	// terminates the search even if a later candidate matches ExpectedCursor.
	request.ExpectedCursor = "cursor-A"
	got, err = service.Root().GetCursor(ctx, request)
	if err == nil {
		if got.Cursor != "cursor-A" || got.Checkpoint != "checkpoint-A" {
			t.Fatalf("matching read = %+v, want A facts", got)
		}
	} else {
		assertCursorReadError(t, err, automations.ErrorCodeConflict, automations.ErrConflict)
	}
	request.ExpectedCursor = "stale"
	_, err = service.Root().GetCursor(ctx, request)
	assertCursorReadError(t, err, automations.ErrorCodeConflict, automations.ErrConflict)
}

func assertSharedCursorReadFailures(t *testing.T, service *Service, instanceID string, files *cursorReadFaultFileSystem, aDir, bDir string) {
	t.Helper()
	ctx := context.Background()
	request := automations.GetCursorRequest{InstanceID: instanceID}
	// A missing candidate is skipped. Other failures stop the lookup. Force
	// these permutations at the cursor IO boundary without relying on map order.
	files.faults[aDir] = fs.ErrNotExist
	got, err := service.Root().GetCursor(ctx, request)
	if err != nil || got.Cursor != "cursor-B" || got.Checkpoint != "checkpoint-B" {
		t.Fatalf("missing A read = %+v, %v, want B facts", got, err)
	}
	readErr := errors.New("cursor read unavailable")
	files.faults[aDir] = readErr
	got, err = service.Root().GetCursor(ctx, request)
	if err != nil {
		assertCursorReadError(t, err, automations.ErrorCodeFailed, readErr)
	} else if got.Cursor != "cursor-B" || got.Checkpoint != "checkpoint-B" {
		t.Fatalf("failed A/live B read = %+v, want B facts or first-candidate failure", got)
	}
	files.faults[aDir] = fs.ErrNotExist
	files.faults[bDir] = readErr
	_, err = service.Root().GetCursor(ctx, request)
	assertCursorReadError(t, err, automations.ErrorCodeFailed, readErr)
	files.faults[bDir] = fs.ErrNotExist
	_, err = service.Root().GetCursor(ctx, request)
	assertCursorReadError(t, err, automations.ErrorCodeNotFound, automations.ErrNotFound)
}

type cursorReadFaultFileSystem struct {
	platformfilesystem.Local
	faults map[string]error
	reads  int
}

func (f *cursorReadFaultFileSystem) ReadFile(path string) ([]byte, error) {
	f.reads++
	for dir, err := range f.faults {
		rel, relErr := filepath.Rel(dir, path)
		if relErr == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return nil, err
		}
	}
	return f.Local.ReadFile(path)
}

func assertCursorReadError(t *testing.T, err error, code automations.ErrorCode, sentinel error) {
	t.Helper()
	var typed *automations.Error
	if !errors.As(err, &typed) || typed.Op != scriptpollers.GetCursorOperation || typed.Code != code || !errors.Is(err, sentinel) {
		t.Fatalf("cursor error = %v, want %s/%s wrapping %v", err, scriptpollers.GetCursorOperation, code, sentinel)
	}
}

func seedLifecycleCursor(t *testing.T, service *Service, dir, id string) {
	t.Helper()
	poller, worker := internalCanonicalScriptPollerWorkstation(), internalCanonicalScriptPollerWorker()
	runner := &internalScriptPollerRunner{outcomes: []internalScriptPollerOutcome{{result: platformprocess.CommandResult{Stdout: cursorPollerOutput(id)}}}}
	err := service.RunScriptPoller(context.Background(), runner, internalScriptPollerLoadedRuntimeConfig(t, dir, poller, worker), poller, worker,
		func(context.Context, work.WorkRequest) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "exited unexpectedly") {
		t.Fatalf("seed poll error = %v, want terminal exit after successful commit", err)
	}
}

func TestNewRootWithoutFactoryDirKeepsCursorInMemory(t *testing.T) {
	t.Parallel()
	files := &cursorReadFaultFileSystem{faults: map[string]error{".": errors.New("unexpected cursor filesystem use")}}
	service := NewWithCursorFileSystem(zap.NewNop(), clockwork.NewFakeClock(), &internalScriptPollerRunner{}, "memory-workflow", "", nil, nil,
		factorydefinitioncomposition.WorkstationExecutionPolicy{}, files,
		cronwire.NewService(),
		fswire.NewService(),
	)
	seedLifecycleCursor(t, service, t.TempDir(), "memory")
	assertLifecycleCursor(t, context.Background(), service, "memory-workflow", "memory")
	if files.reads != 0 {
		t.Fatalf("blank-base cursor filesystem reads = %d, want memory only", files.reads)
	}
	second := NewWithCursorFileSystem(zap.NewNop(), clockwork.NewFakeClock(), &internalScriptPollerRunner{}, "memory-workflow", "", nil, nil,
		factorydefinitioncomposition.WorkstationExecutionPolicy{}, files,
		cronwire.NewService(),
		fswire.NewService(),
	)
	instanceID := scriptpollers.SupervisionFor("memory-workflow", internalCanonicalScriptPollerWorkstation().Name).InstanceID
	_, err := second.Root().GetCursor(context.Background(), automations.GetCursorRequest{InstanceID: instanceID})
	assertCursorReadError(t, err, automations.ErrorCodeNotFound, automations.ErrNotFound)
	if files.reads != 0 {
		t.Fatal("reconstructed blank-base owner consulted durable storage")
	}
}

// The injected services are reused across activations, while each watcher and
// Work admission destination still belongs to its configured runtime.
func TestRuntimeLifecycle_UsesInjectedCronAndWatcherOperations(t *testing.T) {
	t.Parallel()
	clock := clockwork.NewFakeClock()
	crons := &runtimeCronRecorder{}
	watchers := &runtimeWatcherRecorder{}
	service := New(zap.NewNop(), clock, &internalScriptPollerRunner{}, "", "", nil, nil, nil, crons, watchers)
	if crons.calls != 0 || len(watchers.roots) != 0 {
		t.Fatal("construction executed source operations")
	}
	for _, name := range []string{"alpha", "beta"} {
		request := injectedSourceActivation(name)
		if _, err := service.ActivateRuntime(context.Background(), request); err != nil {
			t.Fatalf("ActivateRuntime(%s): %v", name, err)
		}
		t.Cleanup(func() {
			_, err := service.DeactivateRuntime(context.Background(), automations.RuntimeDeactivationRequest{RuntimeID: name})
			if err != nil {
				t.Errorf("DeactivateRuntime(%s): %v", name, err)
			}
		})
		var observed string
		submit := func(_ context.Context, request work.WorkRequest) error { observed = request.RequestID; return nil }
		if err := service.runtimes[name].owner.SubmitCronTick(context.Background(), nil, name, submit, factorydefinitions.FactoryWorkstationConfig{Name: "clock"}, clock.Now()); err != nil {
			t.Fatalf("SubmitCronTick(%s): %v", name, err)
		}
		if observed != name {
			t.Fatalf("runtime %s admitted request %q", name, observed)
		}
	}
	expectedRoots := []string{filepath.Join("/factories/alpha", factorydefinitions.InputsDir), filepath.Join("/factories/beta", factorydefinitions.InputsDir)}
	if crons.calls != 2 || !slices.Equal(watchers.roots, expectedRoots) || watchers.preseeds != 2 {
		t.Fatalf("injected operations: cron=%d roots=%v preseeds=%d", crons.calls, watchers.roots, watchers.preseeds)
	}
}

func TestRuntimeLifecycle_InjectedWatcherFailurePreservesPeer(t *testing.T) {
	t.Parallel()
	failure := errors.New("controlled preseed failure")
	watchers := &runtimeWatcherRecorder{failure: failure}
	service := New(zap.NewNop(), clockwork.NewFakeClock(), &internalScriptPollerRunner{}, "", "", nil, nil, nil, &runtimeCronRecorder{}, watchers)
	peer := runtimeActivationRequestForTest("peer", "peer")
	if _, err := service.ActivateRuntime(context.Background(), peer); err != nil {
		t.Fatal(err)
	}
	_, err := service.ActivateRuntime(context.Background(), injectedSourceActivation("broken"))
	if !errors.Is(err, automations.ErrSupervisionFailed) || !strings.Contains(err.Error(), failure.Error()) {
		t.Fatalf("ActivateRuntime error = %v, want classified injected preseed failure", err)
	}
	if err := service.Root().StartRuntime(context.Background(), peer.RuntimeID); err != nil {
		t.Fatalf("peer StartRuntime: %v", err)
	}
	if _, err := service.DeactivateRuntime(context.Background(), automations.RuntimeDeactivationRequest{RuntimeID: peer.RuntimeID}); err != nil {
		t.Fatal(err)
	}
	if err := service.Root().StartRuntime(context.Background(), "broken"); !errors.Is(err, automations.ErrNotFound) {
		t.Fatalf("failed activation retained a startable runtime: %v", err)
	}
}

func injectedSourceActivation(name string) automations.RuntimeActivationRequest {
	request := runtimeActivationRequestForTest(name, name)
	request.Inputs.Submitter = func(context.Context, work.WorkRequest) error { return nil }
	request.Inputs.Filesystem = automations.RuntimeFilesystemInputs{
		Files: watcherInputFilesystem{}, WalkDirectory: filepath.WalkDir,
		WorkRequestIDs: func() string { return name },
	}
	return request
}

type runtimeCronRecorder struct {
	cron.Service
	calls int
}

func (r *runtimeCronRecorder) SubmitCronTick(ctx context.Context, submit cron.WorkRequestSubmitter, workflow string, _ factorydefinitions.FactoryWorkstationConfig, _ time.Time) (cron.CronTickSubmission, error) {
	r.calls++
	err := submit(ctx, work.WorkRequest{RequestID: workflow})
	return cron.CronTickSubmission{Submitted: err == nil}, err
}

type runtimeWatcherRecorder struct {
	filesystemwatchers.Service
	roots    []string
	preseeds int
	failure  error
}

func (r *runtimeWatcherRecorder) NewWatcher(config filesystemwatchers.Config) filesystemwatchers.Watcher {
	r.roots = append(r.roots, config.Dir)
	return runtimeInjectedWatcher{owner: r}
}

type runtimeInjectedWatcher struct{ owner *runtimeWatcherRecorder }

func (w runtimeInjectedWatcher) PreseedInputs(context.Context) error {
	w.owner.preseeds++
	return w.owner.failure
}
func (runtimeInjectedWatcher) Watch(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }
