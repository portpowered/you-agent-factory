package sessioncursor

import (
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestInvalidationFromSyncPreflight(t *testing.T) {
	t.Parallel()

	backendScopeID := "backend-a"
	logicalSessionKeyID := "folder::default::"
	factorySessionID := "session-live-001"
	streamGenerationID := "2026-06-26T00:00:00Z"

	baseResponse := factoryapi.FactorySessionSyncPreflightResponse{
		RequestedSessionId:  "~default",
		BackendScopeId:      &backendScopeID,
		LogicalSessionKeyId: &logicalSessionKeyID,
		FactorySessionId:    &factorySessionID,
		StreamGenerationId:  &streamGenerationID,
	}

	tests := []struct {
		name           string
		reasonCode     factoryapi.FactorySessionSyncPreflightReasonCode
		wantReason     InvalidationReason
		wantRecovery   RecoveryAction
		wantDiagnostic bool
	}{
		{
			name:           "cursor stale",
			reasonCode:     factoryapi.CursorStale,
			wantReason:     ReasonCursorStale,
			wantRecovery:   RecoveryReplayWithoutCursor,
			wantDiagnostic: true,
		},
		{
			name:           "session not found",
			reasonCode:     factoryapi.SessionNotFound,
			wantReason:     ReasonSessionNotFound,
			wantRecovery:   RecoveryShowExplicitRecovery,
			wantDiagnostic: true,
		},
		{
			name:           "logical session remap",
			reasonCode:     factoryapi.LogicalSessionRemap,
			wantReason:     ReasonSessionRemapped,
			wantRecovery:   RecoveryClearStreamDerivedState,
			wantDiagnostic: true,
		},
		{
			name:           "ok is not an invalidation",
			reasonCode:     factoryapi.Ok,
			wantDiagnostic: false,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			response := baseResponse
			response.ReasonCode = test.reasonCode

			diagnostic, ok := InvalidationFromSyncPreflight(response)
			if ok != test.wantDiagnostic {
				t.Fatalf("ok = %v, want %v", ok, test.wantDiagnostic)
			}
			if !test.wantDiagnostic {
				return
			}
			if diagnostic.Reason != test.wantReason {
				t.Fatalf("reason = %q, want %q", diagnostic.Reason, test.wantReason)
			}
			if diagnostic.RecoveryAction != test.wantRecovery {
				t.Fatalf("recoveryAction = %q, want %q", diagnostic.RecoveryAction, test.wantRecovery)
			}
			if diagnostic.Scope.BackendScopeID != backendScopeID {
				t.Fatalf("backendScopeID = %q, want %q", diagnostic.Scope.BackendScopeID, backendScopeID)
			}
			if diagnostic.Scope.LogicalSessionKeyID != logicalSessionKeyID {
				t.Fatalf("logicalSessionKeyID = %q, want %q", diagnostic.Scope.LogicalSessionKeyID, logicalSessionKeyID)
			}
			if diagnostic.Scope.FactorySessionID != factorySessionID {
				t.Fatalf("factorySessionID = %q, want %q", diagnostic.Scope.FactorySessionID, factorySessionID)
			}
			if diagnostic.Scope.StreamGenerationID != streamGenerationID {
				t.Fatalf("streamGenerationID = %q, want %q", diagnostic.Scope.StreamGenerationID, streamGenerationID)
			}
		})
	}
}

func TestClassifyIdentityMismatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		previous     IdentityScope
		current      IdentityScope
		wantReason   InvalidationReason
		wantMismatch bool
	}{
		{
			name: "backend scope changed",
			previous: IdentityScope{
				BackendScopeID:     "backend-a",
				FactorySessionID:   "session-a",
				StreamGenerationID: "stream-a",
			},
			current: IdentityScope{
				BackendScopeID:     "backend-b",
				FactorySessionID:   "session-a",
				StreamGenerationID: "stream-a",
			},
			wantReason:   ReasonBackendScopeChanged,
			wantMismatch: true,
		},
		{
			name: "session remapped",
			previous: IdentityScope{
				BackendScopeID:     "backend-a",
				FactorySessionID:   "session-a",
				StreamGenerationID: "stream-a",
			},
			current: IdentityScope{
				BackendScopeID:     "backend-a",
				FactorySessionID:   "session-b",
				StreamGenerationID: "stream-a",
			},
			wantReason:   ReasonSessionRemapped,
			wantMismatch: true,
		},
		{
			name: "stream generation changed",
			previous: IdentityScope{
				BackendScopeID:     "backend-a",
				FactorySessionID:   "session-a",
				StreamGenerationID: "stream-a",
			},
			current: IdentityScope{
				BackendScopeID:     "backend-a",
				FactorySessionID:   "session-a",
				StreamGenerationID: "stream-b",
			},
			wantReason:   ReasonStreamGenerationChanged,
			wantMismatch: true,
		},
		{
			name: "matching identity is not a mismatch",
			previous: IdentityScope{
				BackendScopeID:     "backend-a",
				FactorySessionID:   "session-a",
				StreamGenerationID: "stream-a",
			},
			current: IdentityScope{
				BackendScopeID:     "backend-a",
				FactorySessionID:   "session-a",
				StreamGenerationID: "stream-a",
			},
			wantMismatch: false,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			reason, ok := ClassifyIdentityMismatch(test.previous, test.current)
			if ok != test.wantMismatch {
				t.Fatalf("ok = %v, want %v", ok, test.wantMismatch)
			}
			if !test.wantMismatch {
				return
			}
			if reason != test.wantReason {
				t.Fatalf("reason = %q, want %q", reason, test.wantReason)
			}
		})
	}
}

func TestDiagnosticFields_OnlySafeScopeFields(t *testing.T) {
	t.Parallel()

	diagnostic := InvalidationDiagnostic{
		Reason:         ReasonStreamGenerationChanged,
		RecoveryAction: RecoveryClearStreamDerivedState,
		Scope: IdentityScope{
			BackendScopeID:      "backend-a",
			LogicalSessionKeyID: "folder::default::",
			FactorySessionID:    "session-a",
			StreamGenerationID:  "stream-b",
		},
		PreviousScope: &IdentityScope{
			BackendScopeID:      "backend-a",
			LogicalSessionKeyID: "folder::default::",
			FactorySessionID:    "session-a",
			StreamGenerationID:  "stream-a",
		},
		RequestedSessionID: "~default",
	}

	fields := DiagnosticFields(diagnostic)
	for _, forbidden := range []string{
		"provider",
		"token",
		"prompt",
		"payload",
		"workspace",
		"account",
	} {
		for key := range fields {
			if containsFold(key, forbidden) {
				t.Fatalf("field key %q contains forbidden fragment %q", key, forbidden)
			}
		}
	}

	if fields["reason"] != string(ReasonStreamGenerationChanged) {
		t.Fatalf("reason = %q, want %q", fields["reason"], ReasonStreamGenerationChanged)
	}
	if fields["recovery_action"] != string(RecoveryClearStreamDerivedState) {
		t.Fatalf("recovery_action = %q, want %q", fields["recovery_action"], RecoveryClearStreamDerivedState)
	}
	if fields["scope_stream_generation_id"] != "stream-b" {
		t.Fatalf("scope_stream_generation_id = %q, want stream-b", fields["scope_stream_generation_id"])
	}
	if fields["previous_scope_stream_generation_id"] != "stream-a" {
		t.Fatalf("previous_scope_stream_generation_id = %q, want stream-a", fields["previous_scope_stream_generation_id"])
	}
}

func TestSilentReplayRecoveryDiagnostic(t *testing.T) {
	t.Parallel()

	diagnostic := SilentReplayRecoveryDiagnostic(IdentityScope{
		BackendScopeID:     "backend-a",
		FactorySessionID:   "session-a",
		StreamGenerationID: "stream-a",
	}, "session-a")

	if diagnostic.Reason != ReasonCursorStale {
		t.Fatalf("reason = %q, want %q", diagnostic.Reason, ReasonCursorStale)
	}
	if diagnostic.RecoveryAction != RecoveryReplayWithoutCursor {
		t.Fatalf("recoveryAction = %q, want %q", diagnostic.RecoveryAction, RecoveryReplayWithoutCursor)
	}
}

