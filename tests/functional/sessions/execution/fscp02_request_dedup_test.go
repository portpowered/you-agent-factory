package execution_test

import (
	"context"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestFSCP02LiveActivationOnlyRequestIDDedup proves canonical
// Service.Start(live, ActivationOnly:true) RequestID dedup through the
// process-built boundary. Concurrent starts sharing one RequestID must share
// one nonempty SessionID; distinct RequestIDs must yield distinct IDs.
func TestFSCP02LiveActivationOnlyRequestIDDedup(t *testing.T) {
	t.Parallel()
	acquireExecutionFixtureSlot(t)

	factoryDir := support.ScaffoldSingleStepFactory(t, "fscp02-request-dedup")
	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{
		BrowserOpener:         func(context.Context, string) error { return nil },
		ProviderCommandRunner: support.NewStaticSuccessCommandRunner("fscp02 request dedup COMPLETE"),
	})
	if err != nil {
		t.Fatalf("root.BuildProcess() error = %v", err)
	}
	support.CleanupProcess(t, process)

	capability := process.FactorySessions()
	if capability == nil {
		t.Fatal("root process returned no factory sessions capability")
	}
	service, ok := capability.FactorySessions().(factorysessions.Service)
	if !ok || service == nil {
		t.Fatalf("factory sessions capability type = %T, want factorysessions.Service", capability.FactorySessions())
	}

	start := func(requestID string) factorysessions.SessionStartResult {
		started, err := service.Start(t.Context(), factorysessions.SessionStartRequest{
			Mode:           factorysessions.SessionOperationModeLive,
			FolderPath:     factoryDir,
			ActivationOnly: true,
			Correlation:    factorysessions.SessionOperationCorrelation{RequestID: requestID},
			RuntimeSelection: &factorysessions.SessionRuntimeSelection{
				Mode: factorysessions.SessionRuntimeModeService,
			},
		})
		if err != nil {
			t.Errorf("canonical Start(live, ActivationOnly, request %q) error = %v", requestID, err)
			return factorysessions.SessionStartResult{}
		}
		if started.SessionID == "" {
			t.Errorf("canonical Start(live, ActivationOnly, request %q) returned empty SessionID", requestID)
		}
		return started
	}

	const sharedRequestID = "fscp02-dedup-shared"
	const workers = 8
	shared := make([]string, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(slot int) {
			defer wg.Done()
			shared[slot] = start(sharedRequestID).SessionID
		}(i)
	}
	wg.Wait()
	for i := 1; i < workers; i++ {
		if shared[i] == "" || shared[i] != shared[0] {
			t.Fatalf("concurrent same-RequestID SessionIDs = %q, want all %q", shared, shared[0])
		}
	}
	if shared[0] == "" {
		t.Fatal("shared RequestID yielded empty SessionID")
	}

	first := start("fscp02-dedup-distinct-a")
	second := start("fscp02-dedup-distinct-b")
	if first.SessionID == "" || second.SessionID == "" {
		t.Fatalf("distinct RequestIDs yielded empty SessionIDs: %q, %q", first.SessionID, second.SessionID)
	}
	if first.SessionID == second.SessionID {
		t.Fatalf("distinct RequestIDs shared SessionID %q, want distinct IDs", first.SessionID)
	}
	if first.SessionID == shared[0] || second.SessionID == shared[0] {
		t.Fatalf("distinct RequestIDs collided with shared SessionID %q: %q, %q", shared[0], first.SessionID, second.SessionID)
	}

	seen := map[string]struct{}{shared[0]: {}, first.SessionID: {}, second.SessionID: {}}
	t.Cleanup(func() {
		ctx := context.Background()
		for id := range seen {
			if _, err := service.Control(ctx, factorysessions.SessionControlRequest{
				SessionID: id,
				Mode:      factorysessions.SessionOperationModeLive,
				Operation: factorysessions.SessionControlClose,
			}); err != nil {
				t.Errorf("canonical Control(CLOSE) session %q error = %v", id, err)
			}
		}
	})
	t.Log("FSCP-02 live ActivationOnly RequestID dedup PASS")
}

