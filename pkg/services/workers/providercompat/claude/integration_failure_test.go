package claude

import (
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

func TestFailureFromExecuteErrorCarriesFailureSession(t *testing.T) {
	t.Parallel()

	failure := providers.ExecuteFailure{
		Kind:    providers.ExecuteFailureKindDependency,
		Message: "provider failed after opening a session",
		SessionRef: &providers.SessionRef{
			Provider: providers.IDClaude,
			Kind:     providers.SessionIDKind,
			ID:       "failure-session-claude",
		},
	}

	translated := failureFromExecuteError(failure, nil)
	session := translated.ProviderSession()
	if session == nil ||
		session.Provider() != string(providers.IDClaude) ||
		session.Kind() != providers.SessionIDKind ||
		session.ID() != "failure-session-claude" {
		t.Fatalf("translated failure session = %#v, want failure session", session)
	}
}
