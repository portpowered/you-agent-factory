package executionopening

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/processlifecycle"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
)

type directJavaScriptRunOperation struct {
	build             roles.ExecutionServiceBuilder
	runSync           roles.DirectJavaScriptSyncRunner
	generateSessionID factorysessions.SessionIDGenerator
	host              roles.DirectJavaScriptHostAdapter
}

type directJavaScriptTransport interface {
	Start(context.Context) error
	Stop(context.Context) error
}

type preparedDirectJavaScriptExecution struct {
	execution  roles.OwnedExecutionService
	sourcePath string
	requestID  string
	childMode  string
}

// NewDirectJavaScriptRunOperation constructs the Factory Sessions-owned raw
// JavaScript invocation boundary.
func NewDirectJavaScriptRunOperation(
	build roles.ExecutionServiceBuilder,
	runSync roles.DirectJavaScriptSyncRunner,
	generateSessionID factorysessions.SessionIDGenerator,
	host roles.DirectJavaScriptHostAdapter,
) (roles.DirectJavaScriptRunOperation, error) {
	if build == nil {
		return nil, errors.New("session execution builder is required")
	}
	if runSync == nil {
		return nil, errors.New("direct JavaScript sync runner is required")
	}
	if generateSessionID == nil {
		return nil, errors.New("Factory Session ID generator is required")
	}
	if host == nil {
		return nil, errors.New("direct JavaScript host adapter is required")
	}
	return &directJavaScriptRunOperation{
		build: build, runSync: runSync, generateSessionID: generateSessionID, host: host,
	}, nil
}

func (*directJavaScriptRunOperation) Supports(sourcePath string) bool {
	switch strings.ToLower(filepath.Ext(strings.TrimSpace(sourcePath))) {
	case ".js", ".mjs", ".cjs":
		return true
	default:
		return false
	}
}

func (o *directJavaScriptRunOperation) Open(
	ctx context.Context,
	request factorysessions.DirectJavaScriptRunRequest,
) (factorysessions.DirectJavaScriptApplication, error) {
	if o == nil || o.build == nil || o.runSync == nil {
		return factorysessions.DirectJavaScriptApplication{}, errors.New("direct JavaScript run operation is unavailable")
	}
	prepared, err := o.prepareExecution(ctx, request)
	if err != nil {
		return factorysessions.DirectJavaScriptApplication{}, err
	}
	completion := o.completion(prepared, request)
	transport, completion, err := o.prepareHosting(request, prepared.execution, completion)
	if err != nil {
		return factorysessions.DirectJavaScriptApplication{}, errors.Join(err, prepared.execution.Close())
	}
	plan, err := processlifecycle.BuildDirectJavaScriptLifecyclePlan(
		transport, completion, prepared.execution.Close,
	)
	if err != nil {
		return factorysessions.DirectJavaScriptApplication{}, errors.Join(err, prepared.execution.Close())
	}
	return factorysessions.DirectJavaScriptApplication{Plan: plan}, nil
}

func (o *directJavaScriptRunOperation) prepareExecution(
	ctx context.Context,
	request factorysessions.DirectJavaScriptRunRequest,
) (preparedDirectJavaScriptExecution, error) {
	sourcePath, err := filepath.Abs(strings.TrimSpace(request.SourcePath))
	if err != nil {
		return preparedDirectJavaScriptExecution{}, fmt.Errorf("resolve workflow source: %w", err)
	}
	if !o.Supports(sourcePath) {
		return preparedDirectJavaScriptExecution{}, fmt.Errorf(
			"workflow source %q is not a supported JavaScript file", request.SourcePath,
		)
	}
	childMode := factorysessions.ChildExecutorModeLive
	if request.MockWorkersEnabled {
		childMode = factorysessions.ChildExecutorModeFake
	}
	execution, err := o.build(
		ctx,
		string(factorysessions.ExecutionProviderJavaScriptRuntime),
		filepath.Dir(sourcePath),
		"",
		childMode,
	)
	if err != nil {
		return preparedDirectJavaScriptExecution{}, fmt.Errorf("open direct JavaScript execution: %w", err)
	}

	requestID := "run-" + strings.TrimSpace(o.generateSessionID())
	if requestID == "run-" {
		return preparedDirectJavaScriptExecution{}, errors.Join(
			errors.New("Factory Session ID generator returned an empty identity"),
			execution.Close(),
		)
	}
	return preparedDirectJavaScriptExecution{
		execution: execution, sourcePath: sourcePath, requestID: requestID, childMode: childMode,
	}, nil
}