// TestFSCP02LiveTerminalReplacement proves terminal replacement through the
// process-built boundary (core commit d7d0c84cb7): Start(live,
// ActivationOnly:true) with RequestID A, idempotent Control(CANCEL) twice,
// then Start(live, ActivationOnly:true, SessionID prior ID, RequestID B)
// returns the same ID as an active replacement, followed by Invoke and Control(CLOSE).
func TestFSCP02LiveTerminalReplacement(t *testing.T) {
	t.Parallel()
	acquireExecutionFixtureSlot(t)

	factoryDir := scaffoldFSCP03ProbeFactory(t)
	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{
		BrowserOpener:         func(context.Context, string) error { return nil },
		ProviderCommandRunner: support.NewStaticSuccessCommandRunner("fscp02 terminal replacement COMPLETE"),
	})
	if err != nil {
		t.Fatalf("root.BuildProcess() error = %v", err)
	}
	support.CleanupProcess(t, process)

	capability := process.FactorySessions()
	if capability == nil {
		t.Fatal("root process returned no factory sessions capability")
	}
	service, ok := capability.FactorySessions().(factorysessions.Service)
	if !ok || service == nil {
		t.Fatalf("factory sessions capability type = %T, want factorysessions.Service", capability.FactorySessions())
	}

	ctx := t.Context()
	runtimeSelection := &factorysessions.SessionRuntimeSelection{
		Mode: factorysessions.SessionRuntimeModeService,
	}
	started, err := service.Start(ctx, factorysessions.SessionStartRequest{
		Mode:             factorysessions.SessionOperationModeLive,
		FolderPath:       factoryDir,
		ActivationOnly:   true,
		Correlation:      factorysessions.SessionOperationCorrelation{RequestID: "fscp02-terminal-replacement-A"},
		RuntimeSelection: runtimeSelection,
	})
	if err != nil || started.SessionID == "" {
		t.Fatalf("canonical Start(live, ActivationOnly, RequestID A) = %#v, error = %v", started, err)
	}
	priorID := started.SessionID

	for i := 0; i < 2; i++ {
		if _, err := service.Control(ctx, factorysessions.SessionControlRequest{
			SessionID:   priorID,
			Mode:        factorysessions.SessionOperationModeLive,
			Operation:   factorysessions.SessionControlCancel,
			Correlation: factorysessions.SessionOperationCorrelation{RequestID: "fscp02-terminal-replacement-cancel"},
		}); err != nil {
			t.Fatalf("canonical Control(CANCEL) attempt %d error = %v", i+1, err)
		}
	}

	replaced, err := service.Start(ctx, factorysessions.SessionStartRequest{
		SessionID:        priorID,
		Mode:             factorysessions.SessionOperationModeLive,
		FolderPath:       factoryDir,
		ActivationOnly:   true,
		Correlation:      factorysessions.SessionOperationCorrelation{RequestID: "fscp02-terminal-replacement-B"},
		RuntimeSelection: runtimeSelection,
	})
	if err != nil {
		t.Fatalf("canonical Start(live, ActivationOnly, SessionID prior, RequestID B) error = %v", err)
	}
	if replaced.SessionID != priorID {
		t.Fatalf("terminal replacement SessionID = %q, want prior %q", replaced.SessionID, priorID)
	}

	invoked, err := service.Invoke(ctx, factorysessions.SessionInvokeRequest{
		SessionID:   priorID,
		Correlation: factorysessions.SessionOperationCorrelation{RequestID: "fscp02-terminal-replacement-invoke"},
		Input: &work.PreparedInvocationInput{
			Source:        work.InputSourcePositionalText,
			ResolvedInput: &work.ResolvedInput{Source: work.InputSourcePositionalText, Text: "fscp02 terminal replacement"},
		},
	})
	if err != nil {
		t.Fatalf("Invoke on active replacement error = %v", err)
	}
	if invoked.RequestID != "fscp02-terminal-replacement-invoke" {
		t.Fatalf("Invoke RequestID = %q, want fscp02-terminal-replacement-invoke", invoked.RequestID)
	}

	if _, err := service.Control(ctx, factorysessions.SessionControlRequest{
		SessionID: priorID,
		Mode:      factorysessions.SessionOperationModeLive,
		Operation: factorysessions.SessionControlClose,
	}); err != nil {
		t.Fatalf("canonical Control(CLOSE) error = %v", err)
	}
	t.Log("FSCP-02 live ActivationOnly terminal replacement PASS")
}
