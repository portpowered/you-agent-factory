package replay_contracts_test

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestReplayProjectionExposesSelectedHistoricalTick uses two public replay
// executions of the same session history: the prefix selects the intermediate
// tick and the complete artifact selects the later tick. Repeating the public
// read proves the detached projection is stable and the source ledger artifact
// remains unchanged.
func TestReplayProjectionExposesSelectedHistoricalTick(t *testing.T) {
	for _, test := range []struct {
		name          string
		terminal      bool
		wantLocation  string
		wantTotalWork int
	}{
		{name: "intermediate tick", wantLocation: "task:ready", wantTotalWork: 1},
		{name: "later tick", terminal: true, wantLocation: "task:complete", wantTotalWork: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			replaySelectedTickCase(t, test.terminal, test.wantLocation, test.wantTotalWork)
		})
	}
}

func replaySelectedTickCase(t *testing.T, terminal bool, wantLocation string, wantTotalWork int) {
	t.Helper()
	payload := selectedTickArtifactPayload(t, terminal)
	artifactPath := filepath.Join(t.TempDir(), "selected-tick.replay.json")
	if err := os.WriteFile(artifactPath, payload, 0o600); err != nil {
		t.Fatalf("write selected-tick artifact: %v", err)
	}
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: t.TempDir(),
		Args:       []string{"--replay", artifactPath, "--no-record"},
	})
	if terminal {
		support.WaitForTerminalStatus(t, server.URL(), 15*time.Second)
	} else {
		support.WaitForStatus(t, server.URL(), 15*time.Second, func(status factoryapi.StatusResponse) bool {
			return status.TotalTokens > 0 && status.Categories.Initial == 1
		})
	}
	first := support.ListDefaultSessionWork(t, server.URL())
	if len(first.Results) != wantTotalWork || !support.HasWorkAtCustomerState(first, "selected-tick-work", wantLocation) {
		t.Fatalf("selected-tick Work = %#v, want %d item at %s", first.Results, wantTotalWork, wantLocation)
	}
	second := support.ListDefaultSessionWork(t, server.URL())
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("repeated detached projection = %#v, want %#v", second, first)
	}
	liveEvents := server.GetFactoryEvents(t)
	if len(liveEvents) == 0 {
		t.Fatal("selected-tick replay exposed no public Factory Events")
	}
	server.Stop(t)
	if got, err := os.ReadFile(artifactPath); err != nil {
		t.Fatalf("read selected-tick artifact after projection: %v", err)
	} else if !bytes.Equal(got, payload) {
		t.Fatal("selected-tick projection mutated the canonical replay artifact")
	}
	if loaded := testutil.LoadReplayArtifact(t, artifactPath); len(loaded.Events) == 0 {
		t.Fatal("selected-tick artifact lost canonical events after projection")
	}
}
