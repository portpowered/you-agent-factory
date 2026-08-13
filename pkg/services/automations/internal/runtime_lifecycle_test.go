package internal

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

func TestRuntimeLifecycle_IsolatesOwnersAndClassifiesDuplicates(t *testing.T) {
	service := New(zap.NewNop(), nil, nil, "", "", nil, nil, nil)
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
	service := New(zap.NewNop(), nil, nil, "", "", nil, nil, nil)
	_, err := service.Root().ActivateRuntime(context.Background(), automations.RuntimeActivationRequest{})
	var typed *automations.Error
	if !errors.As(err, &typed) || typed.Code != automations.ErrorCodeInvalid {
		t.Fatalf("ActivateRuntime(empty) error = %v, want typed invalid error", err)
	}
}

func TestRuntimeLifecycle_RejectsBehavioralInputConflictsWithSameSnapshot(t *testing.T) {
	service := New(zap.NewNop(), nil, nil, "", "", nil, nil, nil)
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
	service := New(zap.NewNop(), nil, nil, "", "", nil, nil, nil)
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
	service := New(zap.NewNop(), nil, nil, "", "", nil, nil, nil)
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
	service := New(zap.NewNop(), nil, nil, "", "", nil, nil, nil)
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
	service := New(zap.NewNop(), nil, nil, "workflow", "", nil, nil, nil)
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

func TestServiceDefaultEdgesRemainSafeForNilAndMissingCommandRunner(t *testing.T) {
	var nilService *Service
	if root := nilService.Root(); root.Operations != nil || root.Lifecycle != nil || root.Runtime != nil {
		t.Fatalf("nil service Root() = %#v, want empty root", root)
	}
	if nilService.logger() == nil || nilService.commandRunner() == nil || nilService.supervisorClock() == nil {
		t.Fatal("nil service default collaborators were not supplied")
	}
	if _, err := (unavailableCommandRunner{}).Run(context.Background(), workers.CommandRequest{}); err == nil || err.Error() != "automation command runner is required" {
		t.Fatalf("unavailable command runner error = %v", err)
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
