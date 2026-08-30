package replay_contracts_test

import (
	"context"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestComposedWorkSnapshotReaderProjectsCanonicalState exercises the
// Recordings-owned Work snapshot adapter after root.BuildProcess composition.
// The assertion observes the detached Work projection after an ordinary
// customer invocation, rather than reaching into the Recordings implementation.
func TestComposedWorkSnapshotReaderProjectsCanonicalState(t *testing.T) {
	var reader interface {
		ReadWorkSnapshot(context.Context, string) (work.ReadSnapshot, error)
	}
	dir := support.ScaffoldFactory(t, replayContractFactoryConfig())
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"composed Work snapshot"}`))
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{
			RecordingsWorkSnapshotReaderObserver: func(candidate interface {
				ReadWorkSnapshot(context.Context, string) (work.ReadSnapshot, error)
			}) {
				reader = candidate
			},
			ProviderCommandRunner: support.NewStaticSuccessCommandRunner("work snapshot provider COMPLETE"),
		},
	})

	terminalObservation := support.OpenDefaultSessionTerminalFactoryEventObservation(t, server.URL())
	support.WaitForSessionWorkTerminalFromFactoryEvents(t, server.URL(), "~default", 15*time.Second)
	if reader == nil {
		t.Fatal("RecordingsWorkSnapshotReaderObserver was not invoked")
	}
	first, err := readComposedWorkSnapshot(t, reader)
	if err != nil {
		t.Fatalf("ReadWorkSnapshot(first): %v", err)
	}
	if len(first.Items) == 0 || len(first.Admissions) == 0 {
		t.Fatalf("ReadWorkSnapshot(first) = %#v, want admitted and projected Work", first)
	}
	completed := false
	for _, item := range first.Items {
		if item.WorkTypeName == "task" && item.State != nil && item.State.Name == "complete" {
			completed = true
			break
		}
	}
	if !completed {
		t.Fatalf("ReadWorkSnapshot(first) = %#v, want task at complete", first)
	}
	second, err := readComposedWorkSnapshot(t, reader)
	if err != nil {
		t.Fatalf("ReadWorkSnapshot(second): %v", err)
	}
	if len(second.Items) != len(first.Items) || len(second.Admissions) != len(first.Admissions) {
		t.Fatalf("repeated Work snapshot sizes changed: first=%#v second=%#v", first, second)
	}
	server.Stop(t)
	terminalObservation.Wait(15 * time.Second)
}

func readComposedWorkSnapshot(
	t *testing.T,
	reader interface {
		ReadWorkSnapshot(context.Context, string) (work.ReadSnapshot, error)
	},
) (work.ReadSnapshot, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	return reader.ReadWorkSnapshot(ctx, "~default")
}
