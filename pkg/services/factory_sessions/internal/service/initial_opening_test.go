package service

import (
	"context"
	"errors"
	"fmt"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"os"
	"strings"
	"testing"
)

func TestRuntimeActivationInitialFailureRetainsSoleCloserForRetry(t *testing.T) {
	t.Parallel()
	snapshot := activationSnapshot()
	openingErr, closeErr := errors.New("initial opening failed"), errors.New("partial release failed")
	calls := 0
	owner := &Root{initialActivation: func(context.Context, factoryruntime.RuntimeActivationRequest, factoryruntime.SessionObservations) (*factoryruntime.RuntimeInitialOpening, error) {
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
	}}
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
