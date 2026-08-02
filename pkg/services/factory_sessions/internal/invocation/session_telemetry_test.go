package invocation

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
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
	return NewSessionInvocationTelemetry(
		func(metric SessionInvocationMetric) { r.metrics = append(r.metrics, metric) },
		func(record SessionInvocationLogRecord) { r.logs = append(r.logs, record) },
		&PackagedInvocationTelemetry{
			Active:      func(cfg *interfaces.FactoryConfig) bool { return cfg != nil && cfg.Name == testPackagedFactory },
			FactoryName: testPackagedFactory, Backend: "piper",
			AttemptsMetric: testPackagedAttempts, SuccessMetric: testPackagedSuccess,
			FailureMetric: testPackagedFailure, NotReadyMetric: testPackagedNotReady,
			LoadingClass: testPackagedLoadingClass, SuccessClass: testPackagedSuccessClass,
			NotReadyClass: testPackagedNotReadyClass,
		},
	)
}

func TestSessionOwnerTelemetry_NormalizationFailurePreservesStableLabels(t *testing.T) {
	recording := &recordingSessionTelemetry{}
	cfg := sessionOwnerSignatureFactoryConfig()
	cfg.Name = "signature-factory"
	cfg.Project = "telemetry-project"
	owner := newTestSessionOwner(sessionOwnerFixture{
		FactoryConfig: func(string) (*interfaces.FactoryConfig, error) { return cfg, nil },
		SubmitWork: func(context.Context, string, work.SubmitRequest) (work.WorkRequestSubmitResult, error) {
			t.Fatal("SubmitWork called after normalization failure")
			return work.WorkRequestSubmitResult{}, nil
		},
		Observe: func(context.Context, string, SessionInvocationWaitInput) (SessionInvocationObservation, error) {
			t.Fatal("Observe called after normalization failure")
			return SessionInvocationObservation{}, nil
		},
		Telemetry: recording.telemetry(),
	})

	_, err := owner.InvokeFactorySession(context.Background(), "session-1", sessionOwnerInvocationRequest(factoryapi.InvocationRequest{
		Args: &map[string]any{},
	}))
	var argumentErr *work.ArgumentError
	if !errors.As(err, &argumentErr) || argumentErr.Code != work.ArgumentErrorCodeMissingRequiredInput {
		t.Fatalf("error = %v, want %s", err, work.ArgumentErrorCodeMissingRequiredInput)
	}

	baseLabels := map[string]string{
		"input_source":    string(StructuredArgumentsInputSource),
		"factory_name":    cfg.Name,
		"factory_project": cfg.Project,
		"signature_hash":  work.InvocationSignatureHash(cfg.InvocationSignature),
	}
	assertSingleSessionMetric(t, recording.metrics, InvocationMetricNormalizationAttempts, baseLabels)
	failureLabels := cloneMetricLabels(baseLabels)
	failureLabels["error_code"] = string(work.ArgumentErrorCodeMissingRequiredInput)
	assertSingleSessionMetric(t, recording.metrics, InvocationMetricNormalizationFailure, failureLabels)
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
	owner := newTestSessionOwner(sessionOwnerFixture{
		FactoryConfig: func(string) (*interfaces.FactoryConfig, error) { return cfg, nil },
		SubmitWork: func(context.Context, string, work.SubmitRequest) (work.WorkRequestSubmitResult, error) {
			t.Fatal("SubmitWork called after interpolation failure")
			return work.WorkRequestSubmitResult{}, nil
		},
		Observe: func(context.Context, string, SessionInvocationWaitInput) (SessionInvocationObservation, error) {
			return SessionInvocationObservation{}, nil
		},
		Telemetry:         recording.telemetry(),
		ResolveDefinition: rejectingInvocationInterpolation("apiKey"),
	})

	_, err := owner.InvokeFactorySession(context.Background(), "session-1", sessionOwnerInvocationRequest(factoryapi.InvocationRequest{
		Args: &map[string]any{"apiKey": []any{"super-secret", "second-secret"}},
	}))
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

func TestSessionOwnerTelemetry_DefaultWorkTypeFailureIsReportedOnce(t *testing.T) {
	recording := &recordingSessionTelemetry{}
	cfg := sessionOwnerFactoryConfig()
	cfg.Name = "general-factory"
	cfg.Project = "telemetry-project"
	cfg.WorkTypes = nil
	owner := newTestSessionOwner(sessionOwnerFixture{
		FactoryConfig: func(string) (*interfaces.FactoryConfig, error) { return cfg, nil },
		SubmitWork: func(context.Context, string, work.SubmitRequest) (work.WorkRequestSubmitResult, error) {
			t.Fatal("SubmitWork called without a default handling Work type")
			return work.WorkRequestSubmitResult{}, nil
		},
		Observe: func(context.Context, string, SessionInvocationWaitInput) (SessionInvocationObservation, error) {
			t.Fatal("Observe called without submitted Work")
			return SessionInvocationObservation{}, nil
		},
		Telemetry: recording.telemetry(),
		ResolveDefinition: rejectingInvocationWorkType{
			err: errors.New("expected exactly one default Work type"),
		}.ResolveDefinition,
	})

	source := factoryapi.InvocationInputSourceKindText
	content := sessionOwnerTextContent(t, "do not log this payload")
	_, err := owner.InvokeFactorySession(context.Background(), "session-1", sessionOwnerInvocationRequest(factoryapi.InvocationRequest{
		SourceKind: &source,
		Content:    &content,
	}))
	if err == nil || !strings.Contains(err.Error(), "resolve invocation work type:") {
		t.Fatalf("InvokeFactorySession error = %v, want wrapped Work-type resolution error", err)
	}

	labels := map[string]string{
		"input_source":    string(work.InputSourceLabel(work.ArgumentSourceKindCompatibilityContent)),
		"factory_name":    cfg.Name,
		"factory_project": cfg.Project,
	}
	assertSingleSessionMetric(t, recording.metrics, InvocationMetricFailure, labels)
	failed := singleSessionLog(t, recording.logs, "factory session invocation failed")
	assertSessionOwnerEqual(t, "session ID", failed.Fields["session_id"], any("session-1"))
	assertSessionOwnerEqual(t, "status", failed.Fields["status"], any(string(factoryapi.InvocationTerminalStatusFailed)))
	assertSessionOwnerEqual(t, "error code", failed.Fields["error_code"], any(string(factoryapi.INVOCATIONRUNTIMEFAILURE)))
	assertSessionOwnerEqual(t, "failure class", failed.Fields["failure_class"], any("runtime_failure"))
	if failed.Error != err {
		t.Fatalf("logged error = %v, want returned wrapped error %v", failed.Error, err)
	}
	if strings.Contains(fmt.Sprint(recording.metrics, recording.logs), "do not log this payload") {
		t.Fatal("telemetry leaked invocation payload")
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
	assertSessionOwnerEqual(t, "status", result.Status, interfaces.InvocationTerminalStatusCompleted)
	for _, metric := range []string{
		InvocationMetricAttempts, InvocationMetricFallbackPolicyUsed, testPackagedAttempts,
		InvocationMetricSuccess, InvocationMetricResultType, testPackagedSuccess,
	} {
		assertSessionMetricCount(t, recording.metrics, metric, 1)
	}
	generalLabels := map[string]string{
		"input_source": string(work.InputSourceLabel(work.ArgumentSourceKindCompatibilityContent)),
		"factory_name": testPackagedFactory,
	}
	assertSingleSessionMetric(t, recording.metrics, InvocationMetricAttempts, generalLabels)
	assertSingleSessionMetric(t, recording.metrics, InvocationMetricSuccess, generalLabels)
	resultLabels := cloneMetricLabels(generalLabels)
	resultLabels["result_type"] = "text"
	assertSingleSessionMetric(t, recording.metrics, InvocationMetricResultType, resultLabels)
	packagedLabels := map[string]string{
		"input_source":     string(work.InputSourceLabel(work.ArgumentSourceKindCompatibilityContent)),
		"packaged_factory": testPackagedFactory,
	}
	assertSingleSessionMetric(t, recording.metrics, testPackagedAttempts, packagedLabels)
	packagedSuccessLabels := cloneMetricLabels(packagedLabels)
	packagedSuccessLabels["readiness_outcome"] = testPackagedSuccessClass
	assertSingleSessionMetric(t, recording.metrics, testPackagedSuccess, packagedSuccessLabels)
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
			assertSessionOwnerEqual(t, "status", result.Status, interfaces.InvocationTerminalStatusFailed)
			assertSessionOwnerEqual(t, "error code", result.ErrorCode, tt.errorCode)
			assertSessionOwnerEqual(t, "message", result.Message, failure.Message)
			for _, metric := range []string{InvocationMetricAttempts, testPackagedAttempts, InvocationMetricFailure, testPackagedFailure} {
				assertSessionMetricCount(t, recording.metrics, metric, 1)
			}
			assertSessionMetricCount(t, recording.metrics, testPackagedNotReady, tt.wantNotReady)
			packagedLabels := map[string]string{
				"input_source":     string(work.InputSourceLabel(work.ArgumentSourceKindCompatibilityContent)),
				"packaged_factory": testPackagedFactory,
			}
			failureLabels := cloneMetricLabels(packagedLabels)
			failureLabels["failure_class"] = tt.failureClass
			assertSingleSessionMetric(t, recording.metrics, testPackagedFailure, failureLabels)
			if tt.wantNotReady == 1 {
				assertSingleSessionMetric(t, recording.metrics, testPackagedNotReady, packagedLabels)
			}
			assertSessionLogCount(t, recording.logs, "factory session invocation failed", 0)
			assertSessionLogCount(t, recording.logs, "packaged tts invocation failed", 1)
			failed := singleSessionLog(t, recording.logs, "packaged tts invocation failed")
			assertSessionOwnerEqual(t, "failure request ID", failed.Fields["request_id"], any("request-1"))
			assertSessionOwnerEqual(t, "failure trace ID", failed.Fields["trace_id"], any("trace-1"))
			assertSessionOwnerEqual(t, "failure class", failed.Fields["failure_class"], any(tt.failureClass))
		})
	}
}

