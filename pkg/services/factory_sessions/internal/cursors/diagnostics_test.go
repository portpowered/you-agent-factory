package cursors

import "testing"

func TestInvalidationFromPreflightClassifiesRecovery(t *testing.T) {
	tests := []struct {
		reason       PreflightReason
		wantReason   InvalidationReason
		wantRecovery RecoveryAction
		wantOK       bool
	}{
		{PreflightCursorStale, ReasonCursorStale, RecoveryReplayWithoutCursor, true},
		{PreflightSessionNotFound, ReasonSessionNotFound, RecoveryShowExplicitRecovery, true},
		{PreflightSessionRemapped, ReasonSessionRemapped, RecoveryClearStreamDerivedState, true},
		{"future", "", "", false},
	}
	for _, test := range tests {
		diagnostic, ok := InvalidationFromPreflight(PreflightResult{
			Reason: test.reason,
			Scope: IdentityScope{
				BackendScopeID: " backend-a ", FactorySessionID: " session-a ", StreamGenerationID: " generation-a ",
			},
			RequestedSessionID: " ~default ",
		})
		if ok != test.wantOK || diagnostic.Reason != test.wantReason || diagnostic.RecoveryAction != test.wantRecovery {
			t.Fatalf("InvalidationFromPreflight(%q) = %#v, %v", test.reason, diagnostic, ok)
		}
		if ok && (diagnostic.Scope.BackendScopeID != "backend-a" || diagnostic.RequestedSessionID != "~default") {
			t.Fatalf("normalized diagnostic = %#v", diagnostic)
		}
	}
}

func TestIdentityMismatchClassificationAndDiagnostics(t *testing.T) {
	base := IdentityScope{BackendScopeID: "backend-a", FactorySessionID: "session-a", StreamGenerationID: "generation-a"}
	tests := []struct {
		name     string
		current  IdentityScope
		want     InvalidationReason
		mismatch bool
	}{
		{"same", base, "", false},
		{"backend", IdentityScope{BackendScopeID: "backend-b", FactorySessionID: "session-a", StreamGenerationID: "generation-a"}, ReasonBackendScopeChanged, true},
		{"session", IdentityScope{BackendScopeID: "backend-a", FactorySessionID: "session-b", StreamGenerationID: "generation-a"}, ReasonSessionRemapped, true},
		{"generation", IdentityScope{BackendScopeID: "backend-a", FactorySessionID: "session-a", StreamGenerationID: "generation-b"}, ReasonStreamGenerationChanged, true},
	}
	for _, test := range tests {
		reason, ok := ClassifyIdentityMismatch(base, test.current)
		if ok != test.mismatch || reason != test.want {
			t.Fatalf("%s classification = %q, %v", test.name, reason, ok)
		}
		diagnostic, diagnosticOK := IdentityMismatchDiagnostic(base, test.current, " session-a ")
		if diagnosticOK != test.mismatch {
			t.Fatalf("%s diagnostic ok = %v", test.name, diagnosticOK)
		}
		if diagnosticOK && (diagnostic.PreviousScope == nil || diagnostic.RecoveryAction != RecoveryClearStreamDerivedState) {
			t.Fatalf("%s diagnostic = %#v", test.name, diagnostic)
		}
	}
}

func TestIdentityScopeComparisonsDistinguishBackendAndStreamChanges(t *testing.T) {
	previous := IdentityScope{
		BackendScopeID:      "runtime-instance-a",
		LogicalSessionKeyID: "/workspace/root::default::",
		FactorySessionID:    "live-session-default-001",
		StreamGenerationID:  "runtime-instance-a::live-session-default-001",
	}
	tests := []struct {
		name    string
		current IdentityScope
		want    InvalidationReason
	}{
		{
			name: "backend-scope-change",
			current: IdentityScope{
				BackendScopeID:      "runtime-instance-b",
				LogicalSessionKeyID: "/workspace/root::default::",
				FactorySessionID:    "live-session-default-001",
				StreamGenerationID:  "runtime-instance-b::live-session-default-001",
			},
			want: ReasonBackendScopeChanged,
		},
		{
			name: "stream-generation-only-change",
			current: IdentityScope{
				BackendScopeID:      "runtime-instance-a",
				LogicalSessionKeyID: "/workspace/root::default::",
				FactorySessionID:    "live-session-default-001",
				StreamGenerationID:  "runtime-instance-a::live-session-default-001-v2",
			},
			want: ReasonStreamGenerationChanged,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reason, ok := ClassifyIdentityMismatch(previous, test.current)
			if !ok {
				t.Fatal("ClassifyIdentityMismatch = false, want mismatch")
			}
			if reason != test.want {
				t.Fatalf("classification = %q, want %q", reason, test.want)
			}
		})
	}
}

func TestRecoveryDiagnosticHelpers(t *testing.T) {
	if got := RecoveryActionForIdentityMismatch(ReasonCursorStale); got != RecoveryClearCheckpoint {
		t.Fatalf("stale recovery = %q", got)
	}
	if got := RecoveryActionForIdentityMismatch(ReasonSessionRemapped); got != RecoveryClearStreamDerivedState {
		t.Fatalf("remap recovery = %q", got)
	}
	diagnostic := SilentReplayRecoveryDiagnostic(IdentityScope{FactorySessionID: " session-a "}, " session-a ")
	if diagnostic.Reason != ReasonCursorStale || diagnostic.Scope.FactorySessionID != "session-a" || diagnostic.RequestedSessionID != "session-a" {
		t.Fatalf("silent recovery diagnostic = %#v", diagnostic)
	}
}
