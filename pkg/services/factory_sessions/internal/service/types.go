package service

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/recordingreplay"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimeports"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

func recoveryRecordingID(recordingID string) string {
	recordingID = strings.TrimSpace(recordingID)
	if recordingID == "" {
		return ""
	}
	digest := sha256.Sum256([]byte(recordingID))
	return "sha256:" + hex.EncodeToString(digest[:])
}

// runtimeProducts is the invocation-local output of Factory Runtime assembly.
// It is consumed by canonical Start and never stored as another session graph.
type runtimeProducts struct {
	lifecycle       roles.LifecycleRuntime
	replayExecution *recordingreplay.Scope
	closeArtifacts  func() error
	bindRuntime     func(factoryruntime.RuntimeBinding) error
}

type workerSessionsObservationProvider interface {
	WorkerSessionsObservation() workersessions.ObservationService
}

type workerSessionsObservationForSessionProvider interface {
	WorkerSessionsObservationForSession(string) workersessions.ObservationService
}

func runtimeBindingForSession(
	factoryRuntime factoryruntime.Service,
	factorySessionID string,
) func(factoryruntime.RuntimeBinding) error {
	if strings.TrimSpace(factorySessionID) == "" {
		return nil
	}
	binder, ok := factoryRuntime.(interface {
		BindRuntime(string, factoryruntime.RuntimeBinding) error
	})
	if !ok {
		return nil
	}
	return func(binding factoryruntime.RuntimeBinding) error {
		return binder.BindRuntime(factorySessionID, binding)
	}
}

func openedWorkerSessionsObservation(
	factoryRuntime factoryruntime.Service,
	startup runtimeports.RuntimeInstance,
	effectiveFactorySessionID string,
) workersessions.ObservationService {
	// The process Factory Sessions root resolves its current selected runtime,
	// which is not necessarily the runtime being opened here. Prefer the
	// session's freshly assembled runtime instance so its observation decorator
	// retains the matching Worker Sessions registry and canonical event ledger.
	observationRuntime := factoryRuntime
	if startup != nil && startup.RuntimeService() != nil {
		observationRuntime = startup.RuntimeService()
	}
	if provider, ok := observationRuntime.(workerSessionsObservationForSessionProvider); ok {
		return provider.WorkerSessionsObservationForSession(effectiveFactorySessionID)
	}
	if provider, ok := observationRuntime.(workerSessionsObservationProvider); ok {
		return provider.WorkerSessionsObservation()
	}
	return nil
}