func TestSessionOwnerTelemetry_WaitFailuresPreserveCorrelationAndClassification(t *testing.T) {
	tests := []struct {
		name         string
		observation  SessionInvocationObservation
		waitErr      error
		wantStatus   interfaces.InvocationTerminalStatus
		wantCode     string
		failureClass string
	}{
		{
			name: "timeout", observation: activeSessionInvocationObservation(), waitErr: context.DeadlineExceeded,
			wantStatus: interfaces.InvocationTerminalStatusTimedOut, wantCode: string(interfaces.InvocationErrorCodeTimedOut), failureClass: "timeout",
		},
		{
			name: "cancellation", observation: activeSessionInvocationObservation(), waitErr: context.Canceled,
			wantStatus: interfaces.InvocationTerminalStatusCanceled, wantCode: string(interfaces.InvocationErrorCodeCanceled), failureClass: "cancellation",
		},
		{
			name: "primary result failure", observation: classifiedObservation(work.PrimaryResultErrorCodeBlocked, "blocked"),
			wantStatus: interfaces.InvocationTerminalStatusFailed, wantCode: string(work.PrimaryResultErrorCodeBlocked), failureClass: "blocked",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recording := &recordingSessionTelemetry{}
			cfg := sessionOwnerFactoryConfig()
			cfg.Name = "general-factory"
			owner := newTestSessionOwner(sessionOwnerFixture{
				Observe: func(context.Context, string, SessionInvocationWaitInput) (SessionInvocationObservation, error) {
					return tt.observation, nil
				},
				WaitNext:  func(context.Context) error { return tt.waitErr },
				Telemetry: recording.telemetry(),
			})
			input := sessionWaitInput(nil)
			input.FactoryConfig = cfg
			input.InputSource = work.InputSourceLabel(work.ArgumentSourceKindCompatibilityContent)
			result, err := owner.waitForResult(context.Background(), "session-1", input)
			if err != nil {
				t.Fatalf("waitForResult: %v", err)
			}
			assertSessionOwnerEqual(t, "status", result.Status, tt.wantStatus)
			assertSessionOwnerEqual(t, "error code", result.ErrorCode, tt.wantCode)
			assertSingleSessionMetric(t, recording.metrics, InvocationMetricFailure, map[string]string{
				"input_source": string(work.InputSourceLabel(work.ArgumentSourceKindCompatibilityContent)),
				"factory_name": cfg.Name,
			})
			failed := singleSessionLog(t, recording.logs, "factory session invocation failed")
			assertSessionOwnerEqual(t, "request ID", failed.Fields["request_id"], any("request-1"))
			assertSessionOwnerEqual(t, "trace ID", failed.Fields["trace_id"], any("trace-1"))
			assertSessionOwnerEqual(t, "status field", failed.Fields["status"], any(string(tt.wantStatus)))
			assertSessionOwnerEqual(t, "error code field", failed.Fields["error_code"], any(tt.wantCode))
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
	return newTestSessionOwner(sessionOwnerFixture{
		FactoryConfig: func(string) (*interfaces.FactoryConfig, error) { return cfg, nil },
		SubmitWork: func(context.Context, string, work.SubmitRequest) (work.WorkRequestSubmitResult, error) {
			return work.WorkRequestSubmitResult{RequestID: "request-1", TraceID: "trace-1"}, nil
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
	result, err := owner.InvokeFactorySession(context.Background(), "session-1", sessionOwnerInvocationRequest(factoryapi.InvocationRequest{
		SourceKind: &source, Content: &content,
	}))
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

func assertSingleSessionMetric(t *testing.T, metrics []SessionInvocationMetric, name string, wantLabels map[string]string) {
	t.Helper()
	matches := make([]SessionInvocationMetric, 0, 1)
	for _, metric := range metrics {
		if metric.Name == name {
			matches = append(matches, metric)
		}
	}
	if len(matches) != 1 {
		t.Fatalf("metric %q count = %d, want 1; metrics = %#v", name, len(matches), metrics)
	}
	if !reflect.DeepEqual(matches[0].Labels, wantLabels) {
		t.Fatalf("metric %q labels = %#v, want %#v", name, matches[0].Labels, wantLabels)
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

func assertSessionLogCount(t *testing.T, logs []SessionInvocationLogRecord, message string, want int) {
	t.Helper()
	got := 0
	for _, record := range logs {
		if record.Message == message {
			got++
		}
	}
	if got != want {
		t.Fatalf("log %q count = %d, want %d; logs = %#v", message, got, want, logs)
	}
}
