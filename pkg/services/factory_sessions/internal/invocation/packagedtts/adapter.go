// Package packagedtts adapts the built-in TTS Factory's invocation policy to
// the canonical Factory Session invocation service.
package packagedtts

import (
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	sessioninvocation "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/invocation"
)

// Observability is the narrow Definitions-owned packaged-TTS policy consumed
// by the Sessions invocation adapter. It lives at this consumer boundary so
// Sessions does not publish a second cross-service effect interface.
type Observability interface {
	IsPackagedTTSFactory(*interfaces.FactoryConfig) bool
	TTSBackendRuntimeLabel() string
	ClassifyTTSInvocationWait(interfaces.FactoryWorldState, string, bool) (interfaces.TTSInvocationWaitOutcome, *interfaces.TTSInvocationFailure)
	IsTTSModelNotReadyFailure(string) bool
}

// NewTelemetry supplies the packaged TTS telemetry descriptor.
func NewTelemetry(
	observability Observability,
	recordMetric func(sessioninvocation.SessionInvocationMetric),
	recordLog func(sessioninvocation.SessionInvocationLogRecord),
) sessioninvocation.SessionInvocationTelemetry {
	active := func(*interfaces.FactoryConfig) bool { return false }
	backend := ""
	if observability != nil {
		active = observability.IsPackagedTTSFactory
		backend = observability.TTSBackendRuntimeLabel()
	}
	return sessioninvocation.NewSessionInvocationTelemetry(
		recordMetric,
		recordLog,
		&sessioninvocation.PackagedInvocationTelemetry{
			Active: active, FactoryName: interfaces.PackagedTTSFactoryName, Backend: backend,
			AttemptsMetric: interfaces.TTSMetricPackagedFactoryAttempts, SuccessMetric: interfaces.TTSMetricPackagedFactorySuccess,
			FailureMetric: interfaces.TTSMetricPackagedFactoryFailure, NotReadyMetric: interfaces.TTSMetricPackagedFactoryNotReady,
			LoadingClass: interfaces.TTSFailureClassLoading, SuccessClass: interfaces.TTSFailureClassSuccess, NotReadyClass: interfaces.TTSFailureClassModelNotReady,
		},
	)
}

// SpecialCase supplies packaged TTS terminal classification.
type SpecialCase struct {
	observability Observability
}

func NewSpecialCase(observability Observability) SpecialCase {
	return SpecialCase{observability: observability}
}

func (s SpecialCase) Active(cfg *interfaces.FactoryConfig) bool {
	return s.observability != nil && s.observability.IsPackagedTTSFactory(cfg)
}

func (s SpecialCase) TerminalFailure(
	worldState interfaces.FactoryWorldState,
	requestID string,
) *sessioninvocation.SessionInvocationSpecialFailure {
	if s.observability == nil {
		return nil
	}
	_, failure := s.observability.ClassifyTTSInvocationWait(worldState, requestID, false)
	if failure == nil {
		return nil
	}
	return &sessioninvocation.SessionInvocationSpecialFailure{
		ErrorCode: failure.ErrorCode, Message: failure.Message, FailureClass: failure.FailureClass,
	}
}
