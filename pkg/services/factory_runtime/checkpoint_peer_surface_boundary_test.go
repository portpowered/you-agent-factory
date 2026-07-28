package factory_test

import (
	"context"
	"errors"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

const factoryRuntimeRootPackage = "github.com/portpowered/infinite-you/pkg/services/factory_runtime"

var checkpointRootContractTypes = []reflect.Type{
	reflect.TypeOf(factory.CheckpointOutcome("")),
	reflect.TypeOf(factory.Checkpoint{}),
	reflect.TypeOf(factory.CaptureCheckpointRequest{}),
	reflect.TypeOf(factory.CaptureCheckpointResult{}),
	reflect.TypeOf(factory.LoadCheckpointRequest{}),
	reflect.TypeOf(factory.LoadCheckpointResult{}),
	reflect.TypeOf(factory.RestoreCheckpointRequest{}),
	reflect.TypeOf(factory.RestoreCheckpointResult{}),
}

var forbiddenCheckpointPeerImportPrefixes = []string{
	factoryRuntimeRootPackage + "/internal/services/checkpoint_recovery",
	factoryRuntimeRootPackage + "/checkpointstore",
}

var checkpointPeerConsumerPackages = []string{
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions",
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/...",
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/live_runtime/...",
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution/...",
	"github.com/portpowered/infinite-you/pkg/services/automations",
	"github.com/portpowered/infinite-you/pkg/services/recordings",
}

// peerCheckpointRootConsumer depends only on the published Factory Runtime root
// checkpoint vocabulary to recover execution state without CheckpointStore ports
// or Petri/JavaScript checkpoint strategy record types.
type peerCheckpointRootConsumer struct {
	runtime factory.Service
}

func (c peerCheckpointRootConsumer) recoverOpaqueCheckpoint(
	ctx context.Context,
	checkpointID string,
) (factory.Checkpoint, error) {
	captured, err := c.runtime.CaptureCheckpoint(ctx, factory.CaptureCheckpointRequest{CheckpointID: checkpointID})
	if err != nil {
		return factory.Checkpoint{}, err
	}
	if captured.Outcome != factory.CheckpointOutcomeCaptured {
		return factory.Checkpoint{}, factory.ErrCorruptCheckpoint
	}
	loaded, err := c.runtime.LoadCheckpoint(ctx, factory.LoadCheckpointRequest{
		CheckpointID:          checkpointID,
		ExpectedSchemaVersion: captured.Checkpoint.SchemaVersion,
	})
	if err != nil {
		return factory.Checkpoint{}, err
	}
	if loaded.Outcome != factory.CheckpointOutcomeLoaded || !loaded.Compatible {
		return factory.Checkpoint{}, factory.ErrIncompatibleCheckpoint
	}
	restored, err := c.runtime.RestoreCheckpoint(ctx, factory.RestoreCheckpointRequest{Checkpoint: loaded.Checkpoint})
	if err != nil {
		return factory.Checkpoint{}, err
	}
	if restored.Outcome != factory.CheckpointOutcomeRestored || restored.CheckpointID != checkpointID {
		return factory.Checkpoint{}, factory.ErrCorruptCheckpoint
	}
	return loaded.Checkpoint, nil
}

type checkpointRootPeerFake struct {
	captured factory.Checkpoint
}

func (fake *checkpointRootPeerFake) CaptureCheckpoint(
	_ context.Context,
	req factory.CaptureCheckpointRequest,
) (factory.CaptureCheckpointResult, error) {
	fake.captured = factory.Checkpoint{
		CheckpointID:  req.CheckpointID,
		SchemaVersion: 1,
		StrategyKind:  "runtime",
		Payload:       []byte(`{"factoryState":"PAUSED"}`),
	}
	return factory.CaptureCheckpointResult{
		Outcome:    factory.CheckpointOutcomeCaptured,
		Checkpoint: fake.captured,
	}, nil
}

func (fake *checkpointRootPeerFake) LoadCheckpoint(
	_ context.Context,
	req factory.LoadCheckpointRequest,
) (factory.LoadCheckpointResult, error) {
	if req.CheckpointID == "" || fake.captured.CheckpointID == "" {
		return factory.LoadCheckpointResult{}, factory.ErrCheckpointNotFound
	}
	if req.CheckpointID != fake.captured.CheckpointID {
		return factory.LoadCheckpointResult{}, factory.ErrCheckpointNotFound
	}
	compatible := req.ExpectedSchemaVersion == 0 ||
		req.ExpectedSchemaVersion == fake.captured.SchemaVersion
	return factory.LoadCheckpointResult{
		Outcome:    factory.CheckpointOutcomeLoaded,
		Checkpoint: fake.captured,
		Compatible: compatible,
	}, nil
}

func (fake *checkpointRootPeerFake) RestoreCheckpoint(
	_ context.Context,
	req factory.RestoreCheckpointRequest,
) (factory.RestoreCheckpointResult, error) {
	if req.Checkpoint.CheckpointID == "" || len(req.Checkpoint.Payload) == 0 {
		return factory.RestoreCheckpointResult{}, factory.ErrCorruptCheckpoint
	}
	if req.Checkpoint.SchemaVersion != fake.captured.SchemaVersion {
		return factory.RestoreCheckpointResult{}, factory.ErrIncompatibleCheckpoint
	}
	return factory.RestoreCheckpointResult{
		Outcome:      factory.CheckpointOutcomeRestored,
		CheckpointID: req.Checkpoint.CheckpointID,
	}, nil
}

func (fake *checkpointRootPeerFake) ControlPause(context.Context, factory.PauseRequest) (factory.PauseResult, error) {
	return factory.PauseResult{Outcome: factory.ControlOutcomeAccepted}, nil
}
func (fake *checkpointRootPeerFake) ControlResume(context.Context, factory.ResumeRequest) (factory.ResumeResult, error) {
	return factory.ResumeResult{Outcome: factory.ControlOutcomeAccepted}, nil
}
func (fake *checkpointRootPeerFake) ControlTerminate(context.Context, factory.TerminateRequest) (factory.TerminateResult, error) {
	return factory.TerminateResult{Outcome: factory.ControlOutcomeAccepted}, nil
}
func (fake *checkpointRootPeerFake) ControlWaitToComplete(factory.WaitToCompleteRequest) factory.WaitToCompleteResult {
	done := make(chan struct{})
	close(done)
	return factory.WaitToCompleteResult{Done: done}
}
func (fake *checkpointRootPeerFake) ControlMoveWork(context.Context, factory.MoveWorkRequest) (factory.MoveWorkResult, error) {
	return factory.MoveWorkResult{}, nil
}
func (fake *checkpointRootPeerFake) Observe(context.Context, factory.ObserveRequest) (factory.ObserveResult, error) {
	return factory.ObserveResult{}, nil
}
func (fake *checkpointRootPeerFake) PlanDispatch(context.Context, factory.PlanDispatchRequest) (factory.PlanDispatchResult, error) {
	return factory.PlanDispatchResult{}, nil
}
func (fake *checkpointRootPeerFake) AcceptDispatchResult(context.Context, factory.AcceptDispatchResultRequest) (factory.AcceptDispatchResultResult, error) {
	return factory.AcceptDispatchResultResult{}, nil
}

func TestCheckpointRootContracts_DeclareOnlyRuntimeRootVocabulary(t *testing.T) {
	t.Parallel()

	for _, typ := range checkpointRootContractTypes {
		typ := typ
		t.Run(typ.String(), func(t *testing.T) {
			t.Parallel()
			assertCheckpointRootContractUsesOnlyRuntimeRootVocabulary(t, typ, map[reflect.Type]bool{})
		})
	}
}

func TestCheckpointPeerConsumers_DoNotImportCheckpointRecoveryInternals(t *testing.T) {
	t.Parallel()

	for _, root := range checkpointPeerConsumerPackages {
		root := root
		t.Run(root, func(t *testing.T) {
			t.Parallel()
			assertCheckpointPeerPackageTreeForbidden(t, root)
		})
	}
}

func TestCheckpointPeerSurface_PublishedAtRuntimeRoot(t *testing.T) {
	t.Parallel()

	cmd := exec.Command("go", "list", "-f", "{{.GoFiles}}", factoryRuntimeRootPackage)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list files for %s: %v\n%s", factoryRuntimeRootPackage, err, output)
	}
	if !strings.Contains(string(output), "javascript_checkpoint_contract.go") {
		t.Fatalf("%s must publish javascript_checkpoint_contract.go at the Runtime root", factoryRuntimeRootPackage)
	}
}

