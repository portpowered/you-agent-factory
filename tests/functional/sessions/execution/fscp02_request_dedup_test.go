package execution_test

import (
	"context"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
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