func TestIdentityMismatchDiagnostic(t *testing.T) {
	t.Parallel()

	diagnostic, ok := IdentityMismatchDiagnostic(
		IdentityScope{
			BackendScopeID:     "backend-a",
			FactorySessionID:   "session-a",
			StreamGenerationID: "stream-a",
		},
		IdentityScope{
			BackendScopeID:     "backend-b",
			FactorySessionID:   "session-a",
			StreamGenerationID: "stream-a",
		},
		"session-a",
	)
	if !ok {
		t.Fatal("ok = false, want mismatch diagnostic")
	}
	if diagnostic.Reason != ReasonBackendScopeChanged {
		t.Fatalf("reason = %q, want %q", diagnostic.Reason, ReasonBackendScopeChanged)
	}
	if diagnostic.RecoveryAction != RecoveryClearStreamDerivedState {
		t.Fatalf("recovery = %q, want %q", diagnostic.RecoveryAction, RecoveryClearStreamDerivedState)
	}
}

func TestRecoveryActionForIdentityMismatch(t *testing.T) {
	t.Parallel()

	if got := RecoveryActionForIdentityMismatch(ReasonSessionRemapped); got != RecoveryClearStreamDerivedState {
		t.Fatalf("recovery = %q, want %q", got, RecoveryClearStreamDerivedState)
	}
	if got := RecoveryActionForIdentityMismatch(ReasonCursorStale); got != RecoveryClearCheckpoint {
		t.Fatalf("recovery = %q, want %q", got, RecoveryClearCheckpoint)
	}
}

type recordingLogger struct {
	message string
	fields  map[string]string
}

func (l *recordingLogger) Info(msg string, fields map[string]string) {
	l.message = msg
	l.fields = fields
}

type recordingMetrics struct {
	name   string
	labels map[string]string
}

func (m *recordingMetrics) RecordMetric(name string, labels map[string]string) {
	m.name = name
	m.labels = labels
}

func TestObserverRecord(t *testing.T) {
	t.Parallel()

	logger := &recordingLogger{}
	metrics := &recordingMetrics{}
	Observer{Logger: logger, Metrics: metrics}.Record(InvalidationDiagnostic{
		Reason:         ReasonCursorStale,
		RecoveryAction: RecoveryReplayWithoutCursor,
		Scope: IdentityScope{
			FactorySessionID: "session-a",
		},
		RequestedSessionID: "session-a",
	})

	if logger.message != "session persistence invalidation" {
		t.Fatalf("message = %q, want invalidation log message", logger.message)
	}
	if logger.fields["reason"] != string(ReasonCursorStale) {
		t.Fatalf("reason = %q, want %q", logger.fields["reason"], ReasonCursorStale)
	}
	if metrics.name != MetricSessionPersistenceInvalidation {
		t.Fatalf("metric = %q, want %q", metrics.name, MetricSessionPersistenceInvalidation)
	}
}

func TestFieldValueFromObservedLogs(t *testing.T) {
	t.Parallel()

	core, observed := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	logger.Info(
		"session persistence invalidation",
		zap.String("reason", string(ReasonCursorStale)),
		zap.String("recovery_action", string(RecoveryReplayWithoutCursor)),
	)

	if got := FieldValueFromObservedLogs(observed, "reason"); got != string(ReasonCursorStale) {
		t.Fatalf("reason = %q, want %q", got, ReasonCursorStale)
	}
}

func containsFold(value, fragment string) bool {
	return len(fragment) > 0 && len(value) >= len(fragment) &&
		(containsLower(value, fragment))
}

func containsLower(value, fragment string) bool {
	valueLower := make([]byte, len(value))
	fragmentLower := make([]byte, len(fragment))
	for index := range value {
		char := value[index]
		if char >= 'A' && char <= 'Z' {
			char += 'a' - 'A'
		}
		valueLower[index] = char
	}
	for index := range fragment {
		char := fragment[index]
		if char >= 'A' && char <= 'Z' {
			char += 'a' - 'A'
		}
		fragmentLower[index] = char
	}
	for index := 0; index+len(fragmentLower) <= len(valueLower); index++ {
		match := true
		for offset := range fragmentLower {
			if valueLower[index+offset] != fragmentLower[offset] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