func TestCheckpointPeerSurface_PeerRecoversThroughRootContractsOnly(t *testing.T) {
	t.Parallel()

	fake := &checkpointRootPeerFake{}
	peer := peerCheckpointRootConsumer{runtime: fake}
	checkpoint, err := peer.recoverOpaqueCheckpoint(context.Background(), "checkpoint-peer-1")
	if err != nil {
		t.Fatalf("recoverOpaqueCheckpoint() error = %v", err)
	}
	if checkpoint.CheckpointID != "checkpoint-peer-1" ||
		checkpoint.SchemaVersion <= 0 ||
		len(checkpoint.Payload) == 0 {
		t.Fatalf("checkpoint = %#v, want opaque root checkpoint", checkpoint)
	}
}

func TestCheckpointPeerSurface_PeerObservesTypedCheckpointFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want error
		call func(factory.Service) error
	}{
		{
			name: "not found",
			want: factory.ErrCheckpointNotFound,
			call: func(runtime factory.Service) error {
				_, err := runtime.LoadCheckpoint(context.Background(), factory.LoadCheckpointRequest{})
				return err
			},
		},
		{
			name: "corrupt",
			want: factory.ErrCorruptCheckpoint,
			call: func(runtime factory.Service) error {
				_, err := runtime.RestoreCheckpoint(context.Background(), factory.RestoreCheckpointRequest{})
				return err
			},
		},
		{
			name: "incompatible",
			want: factory.ErrIncompatibleCheckpoint,
			call: func(runtime factory.Service) error {
				_, err := runtime.RestoreCheckpoint(context.Background(), factory.RestoreCheckpointRequest{
					Checkpoint: factory.Checkpoint{
						CheckpointID: "checkpoint-peer-1", SchemaVersion: 99, Payload: []byte(`{}`),
					},
				})
				return err
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			fake := &checkpointRootPeerFake{
				captured: factory.Checkpoint{
					CheckpointID: "checkpoint-peer-1", SchemaVersion: 1, Payload: []byte(`{}`),
				},
			}
			if err := test.call(fake); !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
}

func assertCheckpointRootContractUsesOnlyRuntimeRootVocabulary(
	t *testing.T,
	typ reflect.Type,
	visiting map[reflect.Type]bool,
) {
	t.Helper()

	for typ.Kind() == reflect.Pointer || typ.Kind() == reflect.Slice ||
		typ.Kind() == reflect.Array || typ.Kind() == reflect.Map {
		typ = typ.Elem()
	}
	if visiting[typ] {
		return
	}
	visiting[typ] = true
	defer delete(visiting, typ)

	switch typ.Kind() {
	case reflect.Interface:
		if typ.PkgPath() == "context" {
			return
		}
		t.Fatalf("checkpoint root contract %s must not expose non-context interface dependency", typ)
	case reflect.Func, reflect.Chan:
		t.Fatalf("checkpoint root contract %s must not expose non-value dependency %s", typ, typ.Kind())
	case reflect.Struct:
		for index := 0; index < typ.NumField(); index++ {
			field := typ.Field(index)
			if field.PkgPath != "" && !field.IsExported() {
				continue
			}
			assertCheckpointRootContractUsesOnlyRuntimeRootVocabulary(t, field.Type, visiting)
		}
	case reflect.String, reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32,
		reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Uintptr, reflect.Float32, reflect.Float64, reflect.Complex64, reflect.Complex128:
		return
	default:
		if typ.PkgPath() == "" {
			return
		}
	}

	pkgPath := typ.PkgPath()
	if pkgPath == "" || pkgPath == "context" || pkgPath == "time" {
		return
	}
	if pkgPath == factoryRuntimeRootPackage {
		return
	}
	for _, forbidden := range forbiddenCheckpointPeerImportPrefixes {
		if pkgPath == forbidden || strings.HasPrefix(pkgPath, forbidden) {
			t.Fatalf("checkpoint root contract type %s depends on forbidden consumer path %s", typ, pkgPath)
		}
	}
	if strings.Contains(pkgPath, "factory_definitions") {
		t.Fatalf("checkpoint root contract type %s depends on Definitions checkpoint record vocabulary %s", typ, pkgPath)
	}
	t.Fatalf(
		"checkpoint root contract type %s depends on unexpected package %s; peer surface must use only Runtime root checkpoint vocabulary",
		typ,
		pkgPath,
	)
}

func assertCheckpointPeerPackageTreeForbidden(t *testing.T, packageRoot string) {
	t.Helper()

	cmd := exec.Command(
		"go",
		"list",
		"-deps",
		"-f",
		"{{if not .Standard}}{{.ImportPath}}{{end}}",
		packageRoot,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list deps for %s: %v\n%s", packageRoot, err, output)
	}
	for _, dep := range strings.Fields(string(output)) {
		for _, forbidden := range forbiddenCheckpointPeerImportPrefixes {
			if dep == forbidden || strings.HasPrefix(dep, forbidden) {
				t.Fatalf(
					"%s must not depend on checkpoint recovery internals %s; found dependency %s",
					packageRoot,
					forbidden,
					dep,
				)
			}
		}
	}
}
