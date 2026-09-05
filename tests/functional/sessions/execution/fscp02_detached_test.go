package execution_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	processcontract "github.com/portpowered/infinite-you/pkg/initializer/process"
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
	capability := process.DetachedOperations()
	if capability == nil {
		t.Fatal("root process returned no detached capability")
	}
	detached, ok := capability.DetachedOperations().(factorysessions.DetachedService)
	if !ok || detached == nil {
		t.Fatalf("detached capability type = %T, want factorysessions.DetachedService", capability.DetachedOperations())
	}
	assertDetachedLiveBoundary(t, detached, factoryDir)
	assertProcessComposedDurableOwner(t, process.ExecutionRuntimeOpening(), factoryDir, home)
}

func assertDetachedLiveBoundary(t *testing.T, detached factorysessions.DetachedService, factoryDir string) {
	t.Helper()
	_, err := detached.Start(t.Context(), factorysessions.SessionStartRequest{
		Mode:       factorysessions.SessionOperationModeLive,
		FolderPath: factoryDir,
	})
	if err == nil || err.Error() != "Factory Sessions gateway is required" {
		t.Fatalf("detached canonical Start(live) error = %v, want excluded assembly boundary", err)
	}
	t.Logf("FSCP-02 F1/F2/F3/F4/F5/F6/F7/F8/F9/F10/F11/F12/F14 INCONCLUSIVE: root-built detached live start reached excluded assembly boundary: %v", err)

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
	t.Log("FSCP-02 F13 PASS: detached invalid requests returned canonical field-scoped errors before the excluded live gateway")

	t.Log("FSCP-02 durable detached calls remain INCONCLUSIVE at the excluded late-bound assembly boundary")
}

func assertProcessComposedDurableOwner(
	t *testing.T,
	openingCapability processcontract.ExecutionRuntimeOpeningCapability,
	factoryDir, home string,
) {
	t.Helper()
	if openingCapability == nil {
		t.Fatal("root process returned no execution runtime opening capability")
	}
	opening, ok := openingCapability.ExecutionRuntimeOpening().(factorysessions.ExecutionRuntimeOpeningFunc)
	if !ok || opening == nil {
		t.Fatalf("execution runtime opening type = %T, want factorysessions.ExecutionRuntimeOpeningFunc", openingCapability.ExecutionRuntimeOpening())
	}
	opened, err := opening(t.Context(), factorysessions.ExecutionRuntimeOpeningRequest{
		ProjectRoot:       factoryDir,
		SystemConfigHome:  home,
		FactorySessionID:  "fscp02-owner-probe",
		PersistencePolicy: factorysessions.PersistencePolicyDisabled,
	})
	if err != nil {
		t.Fatalf("process execution runtime opening error = %v", err)
	}
	if opened.Execution == nil {
		t.Fatal("process execution runtime opening returned no durable owner")
	}
	if opened.Close != nil {
		t.Cleanup(func() {
			if err := opened.Close(); err != nil {
				t.Errorf("close process execution runtime: %v", err)
			}
		})
	}

	canonical, ok := opened.Execution.(canonicalSessionsOperations)
	if !ok {
		t.Fatalf("process-composed execution type = %T, want canonical Sessions operations", opened.Execution)
	}

	ownerProbeID := "fscp02-owner-missing-session"
	if _, err := canonical.Start(t.Context(), factorysessions.SessionStartRequest{
		Mode:        factorysessions.SessionOperationModeDurable,
		Correlation: factorysessions.SessionOperationCorrelation{RequestID: "fscp02-owner-start"},
		Definition:  factorysessions.SessionDefinitionSelection{FactoryID: "fscp02-missing-factory"},
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

type canonicalSessionsOperations interface {
	Start(context.Context, factorysessions.SessionStartRequest) (factorysessions.SessionStartResult, error)
	List(context.Context, factorysessions.SessionListRequest) (factorysessions.SessionListResult, error)
	Get(context.Context, factorysessions.SessionGetRequest) (factorysessions.SessionGetResult, error)
	Control(context.Context, factorysessions.SessionControlRequest) (factorysessions.SessionControlResult, error)
	ReadResult(context.Context, factorysessions.SessionResultReadRequest) (factorysessions.SessionResultReadResult, error)
	QueryDispatches(context.Context, factorysessions.DispatchQueryRequest) (factorysessions.ListDispatchesResult, error)
	SubscribeResponses(context.Context, factorysessions.SessionResponseSubscriptionRequest) (factorysessions.SessionResponseSubscriptionResult, error)
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
