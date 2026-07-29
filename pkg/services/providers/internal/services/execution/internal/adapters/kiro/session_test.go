package kiro_test

import (
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	kiro "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/kiro"
)

func TestSessionRefFromOutputNormalizesProviderSessions(t *testing.T) {
	t.Parallel()

	resume := &providers.SessionRef{
		Provider: providers.IDKiro,
		Kind:     providers.SessionIDKind,
		ID:       kiroResumedSession,
	}
	testCases := []struct {
		name   string
		stdout []byte
		stderr []byte
		resume *providers.SessionRef
		wantID string
	}{
		{
			name:   "new session",
			stdout: []byte("new session answer"),
			stderr: []byte(`{"event":"session.created","session_id":"` + kiroEmittedSession + `"}`),
			wantID: kiroEmittedSession,
		},
		{
			name:   "resumed session is preserved without emitted metadata",
			stdout: []byte("continued answer"),
			resume: resume,
			wantID: kiroResumedSession,
		},
		{
			name:   "emitted session updates resumed session",
			stdout: []byte("continued answer"),
			stderr: []byte(`{"session_id":"` + kiroEmittedSession + `"}`),
			resume: resume,
			wantID: kiroEmittedSession,
		},
		{
			name:   "absent session stays absent",
			stdout: []byte("answer without session metadata"),
		},
		{
			name:   "empty structured session stays absent",
			stdout: []byte("answer with empty session metadata"),
			stderr: []byte(`{"session_id":""}`),
		},
		{
			name:   "malformed structured session preserves resumed session",
			stdout: []byte("valid answer"),
			stderr: []byte(`{"session_id":"not-a-uuid"}`),
			resume: resume,
			wantID: kiroResumedSession,
		},
		{
			name:   "arbitrary text is not a session",
			stdout: []byte("answer mentioning session_id: " + kiroEmittedSession),
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			ref := kiro.SessionRefFromOutputForTest(
				testCase.stdout,
				testCase.stderr,
				testCase.resume,
			)
			if testCase.wantID == "" {
				if ref != nil {
					t.Fatalf("SessionRef = %#v, want nil", ref)
				}
				return
			}
			if ref == nil ||
				ref.Provider != providers.IDKiro ||
				ref.Kind != providers.SessionIDKind ||
				ref.ID != testCase.wantID {
				t.Fatalf("SessionRef = %#v, want kiro/session_id/%s", ref, testCase.wantID)
			}
		})
	}
}
