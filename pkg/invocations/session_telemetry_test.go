package invocations

import (
	"context"
	"fmt"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

const (
	testPackagedFactory       = "@you/tts"
	testPackagedAttempts      = "packaged_factory.invocation.attempts"
	testPackagedSuccess       = "packaged_factory.invocation.success"
	testPackagedFailure       = "packaged_factory.invocation.failure"
	testPackagedNotReady      = "packaged_factory.invocation.not_ready"
	testPackagedLoadingClass  = "loading"
	testPackagedSuccessClass  = "success"
	testPackagedNotReadyClass = "model_not_ready"
)

type recordingSessionTelemetry struct {
	metrics []SessionInvocationMetric
	logs    []SessionInvocationLogRecord
}

func (r *recordingSessionTelemetry) telemetry() SessionInvocationTelemetry {
	return NewSessionInvocationTelemetry(SessionInvocationTelemetryDependencies{
		RecordMetric: func(metric SessionInvocationMetric) { r.metrics = append(r.metrics, metric) },
		RecordLog:    func(record SessionInvocationLogRecord) { r.logs = append(r.logs, record) },
		Packaged: &PackagedInvocationTelemetry{
			Active:      func(cfg *interfaces.FactoryConfig) bool { return cfg != nil && cfg.Name == testPackagedFactory },
			FactoryName: testPackagedFactory, Backend: "piper",
			AttemptsMetric: testPackagedAttempts, SuccessMetric: testPackagedSuccess,
			FailureMetric: testPackagedFailure, NotReadyMetric: testPackagedNotReady,
			LoadingClass: testPackagedLoadingClass, SuccessClass: testPackagedSuccessClass,
			NotReadyClass: testPackagedNotReadyClass,
		},
	})
}

func TestSessionOwnerTelemetry_RedactsSensitiveArgumentFailure(t *testing.T) {
	recording := &recordingSessionTelemetry{}
	cfg := sessionOwnerFactoryConfig()
	cfg.Name = "signature-factory"
	cfg.InvocationSignature = &interfaces.InvocationSignatureConfig{
		Parameters: []interfaces.InvocationParameterConfig{{
			Name: "apiKey", Sensitive: true, ValueMode: string(factoryapi.FactoryInvocationParameterValueModeRepeated),
			Bindings: []interfaces.InvocationParameterBindingConfig{{Kind: string(factoryapi.FactoryInvocationParameterBindingKindNamed)}},
		}},
		OutputContract: &interfaces.InvocationOutputContractConfig{PathParameter: "apiKey"},
	}
	owner := NewSessionOwner(SessionOwnerDependencies{
		FactoryConfig: func(string) (*interfaces.FactoryConfig, error) { return cfg, nil },
		SubmitWork: func(context.Context, string, interfaces.SubmitRequest) (interfaces.WorkRequestSubmitResult, error) {
			t.Fatal("SubmitWork called after interpolation failure")
			return interfaces.WorkRequestSubmitResult{}, nil
		},
		Observe: func(context.Context, string, SessionInvocationWaitInput) (SessionInvocationObservation, error) {
			return SessionInvocationObservation{}, nil
		},
		Telemetry: recording.telemetry(),
	})

	_, err := owner.InvokeFactorySession(context.Background(), "session-1", factoryapi.InvocationRequest{
		Args: &map[string]any{"apiKey": []any{"super-secret", "second-secret"}},
	})
	if err == nil {
		t.Fatal("InvokeFactorySession error = nil, want interpolation failure")
	}

	assertSessionMetricCount(t, recording.metrics, InvocationMetricNormalizationAttempts, 1)
	assertSessionMetricCount(t, recording.metrics, InvocationMetricNormalizationSuccess, 1)
	assertSessionMetricCount(t, recording.metrics, InvocationMetricInterpolationFailure, 1)
	log := singleSessionLog(t, recording.logs, "factory session invocation argument failure")
	assertSessionOwnerEqual(t, "argument redacted", log.Fields["argument_value_redacted"], any(true))
	assertSessionOwnerEqual(t, "argument value count", log.Fields["argument_value_count"], any(2))
	serialized := fmt.Sprint(recording.metrics, recording.logs)
	if strings.Contains(serialized, "super-secret") || strings.Contains(serialized, "second-secret") {
		t.Fatalf("telemetry leaked sensitive arguments: %s", serialized)
	}
}

func TestSessionOwnerTelemetry_PackagedSuccessEmitsEachOutcomeOnce(t *testing.T) {
	recording := &recordingSessionTelemetry{}
	cfg := packagedSessionOwnerConfig()
	observations := []SessionInvocationObservation{
		activeSessionInvocationObservation(),
		activeSessionInvocationObservation(),
		completedSessionInvocationObservation("request-1", "trace-1", "audio"),
	}
	owner := packagedSessionOwner(recording, cfg, observations, nil)

	result := invokePackagedSessionOwner(t, owner)
	assertSessionOwnerEqual(t, "status", result.Status, factoryapi.InvocationTerminalStatusCompleted)
	for _, metric := range []string{
		InvocationMetricAttempts, InvocationMetricFallbackPolicyUsed, testPackagedAttempts,
		InvocationMetricSuccess, InvocationMetricResultType, testPackagedSuccess,
	} {
		assertSessionMetricCount(t, recording.metrics, metric, 1)
	}
	for _, message := range []string{
		"packaged tts invocation submitted", "factory session invocation submitted",
		"packaged tts invocation loading", "packaged tts invocation completed",
		"factory session invocation completed",
	} {
		singleSessionLog(t, recording.logs, message)
	}
	completed := singleSessionLog(t, recording.logs, "packaged tts invocation completed")
	assertSessionOwnerEqual(t, "completed request ID", completed.Fields["request_id"], any("request-1"))
	assertSessionOwnerEqual(t, "completed trace ID", completed.Fields["trace_id"], any("trace-1"))
	assertSessionOwnerEqual(t, "completed readiness", completed.Fields["readiness_outcome"], any(testPackagedSuccessClass))
}

func TestSessionOwnerTelemetry_PackagedFailuresPreserveClassificationAndCounts(t *testing.T) {
	tests := []struct {
		name         string
		failureClass string
		errorCode    string
		wantNotReady int
	}{
		{name: "model not ready", failureClass: testPackagedNotReadyClass, errorCode: "INVOCATION_TTS_MODEL_NOT_READY", wantNotReady: 1},
		{name: "generation failed", failureClass: "generation_failed", errorCode: "INVOCATION_TTS_GENERATION_FAILED"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recording := &recordingSessionTelemetry{}
			failure := &SessionInvocationSpecialFailure{
				ErrorCode: tt.errorCode, Message: "safe packaged runtime failure", FailureClass: tt.failureClass,
			}
			owner := packagedSessionOwner(recording, packagedSessionOwnerConfig(), []SessionInvocationObservation{stoppedSessionInvocationObservation()}, failure)

			result := invokePackagedSessionOwner(t, owner)
			assertSessionOwnerEqual(t, "status", result.Status, factoryapi.InvocationTerminalStatusFailed)
			assertSessionOwnerEqual(t, "error code", result.ErrorCode, tt.errorCode)
			assertSessionOwnerEqual(t, "message", result.Message, failure.Message)
			for _, metric := range []string{InvocationMetricAttempts, testPackagedAttempts, InvocationMetricFailure, testPackagedFailure} {
				assertSessionMetricCount(t, recording.metrics, metric, 1)
			}
			assertSessionMetricCount(t, recording.metrics, testPackagedNotReady, tt.wantNotReady)
			failed := singleSessionLog(t, recording.logs, "packaged tts invocation failed")
			assertSessionOwnerEqual(t, "failure request ID", failed.Fields["request_id"], any("request-1"))
			assertSessionOwnerEqual(t, "failure trace ID", failed.Fields["trace_id"], any("trace-1"))
			assertSessionOwnerEqual(t, "failure class", failed.Fields["failure_class"], any(tt.failureClass))
		})
	}
}

