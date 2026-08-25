package root_composition_test

import (
	"context"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestFactorySessionsRootPublishesDetachedOperationsThroughProcess proves the
// canonical root.BuildProcess composition publishes the Sessions-owned view
// and that the returned capability performs a detached operation successfully.
func TestFactorySessionsRootPublishesDetachedOperationsThroughProcess(t *testing.T) {
	t.Parallel()

	process := support.BuildProcess(t, serviceedges.Edges{})
	support.CleanupProcess(t, process)
	help := support.FakeInputs(context.Background(), []string{"you", "--help"})
	if err := process.Execute(help.Input); err != nil {
		t.Fatalf("execute canonical process help: %v", err)
	}
	operations := process.DetachedOperations()
	if operations == nil {
		t.Fatal("canonical process published nil detached operations")
	}

	prepared, err := operations.PrepareSync(context.Background(), factorysessions.SessionSyncPreparationRequest{
		Start: factorysessions.SessionStartRequest{
			Mode:        factorysessions.SessionOperationModeDurable,
			Correlation: factorysessions.SessionOperationCorrelation{RequestID: "process-composed"},
		},
		Wait: factorysessions.SessionOperationWait{TimeoutMillis: 25, CancelOnTimeout: true},
	})
	if err != nil {
		t.Fatalf("prepare detached synchronous operation: %v", err)
	}
	if !prepared.Request.Synchronous || prepared.Request.Correlation.RequestID != "process-composed" || prepared.Wait.TimeoutMillis != 25 || !prepared.Wait.CancelOnTimeout {
		t.Fatalf("prepared detached operation = %#v, want normalized synchronous request", prepared)
	}
}
