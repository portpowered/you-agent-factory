package restart_test

import (
	"fmt"
	"net/url"
	"runtime"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestFactorySessionRestartRemapsLiveIDToLogicalIdentity proves that after a
// simulated backend restart invalidates a previously live factorySessionID, the
// public sync-preflight surface remaps the stale selector through logical
// identity (backendScopeID + logicalSessionKeyID) to the current live session
// and stream generation for the same logical target.
func TestFactorySessionRestartRemapsLiveIDToLogicalIdentity(t *testing.T) {
	factoryDir := support.ScaffoldSingleStepFactory(t, "logical-identity-restart")
	home := t.TempDir()
	env := functionalHomeEnvironment(home)

	beforeRestart := startLogicalIdentityRestartServer(t, factoryDir, env)
	staleSession := support.GetDefaultSession(t, beforeRestart.URL())
	staleIdentity := requireLiveStreamIdentity(t, staleSession)
	if staleIdentity.FactorySessionID == "" {
		t.Fatal("pre-restart factorySessionID unexpectedly empty")
	}

	beforeRestart.Stop(t)

	afterRestart := startLogicalIdentityRestartServer(t, factoryDir, env)
	currentSession := support.GetDefaultSession(t, afterRestart.URL())
	currentIdentity := requireLiveStreamIdentity(t, currentSession)
	if currentIdentity.FactorySessionID == staleIdentity.FactorySessionID {
		t.Fatalf(
			"post-restart factorySessionID = %q, want distinct live id from pre-restart %q",
			currentIdentity.FactorySessionID,
			staleIdentity.FactorySessionID,
		)
	}
	if currentIdentity.LogicalSessionKeyID != staleIdentity.LogicalSessionKeyID {
		t.Fatalf(
			"post-restart logicalSessionKeyID = %q, want stable logical key %q",
			currentIdentity.LogicalSessionKeyID,
			staleIdentity.LogicalSessionKeyID,
		)
	}

	preflight := getFactorySessionSyncPreflight(
		t,
		afterRestart.URL(),
		staleIdentity.FactorySessionID,
		staleIdentity.BackendScopeID,
		staleIdentity.LogicalSessionKeyID,
	)
	if preflight.ReasonCode != factoryapi.LogicalSessionRemap {
		t.Fatalf("sync-preflight reasonCode = %q, want %q", preflight.ReasonCode, factoryapi.LogicalSessionRemap)
	}
	if preflight.CheckpointReusable {
		t.Fatal("sync-preflight checkpointReusable = true, want false for logical remap")
	}
	if preflight.FactorySessionId == nil || *preflight.FactorySessionId != currentIdentity.FactorySessionID {
		t.Fatalf(
			"sync-preflight factorySessionId = %#v, want current live id %q",
			preflight.FactorySessionId,
			currentIdentity.FactorySessionID,
		)
	}
	if preflight.StreamGenerationId == nil || *preflight.StreamGenerationId != currentIdentity.StreamGenerationID {
		t.Fatalf(
			"sync-preflight streamGenerationId = %#v, want current stream generation %q",
			preflight.StreamGenerationId,
			currentIdentity.StreamGenerationID,
		)
	}
	if preflight.BackendScopeId == nil || *preflight.BackendScopeId != currentIdentity.BackendScopeID {
		t.Fatalf(
			"sync-preflight backendScopeId = %#v, want current backend scope %q",
			preflight.BackendScopeId,
			currentIdentity.BackendScopeID,
		)
	}
	if preflight.LogicalSessionKeyId == nil ||
		*preflight.LogicalSessionKeyId != staleIdentity.LogicalSessionKeyID {
		t.Fatalf(
			"sync-preflight logicalSessionKeyId = %#v, want stable logical key %q",
			preflight.LogicalSessionKeyId,
			staleIdentity.LogicalSessionKeyID,
		)
	}
}

func startLogicalIdentityRestartServer(
	t *testing.T,
	factoryDir string,
	env []string,
) *support.FunctionalAPIServer {
	t.Helper()
	return support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
		Env:                       env,
	})
}

func requireLiveStreamIdentity(
	t *testing.T,
	session factoryapi.FactorySession,
) factoryapi.FactorySessionStreamIdentity {
	t.Helper()
	if session.Runtime.StreamIdentity == nil {
		t.Fatalf("session %q streamIdentity = nil, want public logical identity facts", session.Id)
	}
	identity := *session.Runtime.StreamIdentity
	if strings.TrimSpace(identity.BackendScopeID) == "" {
		t.Fatalf("streamIdentity.backendScopeID = %#v, want non-empty backend scope", identity)
	}
	if strings.TrimSpace(identity.LogicalSessionKeyID) == "" {
		t.Fatalf("streamIdentity.logicalSessionKeyID = %#v, want non-empty logical key", identity)
	}
	if strings.TrimSpace(identity.StreamGenerationID) == "" {
		t.Fatalf("streamIdentity.streamGenerationID = %#v, want non-empty stream generation", identity)
	}
	if strings.TrimSpace(identity.FactorySessionID) == "" {
		t.Fatalf("streamIdentity.factorySessionID = %#v, want non-empty live session id", identity)
	}
	return identity
}

func getFactorySessionSyncPreflight(
	t *testing.T,
	baseURL string,
	staleSessionID string,
	backendScopeID string,
	logicalSessionKeyID string,
) factoryapi.FactorySessionSyncPreflightResponse {
	t.Helper()

	endpoint := strings.TrimSuffix(baseURL, "/") +
		"/factory-sessions/" + url.PathEscape(staleSessionID) + "/sync-preflight"
	query := url.Values{}
	query.Set("backend_scope_id", backendScopeID)
	query.Set("logical_session_key_id", logicalSessionKeyID)
	return support.GetJSON[factoryapi.FactorySessionSyncPreflightResponse](
		t,
		endpoint+"?"+query.Encode(),
	)
}

func functionalHomeEnvironment(home string) []string {
	if runtime.GOOS == "windows" {
		return []string{"USERPROFILE=" + home}
	}
	if runtime.GOOS == "plan9" {
		return []string{"home=" + home}
	}
	return []string{fmt.Sprintf("HOME=%s", home)}
}
