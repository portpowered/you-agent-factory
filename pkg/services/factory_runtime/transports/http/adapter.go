// Package http owns HTTP adaptation for Factory Runtime operations.
//
// The top-level HTTP transport registers generated routes and composes this
// adapter when PSS fan-in arrives. Request decoding, generated contract mapping,
// Runtime root invocation, error mapping, and cancel/timeout policy for
// Runtime-owned HTTP operations remain here with the owning service.
package http

import (
	"context"
	"net/http"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/transports/http/checkpoint"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/transports/http/control"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/transports/http/dispatch"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/transports/http/internal/common"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/transports/http/observation"
)

// RuntimeRoot is the accepted Factory Runtime root contract used by the HTTP
// adapter. Adapter-owned operations invoke this surface rather than Runtime
// internal packages.
type RuntimeRoot = factoryruntime.Service

// SessionObserver routes session-scoped Runtime observation requests through
// the Factory Sessions peer surface without importing Sessions internals.
type SessionObserver = observation.SessionObserver

// These aliases keep the existing package-local HTTP test vocabulary stable
// while the response shapes are owned by their operation packages.
type runtimeControlHTTPResponse = control.RuntimeControlHTTPResponse
type runtimeDispatchPlanHTTPResponse = dispatch.RuntimeDispatchPlanHTTPResponse
type runtimeCheckpointHTTP = checkpoint.RuntimeCheckpointHTTP
type runtimeCaptureCheckpointHTTPResponse = checkpoint.RuntimeCaptureCheckpointHTTPResponse
type runtimeLoadCheckpointHTTPResponse = checkpoint.RuntimeLoadCheckpointHTTPResponse
type runtimeRestoreCheckpointHTTPResponse = checkpoint.RuntimeRestoreCheckpointHTTPResponse

type observationHandler = observation.Handler
type controlHandler = control.Handler
type dispatchHandler = dispatch.Handler
type checkpointHandler = checkpoint.Handler

// Adapter maps Factory Runtime HTTP operations through the accepted root
// contract without importing Runtime internals or owning canonical state.
type Adapter struct {
	root RuntimeRoot

	*observationHandler
	*controlHandler
	*dispatchHandler
	*checkpointHandler
}

// NewAdapter constructs the Factory Runtime HTTP adapter bound to the accepted
// root Service seam. Nil roots fail closed by returning nil.
func NewAdapter(root RuntimeRoot) *Adapter {
	if root == nil {
		return nil
	}
	return &Adapter{
		root:               root,
		observationHandler: observation.NewHandler(root),
		controlHandler:     control.NewHandler(root),
		dispatchHandler:    dispatch.NewHandler(root),
		checkpointHandler:  checkpoint.NewHandler(root),
	}
}

// BindSessionObserver attaches the peer used for session-scoped status reads.
func (a *Adapter) BindSessionObserver(sessions SessionObserver) {
	if a != nil && a.observationHandler != nil {
		a.observationHandler.BindSessionObserver(sessions)
	}
}

// Root returns the injected Factory Runtime root consumed by adapter-owned
// operations.
func (a *Adapter) Root() RuntimeRoot {
	if a == nil {
		return nil
	}
	return a.root
}

func (a *Adapter) runtimeRoot() (RuntimeRoot, error) {
	if a == nil {
		return common.RequireRuntimeRoot(nil)
	}
	return common.RequireRuntimeRoot(a.root)
}

// invokeControlResume preserves the package-local helper used by the legacy
// adapter tests while the actual operation lives in the control child package.
func (a *Adapter) invokeControlResume(w http.ResponseWriter, ctx context.Context) {
	if a == nil || a.controlHandler == nil {
		common.WriteError(w, http.StatusInternalServerError, "factory runtime service is required", "INTERNAL_ERROR")
		return
	}
	a.controlHandler.InvokeResume(w, ctx)
}

// writeJSON and writeError retain the old package-local helpers for the
// compatibility façade and its focused tests. New operation handlers use the
// shared common package directly.
func (a *Adapter) writeJSON(w http.ResponseWriter, status int, value any) {
	common.WriteJSON(w, status, value)
}

func (a *Adapter) writeError(w http.ResponseWriter, status int, message, code string) {
	common.WriteError(w, status, message, code)
}

func runtimeRequestContextErrorResponse(err error) (status int, response any, handled bool) {
	return common.RequestContextErrorResponse(err)
}

func requestContextEnded(ctx context.Context) bool {
	return common.RequestContextEnded(ctx)
}

func isRequestContextEnded(err error) bool {
	return common.IsRequestContextEnded(err)
}

func shouldEndOnRequestContext(ctx context.Context, err error) bool {
	return common.ShouldEndOnRequestContext(ctx, err)
}

func (a *Adapter) writeRuntimeRequestContextOutcome(w http.ResponseWriter, err error) bool {
	return common.WriteRequestContextOutcome(w, err)
}

func (a *Adapter) guardRuntimeRequestContext(w http.ResponseWriter, r *http.Request) bool {
	return common.GuardRequestContext(w, r)
}

// OwnedHTTPOperationIDs lists the generated OpenAPI operationIds adapted by
// this package. HTTP-RUN adds Runtime control, move-work, dispatch-plan, and
// checkpoint slices without authoring new shared OpenAPI operations.
var OwnedHTTPOperationIDs = []string{
	"getStatus",
	"getStatusBySessionId",
	"moveWorkBySessionId",
}
