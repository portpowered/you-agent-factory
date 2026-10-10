package run

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/portpowered/infinite-you/pkg/initializer"
	"github.com/portpowered/infinite-you/pkg/initializer/lifecycle"
	"github.com/portpowered/infinite-you/pkg/platform/runtimeartifact"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/cli/sessionexecution"
)

// DirectJavaScriptHost binds the process Factory Sessions root to a CLI HTTP
// transport. It owns no Factory Session state.
type DirectJavaScriptHost func(
	factorysessions.Service,
	factorysessions.RuntimeHostRequest,
	initializer.InvocationCancellation,
	factorysessions.RuntimeHostObserver,
) (lifecycle.Component, error)

type directJavaScriptRun struct {
	sessions          factorysessions.Service
	generateSessionID factorysessions.SessionIDGenerator
	host              DirectJavaScriptHost
	presentations     factorysessions.OpeningPresentationOwner
	buildApplication  initializer.LifecycleRunnerBuilder
}

func NewDirectJavaScriptRunOperation(
	sessions factorysessions.Service,
	generateSessionID factorysessions.SessionIDGenerator,
	host DirectJavaScriptHost,
	presentations factorysessions.OpeningPresentationOwner,
	buildApplication initializer.LifecycleRunnerBuilder,
) (DirectJavaScriptRunOperation, error) {
	if sessions == nil || generateSessionID == nil || host == nil || presentations == nil || buildApplication == nil {
		return nil, fmt.Errorf("direct JavaScript run requires Factory Sessions, identity, HTTP host, presentation, and lifecycle")
	}
	return &directJavaScriptRun{sessions: sessions, generateSessionID: generateSessionID, host: host, presentations: presentations, buildApplication: buildApplication}, nil
}

func (*directJavaScriptRun) Supports(sourcePath string) bool {
	switch strings.ToLower(filepath.Ext(strings.TrimSpace(sourcePath))) {
	case ".js", ".mjs", ".cjs":
		return true
	default:
		return false
	}
}

func (operation *directJavaScriptRun) Run(
	ctx context.Context,
	request factorysessions.DirectJavaScriptRunRequest,
	cancellation initializer.InvocationCancellation,
	discloseStartup func(),
) error {
	request.Caller = request.Caller.Clone()
	if operation == nil || operation.sessions == nil {
		return fmt.Errorf("Factory Sessions service is required")
	}
	if ctx == nil {
		return fmt.Errorf("direct JavaScript run context is required")
	}
	scope, ok := operation.presentations.DirectJavaScript(request.ScopeID)
	if !ok {
		return fmt.Errorf("direct JavaScript presentation scope %q is unavailable", request.ScopeID)
	}
	sourcePath, err := filepath.Abs(strings.TrimSpace(request.SourcePath))
	if err != nil {
		return fmt.Errorf("resolve workflow source: %w", err)
	}
	if !operation.Supports(sourcePath) {
		return fmt.Errorf("workflow source %q is not a supported JavaScript file", request.SourcePath)
	}
	requestID := strings.TrimSpace(operation.generateSessionID())
	if requestID == "" {
		return fmt.Errorf("Factory Session ID generator returned an empty identity")
	}
	childMode := factorysessions.ChildExecutorModeLive
	if request.MockWorkersEnabled {
		childMode = factorysessions.ChildExecutorModeFake
	}
	start := factorysessions.SessionStartRequest{
		Caller:      request.Caller.Clone(),
		Mode:        factorysessions.SessionOperationModeDurable,
		FolderPath:  filepath.Dir(sourcePath),
		Synchronous: true,
		Correlation: factorysessions.SessionOperationCorrelation{RequestID: "run-" + requestID},
		Source: factorysessions.Source{
			Kind: factoryruntime.WorkflowSourceKindWorkflowFile, WorkflowFile: sourcePath,
		},
		RuntimeOptions: &factorysessions.RuntimeOptions{ChildExecutorMode: childMode},
	}
	if request.RecordPath != "" {
		start.RuntimeSelection = &factorysessions.SessionRuntimeSelection{
			Recording: factorysessions.SessionRecordingSelection{RecordPath: request.RecordPath},
		}
	}
	completion := func(runCtx context.Context) error {
		return sessionexecution.RunCanonicalSync(runCtx, operation.sessions, start, request.JSONOutput, scope.Output)
	}
	var transport lifecycle.Component
	if request.Host != nil {
		readiness := lifecycle.NewReadinessGate()
		observer := func(binding factorysessions.RuntimeHostBinding) {
			readiness.Publish(func() {
				if scope.RuntimeHostObserver != nil {
					scope.RuntimeHostObserver(binding)
				}
			})
		}
		runAfterReady := completion
		completion = func(runCtx context.Context) error {
			if err := readiness.Wait(runCtx); err != nil {
				return err
			}
			return runAfterReady(runCtx)
		}
		transport, err = operation.host(operation.sessions, *request.Host, cancellation, observer)
		if err != nil {
			return err
		}
	}
	plan, err := sessionexecution.DirectJavaScriptLifecyclePlan(transport, completion)
	if err != nil {
		return errors.Join(err, stopDirectJavaScriptTransport(ctx, transport))
	}
	return operation.runLifecycle(ctx, plan, transport, discloseStartup)
}

func (operation *directJavaScriptRun) runLifecycle(ctx context.Context, plan lifecycle.Plan, transport lifecycle.Component, discloseStartup func()) error {
	runner, err := operation.buildApplication(ctx, plan, runtimeartifact.Diagnostics{}, nil)
	if err != nil {
		return errors.Join(err, stopDirectJavaScriptTransport(ctx, transport))
	}
	if runner == nil {
		return errors.Join(fmt.Errorf("direct JavaScript application builder returned nil runner"), stopDirectJavaScriptTransport(ctx, transport))
	}
	if discloseStartup != nil {
		discloseStartup()
	}
	return runner.Run(ctx)
}

func stopDirectJavaScriptTransport(ctx context.Context, transport lifecycle.Component) error {
	if transport == nil {
		return nil
	}
	return transport.Stop(context.WithoutCancel(ctx))
}

var _ DirectJavaScriptRunOperation = (*directJavaScriptRun)(nil)
