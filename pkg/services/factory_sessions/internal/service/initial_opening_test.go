package service

import (
	"context"
	"errors"
	"fmt"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestRuntimeActivationInitialFailureRetainsSoleCloserForRetry(t *testing.T) {
	t.Parallel()
	snapshot := activationSnapshot()
	openingErr, closeErr := errors.New("initial opening failed"), errors.New("partial release failed")
	calls := 0
	owner := &Root{initialEngine: NewRuntimeInitialEngine(nil, func(context.Context, factoryruntime.RuntimeActivationRequest, factoryruntime.SessionObservations) (*factoryruntime.RuntimeInitialOpening, error) {
		return &factoryruntime.RuntimeInitialOpening{Activation: &factoryruntime.RuntimeActivation{
			Close: func(ctx context.Context) error {
				if ctx.Err() != nil {
					t.Fatal("cleanup inherited opening cancellation")
				}
				calls++
				if calls == 1 {
					return closeErr
				}
				return nil
			},
		}}, openingErr
	})}
	opening := &sessionRuntimeOpening{sessionID: "candidate", configured: preparedRuntime{DefinitionSnapshot: &snapshot},
		load: RuntimeLoad{LoadedFactoryCfg: initialOpeningLoadedStub{}}}
	cleanup := &runtimeOpeningCleanup{}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err := owner.openSessionEngine(ctx, opening, cleanup)
	if !errors.Is(err, openingErr) || calls != 0 {
		t.Fatalf("opening = %v, calls=%d", err, calls)
	}
	joined := errors.Join(err, cleanup.Close())
	if !errors.Is(joined, openingErr) || !errors.Is(joined, closeErr) {
		t.Fatalf("lost primary/cleanup causes: %v", joined)
	}
	if err := cleanup.Close(); err != nil {
		t.Fatal(err)
	}
	if err := cleanup.Close(); err != nil || calls != 2 {
		t.Fatalf("successful release repeated: %v calls=%d", err, calls)
	}
}

type initialOpeningLoadedStub struct {
	factorydefinitions.MutableLoadedFactorySource
}

func (initialOpeningLoadedStub) FactoryConfig() *factorydefinitions.FactoryConfig {
	return &factorydefinitions.FactoryConfig{Name: "snapshot"}
}

func (initialOpeningLoadedStub) FactoryDir() string { return "isolated-factory" }

type failedOpeningLogOwner struct {
	sink    factoryruntime.RuntimeLogSink
	err     error
	request factoryruntime.RuntimeLogScopeRequest
	calls   int
}

func (owner *failedOpeningLogOwner) Open(_ *zap.Logger, request factoryruntime.RuntimeLogScopeRequest) (factoryruntime.RuntimeLogSink, error) {
	owner.calls++
	owner.request = request
	return owner.sink, owner.err
}

type failedOpeningLogSink struct {
	logger   *zap.Logger
	closeErr error
	closes   int
}

func (sink *failedOpeningLogSink) Logger() *zap.Logger { return sink.logger }
func (sink *failedOpeningLogSink) Artifact() factoryruntime.RuntimeLogArtifact {
	return factoryruntime.RuntimeLogArtifact{}
}
func (sink *failedOpeningLogSink) Close() error { sink.closes++; return sink.closeErr }

func TestFailedSessionOpeningLogRetainsSafeJoinedCauseAndReleasesSink(t *testing.T) {
	t.Parallel()
	core, logs := observer.New(zap.ErrorLevel)
	closeErr := errors.New("close diagnostic sink")
	sink := &failedOpeningLogSink{logger: zap.New(core), closeErr: closeErr}
	owner := &failedOpeningLogOwner{sink: sink}
	root := &Root{runtimeLogs: owner}
	opening := &sessionRuntimeOpening{sessionID: "candidate", load: RuntimeLoad{LoadedFactoryCfg: initialOpeningLoadedStub{}},
		configured: preparedRuntime{Runtime: factoryruntime.RuntimeSelection{RuntimeInstanceID: "candidate-runtime", LogDirectory: "isolated-logs"}}}
	cause := errors.Join(fmt.Errorf("restore retained history: %w", &os.PathError{Op: "read", Path: "retained.json", Err: os.ErrPermission}), errors.New("private-payload-marker"))
	cleanup := &runtimeOpeningCleanup{}
	err := root.logFailedSessionOpening(opening, cause, cleanup)
	if !errors.Is(err, closeErr) || sink.closes != 1 || owner.request.SessionID != "candidate" || owner.request.RootDirectory != "isolated-logs" {
		t.Fatalf("diagnostic lifecycle = %v, closes=%d, request=%#v", err, sink.closes, owner.request)
	}
	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("diagnostic entries = %v", entries)
	}
	fields := entries[0].ContextMap()
	text, _ := fields["cause"].(string)
	if fields["failure_class"] != "runtime_startup_failed" || !strings.Contains(text, "permission denied") || strings.Contains(text, "private-payload-marker") {
		t.Fatalf("unsafe or missing joined diagnostic: %#v", fields)
	}
	sink.closeErr = nil
	if err := cleanup.Close(); err != nil || sink.closes != 2 {
		t.Fatalf("diagnostic sink retry = %v, closes=%d", err, sink.closes)
	}
	opening.configured.Runtime.FileLoggingPolicy = factoryruntime.RuntimeFileLoggingPolicyDisabled
	if err := root.logFailedSessionOpening(opening, cause, cleanup); err != nil || owner.calls != 1 {
		t.Fatalf("disabled logging opened a sink: %v, calls=%d", err, owner.calls)
	}
}

