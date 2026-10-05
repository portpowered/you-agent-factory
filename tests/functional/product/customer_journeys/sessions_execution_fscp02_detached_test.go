package customer_journeys_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestFSCP02DetachedCanonicalProcessBoundary proves the process-owned detached
// capability is reachable through the same root.BuildProcess/Process.Execute
// construction used by customers. It records the excluded assembly boundary
// for valid live work while proving field-scoped validation and the
// process-composed durable canonical owner.
func TestFSCP02DetachedCanonicalProcessBoundary(t *testing.T) {
	t.Parallel()
	acquireExecutionFixtureSlot(t)

	factoryDir := support.ScaffoldSingleStepFactory(t, "fscp02-detached-live")
	home := t.TempDir()
	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{
		BrowserOpener:         func(context.Context, string) error { return nil },
		ProviderCommandRunner: support.NewStaticSuccessCommandRunner("fscp02 durable owner probe COMPLETE"),
	})
	if err != nil {
		t.Fatalf("root.BuildProcess() error = %v", err)
	}
	support.CleanupProcess(t, process)

	inputs := support.FakeInputs(t.Context(), []string{"you", "--help"})
	inputs.Input.Env = isolatedEnvironment(home)
	inputs.Input.WorkingDirectory = factoryDir
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(--help) error = %v\nstdout=%s\nstderr=%s", err, inputs.Stdout(), inputs.Stderr())
	}
	capability := process.FactorySessions()
	if capability == nil {
		t.Fatal("root process returned no factory sessions capability")
	}
	detached, ok := capability.FactorySessions().(factorysessions.Service)
	if !ok || detached == nil {
		t.Fatalf("factory sessions capability type = %T, want factorysessions.Service", capability.FactorySessions())
	}
	assertDetachedLiveBoundary(t, detached, factoryDir)
	assertProcessComposedDurableOwner(t, detached, factoryDir, home)
}

func assertDetachedLiveBoundary(t *testing.T, detached factorysessions.Service, factoryDir string) {
	t.Helper()
	started, err := detached.Start(t.Context(), factorysessions.SessionStartRequest{
		Mode:       factorysessions.SessionOperationModeLive,
		FolderPath: factoryDir,
	})
	if err != nil || started.SessionID == "" || started.Mode != factorysessions.SessionOperationModeLive {
		t.Fatalf("canonical Start(live) = %#v, error = %v, want live session", started, err)
	}
	got, err := detached.Get(t.Context(), factorysessions.SessionGetRequest{SessionID: started.SessionID, Mode: factorysessions.SessionOperationModeLive})
	if err != nil || got.Session.SessionID != started.SessionID {
		t.Fatalf("canonical Get(live) = %#v, error = %v", got, err)
	}
	listed, err := detached.List(t.Context(), factorysessions.SessionListRequest{Mode: factorysessions.SessionOperationModeLive})
	if err != nil {
		t.Fatalf("canonical List(live) error = %v", err)
	}
	found := false
	for _, session := range listed.Sessions {
		found = found || session.SessionID == started.SessionID
	}
	if !found {
		t.Fatalf("canonical List(live) omitted %q: %#v", started.SessionID, listed.Sessions)
	}
	closed, err := detached.Control(t.Context(), factorysessions.SessionControlRequest{SessionID: started.SessionID, Mode: factorysessions.SessionOperationModeLive, Operation: factorysessions.SessionControlClose})
	if err != nil || !closed.Closed {
		t.Fatalf("canonical Control(CLOSE) = %#v, error = %v", closed, err)
	}

	_, err = detached.Start(t.Context(), factorysessions.SessionStartRequest{Mode: factorysessions.SessionOperationMode("invalid")})
	var requestErr *factorysessions.DetachedRequestError
	if !errors.As(err, &requestErr) || requestErr.Field != "mode" {
		t.Fatalf("detached canonical invalid Start() error = %v, want mode-scoped DetachedRequestError", err)
	}
	_, err = detached.Get(t.Context(), factorysessions.SessionGetRequest{})
	requestErr = nil
	if !errors.As(err, &requestErr) || requestErr.Field != "sessionId" {
		t.Fatalf("detached canonical invalid Get() error = %v, want sessionId-scoped DetachedRequestError", err)
	}
	t.Log("FSCP-02 live canonical Start/Get/List/Close and field validation PASS")
}