type fixedPackagedSpecialCase struct {
	failure *SessionInvocationSpecialFailure
}

func (fixedPackagedSpecialCase) Active(*interfaces.FactoryConfig) bool { return true }

func (s fixedPackagedSpecialCase) TerminalFailure(interfaces.FactoryWorldState, string) *SessionInvocationSpecialFailure {
	return s.failure
}

func packagedSessionOwner(
	recording *recordingSessionTelemetry,
	cfg *interfaces.FactoryConfig,
	observations []SessionInvocationObservation,
	failure *SessionInvocationSpecialFailure,
) *SessionOwner {
	observationIndex := 0
	return NewSessionOwner(SessionOwnerDependencies{
		FactoryConfig: func(string) (*interfaces.FactoryConfig, error) { return cfg, nil },
		SubmitWork: func(context.Context, string, interfaces.SubmitRequest) (interfaces.WorkRequestSubmitResult, error) {
			return interfaces.WorkRequestSubmitResult{RequestID: "request-1", TraceID: "trace-1"}, nil
		},
		Observe: func(context.Context, string, SessionInvocationWaitInput) (SessionInvocationObservation, error) {
			observation := observations[observationIndex]
			if observationIndex < len(observations)-1 {
				observationIndex++
			}
			return observation, nil
		},
		WaitNext:    func(context.Context) error { return nil },
		Telemetry:   recording.telemetry(),
		SpecialCase: fixedPackagedSpecialCase{failure: failure},
	})
}

func packagedSessionOwnerConfig() *interfaces.FactoryConfig {
	cfg := sessionOwnerFactoryConfig()
	cfg.Name = testPackagedFactory
	return cfg
}

func invokePackagedSessionOwner(t *testing.T, owner *SessionOwner) FactoryInvocationResult {
	t.Helper()
	source := factoryapi.InvocationInputSourceKindText
	content := sessionOwnerTextContent(t, "hello")
	result, err := owner.InvokeFactorySession(context.Background(), "session-1", factoryapi.InvocationRequest{
		SourceKind: &source, Content: &content,
	})
	if err != nil {
		t.Fatalf("InvokeFactorySession: %v", err)
	}
	return result
}

func assertSessionMetricCount(t *testing.T, metrics []SessionInvocationMetric, name string, want int) {
	t.Helper()
	got := 0
	for _, metric := range metrics {
		if metric.Name == name {
			got++
		}
	}
	if got != want {
		t.Fatalf("metric %q count = %d, want %d; metrics = %#v", name, got, want, metrics)
	}
}

func singleSessionLog(t *testing.T, logs []SessionInvocationLogRecord, message string) SessionInvocationLogRecord {
	t.Helper()
	matches := make([]SessionInvocationLogRecord, 0, 1)
	for _, record := range logs {
		if record.Message == message {
			matches = append(matches, record)
		}
	}
	if len(matches) != 1 {
		t.Fatalf("log %q count = %d, want 1; logs = %#v", message, len(matches), logs)
	}
	return matches[0]
}