func (o *directJavaScriptRunOperation) completion(
	prepared preparedDirectJavaScriptExecution,
	request factorysessions.DirectJavaScriptRunRequest,
) func(context.Context) error {
	return func(runCtx context.Context) error {
		return o.runSync(runCtx, prepared.execution, factorysessions.StartRequest{
			RequestID: prepared.requestID,
			Source: factorysessions.Source{
				Kind:         factoryruntime.WorkflowSourceKindWorkflowFile,
				WorkflowFile: prepared.sourcePath,
			},
			Runtime: &factorysessions.RuntimeOptions{ChildExecutorMode: prepared.childMode},
		}, request.JSONOutput, request.Output)
	}
}

func (o *directJavaScriptRunOperation) prepareHosting(
	request factorysessions.DirectJavaScriptRunRequest,
	execution roles.OwnedExecutionService,
	completion func(context.Context) error,
) (directJavaScriptTransport, func(context.Context) error, error) {
	if request.Host == nil {
		return nil, completion, nil
	}
	ready := make(chan struct{})
	observer := request.RuntimeHostObserver
	var publish sync.Once
	request.RuntimeHostObserver = func(binding factorysessions.RuntimeHostBinding) {
		publish.Do(func() {
			if observer != nil {
				observer(binding)
			}
			close(ready)
		})
	}
	runAfterReady := completion
	completion = func(runCtx context.Context) error {
		select {
		case <-ready:
			return runAfterReady(runCtx)
		case <-runCtx.Done():
			return runCtx.Err()
		}
	}
	transport, err := o.host(execution, directJavaScriptLifecycle{execution}, request)
	return transport, completion, err
}

type directJavaScriptLifecycle struct {
	execution durableexecution.Service
}

func (adapter directJavaScriptLifecycle) PauseDurableFactorySession(
	ctx context.Context, sessionID string, request factorysessions.ControlRequest,
) (factorysessions.LifecycleControlResult, error) {
	return adapter.execution.Pause(ctx, sessionID, request)
}

func (adapter directJavaScriptLifecycle) ResumeDurableFactorySession(
	ctx context.Context, sessionID string, request factorysessions.ControlRequest,
) (factorysessions.LifecycleControlResult, error) {
	return adapter.execution.Resume(ctx, sessionID, request)
}

func (adapter directJavaScriptLifecycle) CancelDurableFactorySession(
	ctx context.Context, sessionID string, request factorysessions.ControlRequest,
) (factorysessions.LifecycleControlResult, error) {
	return adapter.execution.Cancel(ctx, sessionID, request)
}

func (adapter directJavaScriptLifecycle) TerminateDurableFactorySession(
	ctx context.Context, sessionID string, request factorysessions.ControlRequest,
) (factorysessions.LifecycleControlResult, error) {
	return adapter.execution.Terminate(ctx, sessionID, request)
}

func (adapter directJavaScriptLifecycle) ApproveDurableFactorySession(
	ctx context.Context, sessionID string, request factorysessions.ApproveRequest,
) (factorysessions.LifecycleControlResult, error) {
	return adapter.execution.Approve(ctx, sessionID, request)
}

func (adapter directJavaScriptLifecycle) RetryDurableFactorySessionDispatch(
	ctx context.Context, sessionID string, request factorysessions.RetryDispatchRequest,
) (factorysessions.LifecycleControlResult, error) {
	return adapter.execution.RetryDispatch(ctx, sessionID, request)
}

func (adapter directJavaScriptLifecycle) InterruptDurableFactorySessionDispatch(
	ctx context.Context, sessionID string, request factorysessions.InterruptDispatchRequest,
) (factorysessions.LifecycleControlResult, error) {
	return adapter.execution.InterruptDispatch(ctx, sessionID, request)
}

func (adapter directJavaScriptLifecycle) ReadDurableFactorySessionEventStream(
	ctx context.Context, sessionID string, request factorysessions.EventReconnectRequest,
) (*factorydefinitions.FactoryEventStream, error) {
	result, err := adapter.execution.ReadEvents(ctx, sessionID, request)
	if err != nil {
		return nil, err
	}
	return factorysessions.MaterializeEventReadStream(result), nil
}

func (adapter directJavaScriptLifecycle) ProbeDurableFactorySessionEvents(
	ctx context.Context, sessionID string, request factorysessions.EventReconnectRequest,
) error {
	_, err := adapter.execution.ReadEvents(ctx, sessionID, request)
	return err
}

func (adapter directJavaScriptLifecycle) SubscribeDurableFactoryResponseEvents(
	ctx context.Context,
	request factorysessions.ResponseEventSubscriptionRequest,
) (*factorysessions.ResponseEventCursor, error) {
	subscriber, ok := adapter.execution.(interface {
		SubscribeResponseEvents(context.Context, string, factorysessions.ResponseEventSubscriptionRequest) (*factorysessions.ResponseEventCursor, error)
	})
	if !ok {
		return nil, factorysessions.ErrRuntimeNotAvailable
	}
	return subscriber.SubscribeResponseEvents(ctx, request.SessionID, request)
}

var _ roles.DirectJavaScriptRunOperation = (*directJavaScriptRunOperation)(nil)
