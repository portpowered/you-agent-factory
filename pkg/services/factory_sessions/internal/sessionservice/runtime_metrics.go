// Session metrics are domain contracts consumed by composition adapters.
package service

import (
	"strings"

	factorymetrics "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	sessioninvocation "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/invocation"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

func retainedRuntimeMetricsSessionIDs(currentID, sourceID string) []string {
	ids := make([]string, 0, 2)
	for _, candidate := range []string{currentID, sourceID} {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		alreadyRetained := false
		for _, retained := range ids {
			if retained == candidate {
				alreadyRetained = true
				break
			}
		}
		if !alreadyRetained {
			ids = append(ids, candidate)
		}
	}
	return ids
}

// Runtime metric names emitted by the transport runtime host.
const (
	RuntimeMetricLifecycleStarted               = factorymetrics.RuntimeLifecycleStarted
	RuntimeMetricLifecycleStopped               = factorymetrics.RuntimeLifecycleStopped
	RuntimeMetricStateActive                    = factorymetrics.RuntimeStateActive
	RuntimeMetricStateIdle                      = factorymetrics.RuntimeStateIdle
	RuntimeMetricStatePaused                    = factorymetrics.RuntimeStatePaused
	RuntimeMetricStateFailed                    = factorymetrics.RuntimeStateFailed
	RuntimeMetricQueueInFlight                  = factorymetrics.RuntimeQueueInFlight
	RuntimeMetricQueueSubmissionCount           = factorymetrics.RuntimeQueueSubmissionCount
	RuntimeMetricDispatchStarted                = factorymetrics.RuntimeDispatchStarted
	RuntimeMetricDispatchComplete               = factorymetrics.RuntimeDispatchComplete
	RuntimeMetricDispatchDuration               = factorymetrics.RuntimeDispatchDuration
	RuntimeMetricDispatchRetries                = factorymetrics.RuntimeDispatchRetries
	RuntimeMetricDispatchCost                   = factorymetrics.RuntimeDispatchCost
	RuntimeMetricProviderRequest                = factorymetrics.RuntimeProviderRequest
	RuntimeMetricProviderComplete               = factorymetrics.RuntimeProviderComplete
	RuntimeMetricProviderFailed                 = factorymetrics.RuntimeProviderFailed
	RuntimeMetricProviderDuration               = factorymetrics.RuntimeProviderDuration
	RuntimeMetricProviderInputTok               = factorymetrics.RuntimeProviderInputTokens
	RuntimeMetricProviderOutputTok              = factorymetrics.RuntimeProviderOutputTokens
	RuntimeMetricProviderCost                   = factorymetrics.RuntimeProviderCost
	RuntimeMetricScriptStarted                  = factorymetrics.RuntimeScriptStarted
	RuntimeMetricScriptComplete                 = factorymetrics.RuntimeScriptComplete
	RuntimeMetricScriptDuration                 = factorymetrics.RuntimeScriptDuration
	RuntimeMetricScriptTimedOut                 = factorymetrics.RuntimeScriptTimedOut
	RuntimeMetricScriptFailed                   = factorymetrics.RuntimeScriptFailed
	RuntimeMetricSessionResponseStreamPublished = factorymetrics.RuntimeSessionResponseStreamPublished
	RuntimeMetricSessionResponseStreamCompacted = factorymetrics.RuntimeSessionResponseStreamCompacted
	RuntimeMetricSessionResponseStreamDegraded  = factorymetrics.RuntimeSessionResponseStreamDegraded
	RuntimeMetricLifecycleControl               = factorymetrics.RuntimeLifecycleControl
)

const (
	runtimeMetricSessionResponseStreamPublished = RuntimeMetricSessionResponseStreamPublished
	runtimeMetricSessionResponseStreamCompacted = RuntimeMetricSessionResponseStreamCompacted
	runtimeMetricSessionResponseStreamDegraded  = RuntimeMetricSessionResponseStreamDegraded
	runtimeMetricLifecycleControl               = RuntimeMetricLifecycleControl
)

const (
	ModelPullMetricAttempts      = "managed_runtime.pull.attempts"
	ModelPullMetricSuccess       = "managed_runtime.pull.success"
	ModelPullMetricFailure       = "managed_runtime.pull.failure"
	ModelPullMetricSourceFailure = "managed_runtime.pull.source_failure"
)

const (
	InvocationMetricNormalizationAttempts = sessioninvocation.InvocationMetricNormalizationAttempts
	InvocationMetricNormalizationSuccess  = sessioninvocation.InvocationMetricNormalizationSuccess
	InvocationMetricNormalizationFailure  = sessioninvocation.InvocationMetricNormalizationFailure
	InvocationMetricInterpolationFailure  = sessioninvocation.InvocationMetricInterpolationFailure
	InvocationMetricAttempts              = sessioninvocation.InvocationMetricAttempts
	InvocationMetricSuccess               = sessioninvocation.InvocationMetricSuccess
	InvocationMetricFailure               = sessioninvocation.InvocationMetricFailure
	InvocationMetricUnresolvedPrimary     = sessioninvocation.InvocationMetricUnresolvedPrimary
	InvocationMetricFallbackPolicyUsed    = sessioninvocation.InvocationMetricFallbackPolicyUsed
	InvocationMetricResultType            = sessioninvocation.InvocationMetricResultType
)

const (
	modelPullMetricAttempts      = ModelPullMetricAttempts
	modelPullMetricSuccess       = ModelPullMetricSuccess
	modelPullMetricFailure       = ModelPullMetricFailure
	modelPullMetricSourceFailure = ModelPullMetricSourceFailure
)

type runtimeReadMetricsAdapter struct {
	recorder roles.InvocationMetricsRecorder
}

func (adapter runtimeReadMetricsAdapter) record(metric recordings.RuntimeReadMetric) {
	if adapter.recorder == nil {
		return
	}
	labels := make(map[string]string, len(metric.Labels))
	for key, value := range metric.Labels {
		labels[key] = value
	}
	adapter.recorder.RecordInvocationMetric(factorysessions.InvocationMetric{
		Name: metric.Name, Labels: labels,
	})
}

func (fs *SessionRuntime) bindRuntimeReadMetrics(bundle factoryRuntimeBundle) {
	if fs == nil || bundle == nil {
		return
	}
	ledger := runtimeLedgerForReadMetrics(bundle)
	binder, ok := ledger.(interface {
		SetRuntimeReadMetricsRecorder(recordings.RuntimeReadMetricsRecorder)
	})
	if !ok || binder == nil {
		return
	}
	binder.SetRuntimeReadMetricsRecorder(runtimeReadMetricsAdapter{
		recorder: fs.invocationMetricsRecorder,
	}.record)
}

// runtimeLedgerForReadMetrics keeps optional telemetry compatible with legacy
// RuntimeRecord test doubles that promote RecordingLedger from a nil embed.
func runtimeLedgerForReadMetrics(bundle factoryRuntimeBundle) (ledger recordings.Ledger) {
	defer func() {
		if recover() != nil {
			ledger = nil
		}
	}()
	return bundle.RecordingLedger()
}
