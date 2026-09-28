package run

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/portpowered/infinite-you/pkg/initializer"
	"github.com/portpowered/infinite-you/pkg/initializer/lifecycle"
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
}

func NewDirectJavaScriptRunOperation(
	sessions factorysessions.Service,
	generateSessionID factorysessions.SessionIDGenerator,
	host DirectJavaScriptHost,
	presentations factorysessions.OpeningPresentationOwner,
) (DirectJavaScriptRunOperation, error) {
	if sessions == nil || generateSessionID == nil || host == nil || presentations == nil {
		return nil, fmt.Errorf("direct JavaScript run requires Factory Sessions, identity, HTTP host, and presentation")
	}
	return &directJavaScriptRun{sessions: sessions, generateSessionID: generateSessionID, host: host, presentations: presentations}, nil
}

func (*directJavaScriptRun) Supports(sourcePath string) bool {
	switch strings.ToLower(filepath.Ext(strings.TrimSpace(sourcePath))) {
	case ".js", ".mjs", ".cjs":
		return true
	default:
		return false
	}
}

func (operation *directJavaScriptRun) Open(
	ctx context.Context,
	request factorysessions.DirectJavaScriptRunRequest,
	cancellation initializer.InvocationCancellation,
) (factorysessions.DirectJavaScriptApplication, error) {
	if operation == nil || operation.sessions == nil {
		return factorysessions.DirectJavaScriptApplication{}, fmt.Errorf("Factory Sessions service is required")
	}
	scope, ok := operation.presentations.DirectJavaScript(request.ScopeID)
	if !ok {
		return factorysessions.DirectJavaScriptApplication{}, fmt.Errorf("direct JavaScript presentation scope %q is unavailable", request.ScopeID)
	}
	sourcePath, err := filepath.Abs(strings.TrimSpace(request.SourcePath))
	if err != nil {
		return factorysessions.DirectJavaScriptApplication{}, fmt.Errorf("resolve workflow source: %w", err)
	}
	if !operation.Supports(sourcePath) {
		return factorysessions.DirectJavaScriptApplication{}, fmt.Errorf("workflow source %q is not a supported JavaScript file", request.SourcePath)
	}
	requestID := strings.TrimSpace(operation.generateSessionID())
	if requestID == "" {
		return factorysessions.DirectJavaScriptApplication{}, fmt.Errorf("Factory Session ID generator returned an empty identity")
	}
	childMode := factorysessions.ChildExecutorModeLive
	if request.MockWorkersEnabled {
		childMode = factorysessions.ChildExecutorModeFake
	}
	start := factorysessions.SessionStartRequest{
		Mode:        factorysessions.SessionOperationModeDurable,
		FolderPath:  filepath.Dir(sourcePath),
		Synchronous: true,
		Correlation: factorysessions.SessionOperationCorrelation{RequestID: "run-" + requestID},
		Source: factorysessions.Source{
			Kind: factoryruntime.WorkflowSourceKindWorkflowFile, WorkflowFile: sourcePath,
		},
		RuntimeOptions: &factorysessions.RuntimeOptions{ChildExecutorMode: childMode},
	}
	completion := func(runCtx context.Context) error {
		return sessionexecution.RunCanonicalSync(runCtx, operation.sessions, start, request.JSONOutput, scope.Output)
	}
	var transport lifecycle.Component
	if request.Host != nil {
		ready := make(chan struct{})
		var publish sync.Once
		observer := func(binding factorysessions.RuntimeHostBinding) {
			publish.Do(func() {
				if scope.RuntimeHostObserver != nil {
					scope.RuntimeHostObserver(binding)
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
		transport, err = operation.host(operation.sessions, *request.Host, cancellation, observer)
		if err != nil {
			return factorysessions.DirectJavaScriptApplication{}, err
		}
	}
	plan, err := sessionexecution.DirectJavaScriptLifecyclePlan(transport, completion)
	if err != nil {
		return factorysessions.DirectJavaScriptApplication{}, errors.Join(err, stopDirectJavaScriptTransport(ctx, transport))
	}
	return factorysessions.DirectJavaScriptApplication{Plan: plan}, nil
}

func stopDirectJavaScriptTransport(ctx context.Context, transport lifecycle.Component) error {
	if transport == nil {
		return nil
	}
	return transport.Stop(ctx)
}

var _ DirectJavaScriptRunOperation = (*directJavaScriptRun)(nil)