func TestInitialEngineLiveDetachesOverlappingSelectionsAndObservations(t *testing.T) {
	t.Parallel()
	snapshot := activationSnapshot()
	snapshot.Workers = []factorydefinitions.FactoryWorkerConfig{{Name: "worker", Args: []string{"original"}}}
	snapshot.Workstations = []factorydefinitions.FactoryWorkstationConfig{{Name: "station"}}
	entered := make(chan factoryruntime.RuntimeActivationRequest, 2)
	release := make(chan struct{})
	engine := NewRuntimeInitialEngine(nil, func(_ context.Context, request factoryruntime.RuntimeActivationRequest,
		observations factoryruntime.SessionObservations) (*factoryruntime.RuntimeInitialOpening, error) {
		entered <- request
		<-release
		if err := observations.RecordPetriTokenMutations(request.FactorySessionID, nil); err != nil {
			return nil, err
		}
		observations.PublishWorkerProgress(workers.ProgressFragment{Payload: request.RuntimeID})
		request.Snapshot.Workers[0].Args[0] = "activation-owned"
		return nil, nil
	})
	done := make(chan error, 2)
	mutations := make(chan string, 2)
	progress := make(chan string, 2)
	for _, id := range []string{"one", "two"} {
		go func() {
			_, err := engine.OpenLive(t.Context(), initialEngineLiveRequest{
				Configured: preparedRuntime{DefinitionSnapshot: &snapshot,
					Session: factorysessions.SessionStartRequest{SessionID: id},
					Runtime: factoryruntime.RuntimeSelection{RuntimeInstanceID: id},
				}, EffectiveFactory: factorydefinitions.FactoryConfig{Name: id},
				Workers:      []factorydefinitions.FactoryWorkerConfig{{Name: "worker", Args: []string{id}}},
				Workstations: []factorydefinitions.FactoryWorkstationConfig{{Name: "station", WorkerTypeName: id}},
			}, replaySessionObservations{
				mutations: func(sessionID string, _ []factorydefinitions.TokenMutationRecord) error {
					if sessionID != id {
						return fmt.Errorf("observation retargeted: %s", sessionID)
					}
					mutations <- sessionID
					return nil
				}, progress: func(fragment workers.ProgressFragment) { progress <- fragment.Payload },
			})
			done <- err
		}()
	}
	for range 2 {
		request := <-entered
		id := request.FactorySessionID
		if got := [4]string{request.RuntimeID, request.Snapshot.EffectiveFactory.Name,
			request.Snapshot.Workers[0].Args[0], request.Snapshot.Workstations[0].WorkerTypeName}; got != [4]string{id, id, id, id} {
			t.Errorf("selected facts crossed sessions: %#v", request)
		}
	}
	close(release)
	for range 2 {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	if snapshot.Workers[0].Args[0] != "original" || snapshot.EffectiveFactory.Name != "snapshot" {
		t.Fatalf("caller snapshot mutated: %#v", snapshot)
	}
	seenMutations, seenProgress := map[string]bool{}, map[string]bool{}
	for range 2 {
		seenMutations[<-mutations] = true
		seenProgress[<-progress] = true
	}
	want := map[string]bool{"one": true, "two": true}
	if !reflect.DeepEqual(seenMutations, want) || !reflect.DeepEqual(seenProgress, want) {
		t.Fatalf("scoped observations lost: %v %v", seenMutations, seenProgress)
	}
}

func TestInitialEngineCheckpointAuthoredSelectionRetainsPartialError(t *testing.T) {
	t.Parallel()
	snapshot := activationSnapshot()
	snapshot.Workers = []factorydefinitions.FactoryWorkerConfig{{Name: "authored", Args: []string{"original"}}}
	cause := &os.PathError{Op: "open", Path: "selected", Err: os.ErrPermission}
	partial := &factoryruntime.RuntimeInitialOpening{Activation: &factoryruntime.RuntimeActivation{}}
	observation := replaySessionObservations{mutations: func(string, []factorydefinitions.TokenMutationRecord) error { return nil }}
	engine := NewRuntimeInitialEngine(func(_ context.Context, definition factorydefinitions.RuntimeSelection,
		recording recordings.RuntimeSelection, replay *recordings.LoadReplayInputResult,
		resume *recordings.LoadResumeInputResult, id string) (activationSnapshotResolution, error) {
		if definition.Directory != "authored" || recording.ReplayPath != "" || replay != nil || resume != nil || id != "selected" {
			t.Fatalf("checkpoint selected recorded inputs: %#v %#v %s", definition, recording, id)
		}
		return activationSnapshotResolution{snapshot: snapshot}, nil
	}, func(_ context.Context, request factoryruntime.RuntimeActivationRequest,
		observations factoryruntime.SessionObservations) (*factoryruntime.RuntimeInitialOpening, error) {
		if request.Inputs.Session.CanonicalSessionID != "selected" || !request.Inputs.RecoveryInput.CheckpointContinuation ||
			request.RuntimeID != "selected-runtime" || request.Snapshot.Workers[0].Name != "authored" {
			t.Fatalf("checkpoint identity lost: %#v", request)
		}
		if err := observations.RecordPetriTokenMutations("selected", nil); err != nil {
			t.Fatal(err)
		}
		observations.PublishWorkerProgress(workers.ProgressFragment{}) // Optional on portable compatibility path.
		request.Snapshot.Workers[0].Args[0] = "activation-owned"
		return partial, cause
	})
	opening, err := engine.OpenCheckpoint(t.Context(), preparedRuntime{
		Definition: factorydefinitions.RuntimeSelection{Directory: "authored"},
		Session:    factorysessions.SessionStartRequest{SessionID: "selected"},
		Runtime:    factoryruntime.RuntimeSelection{RuntimeInstanceID: "selected-runtime"},
		Recordings: recordings.RuntimeSelection{ReplayPath: "recorded"},
	}, observation)
	if opening != partial || !errors.Is(err, cause) || snapshot.Workers[0].Args[0] != "original" {
		t.Fatalf("partial opening/error or detached snapshot lost: %p %v %#v", opening, err, snapshot)
	}
}