func assertProcessComposedDurableOwner(
	t *testing.T,
	canonical factorysessions.Service,
	factoryDir, home string,
) {
	t.Helper()
	if canonical == nil {
		t.Fatal("root process returned no factory sessions Service")
	}

	ownerProbeID := "fscp02-owner-missing-session"
	if _, err := canonical.Start(t.Context(), factorysessions.SessionStartRequest{
		Mode:        factorysessions.SessionOperationModeDurable,
		FolderPath:  factoryDir,
		Persistence: factorysessions.PersistencePolicyDisabled,
		Correlation: factorysessions.SessionOperationCorrelation{RequestID: "fscp02-owner-start"},
		Definition:  factorysessions.SessionDefinitionSelection{FactoryID: "fscp02-missing-factory"},
		RuntimeSelection: &factorysessions.SessionRuntimeSelection{
			SystemConfigHome: home,
			ExecutionBaseDir: factoryDir,
			LogPolicy:        factorysessions.SessionArtifactPolicyDisabled,
			MetricsPolicy:    factorysessions.SessionArtifactPolicyDisabled,
		},
	}); err == nil {
		t.Fatal("canonical durable Start() unexpectedly succeeded for a missing factory")
	}
	listed, err := canonical.List(t.Context(), factorysessions.SessionListRequest{
		Mode: factorysessions.SessionOperationModeDurable,
	})
	if err != nil {
		t.Fatalf("canonical durable List() error = %v", err)
	}
	if listed.Mode != factorysessions.SessionOperationModeDurable {
		t.Fatalf("canonical durable List() mode = %q, want durable", listed.Mode)
	}
	_, err = canonical.Get(t.Context(), factorysessions.SessionGetRequest{
		SessionID: ownerProbeID,
		Mode:      factorysessions.SessionOperationModeDurable,
	})
	if !errors.Is(err, factorysessions.ErrDurableSessionNotFound) {
		t.Fatalf("canonical durable Get() error = %v, want ErrDurableSessionNotFound", err)
	}
	_, err = canonical.Control(t.Context(), factorysessions.SessionControlRequest{
		SessionID: ownerProbeID,
		Mode:      factorysessions.SessionOperationModeDurable,
		Operation: factorysessions.SessionControlPause,
		Control:   factorysessions.ControlRequest{RequestID: "fscp02-owner-control"},
	})
	if !errors.Is(err, factorysessions.ErrDurableSessionNotFound) {
		t.Fatalf("canonical durable Control() error = %v, want ErrDurableSessionNotFound", err)
	}
	_, err = canonical.ReadResult(t.Context(), factorysessions.SessionResultReadRequest{
		SessionID: ownerProbeID,
		Mode:      factorysessions.SessionOperationModeDurable,
		Request:   factorysessions.ResultRequest{Mode: factorysessions.ResultModeFinal},
	})
	if !errors.Is(err, factorysessions.ErrDurableSessionNotFound) {
		t.Fatalf("canonical durable ReadResult() error = %v, want ErrDurableSessionNotFound", err)
	}
	_, err = canonical.QueryDispatches(t.Context(), factorysessions.DispatchQueryRequest{
		SessionID: ownerProbeID,
	})
	if !errors.Is(err, factorysessions.ErrDurableSessionNotFound) {
		t.Fatalf("canonical durable QueryDispatches() error = %v, want ErrDurableSessionNotFound", err)
	}
	_, err = canonical.SubscribeResponses(t.Context(), factorysessions.SessionResponseSubscriptionRequest{
		SessionID: ownerProbeID,
	})
	if !errors.Is(err, factorysessions.ErrDurableSessionNotFound) {
		t.Fatalf("canonical durable SubscribeResponses() error = %v, want ErrDurableSessionNotFound", err)
	}
	t.Log("FSCP-02 durable canonical wrapper methods executed through the process-composed runtime owner")
}

func isolatedEnvironment(home string) []string {
	state := filepath.Join(home, "state")
	cache := filepath.Join(home, "cache")
	return append(os.Environ(),
		"HOME="+home,
		"USERPROFILE="+home,
		"APPDATA="+filepath.Join(home, "appdata"),
		"LOCALAPPDATA="+filepath.Join(home, "localappdata"),
		"XDG_CONFIG_HOME="+filepath.Join(home, "config"),
		"XDG_DATA_HOME="+state,
		"XDG_CACHE_HOME="+cache,
	)
}
