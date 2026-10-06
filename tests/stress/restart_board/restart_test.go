package restart_board_test

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// Select only on recorded CI hardware; ordinary shared-host runs are diagnostic.
var enforceRestartLatency = flag.Bool("restart-ci-latency", false, "enforce the customer CI-hardware restart bound of less than 10 seconds")

const boardSize = 2000

func TestLargeBoardRestart(t *testing.T) {
	if testing.Short() {
		t.Skip("dedicated 2,000-Work capacity witness")
	}
	repo, home := t.TempDir(), t.TempDir()
	scaffoldBoard(t, repo)
	host := newBoardHost(t, repo, home)
	url, _ := host.start(t)
	fixtureStart := time.Now()
	host.admit(t, url)
	before := host.waitConfirmed(t, url, 1)
	assertBoard(t, before, 1)
	t.Logf("fixture admission/confirmation=%s", time.Since(fixtureStart))
	host.logRetainedEvents(t, url)
	host.shutdown(t, url)
	logRecordingSize(t, repo)

	// Root construction and first-run profile bootstrap are fixture costs.
	// Every relaunch operation from Execute through serving and the successful
	// Work-count/state-summary read belongs to the customer readiness interval.
	t.Logf("resources before restart=%s", processResources())
	url, started := host.start(t)
	serving := time.Since(started)
	host.assertReadyBoard(t, url)
	ready := time.Since(started)
	t.Logf("restart mode strict_ci=%t start=%s serving=%s work_count_state_read=%s OS=%s arch=%s CPUs=%d Go=%s source_head=%s resources=%s",
		*enforceRestartLatency, started.UTC().Format(time.RFC3339Nano), serving, ready,
		runtime.GOOS, runtime.GOARCH, runtime.NumCPU(), runtime.Version(), os.Getenv("INFINITE_YOU_SOURCE_HEAD"), processResources())
	//nolint:testsleep // Explicit customer <10s latency contract in the dedicated load lane.
	if *enforceRestartLatency && ready >= 10*time.Second {
		t.Errorf("CI restart readiness=%s; require less than 10 seconds", ready)
	}
	after := host.readBoard(t, url)
	assertBoard(t, after, 1)
	assertSameBoard(t, before, after)
	t.Logf("restart complete paginated equality=%s", time.Since(started))
	if host.runner.calls.Load() != 1 {
		t.Fatal("restart dispatched terminal prerequisite or blocked descendants")
	}
	host.request(t, "POST", url+"/factory-sessions/~default/work/B/move", []byte(`{"stateName":"init"}`), nil)
	completed := host.waitConfirmed(t, url, 3)
	assertBoard(t, completed, 3)
	if host.runner.calls.Load() != 3 {
		t.Fatalf("dependency release calls=%d, want three total", host.runner.calls.Load())
	}
	host.shutdown(t, url)
}

func assertBoard(t *testing.T, works map[string]factoryapi.Work, terminal int) {
	t.Helper()
	if len(works) != boardSize {
		t.Fatalf("board count=%d, want %d", len(works), boardSize)
	}
	states := map[string]int{}
	for _, w := range works {
		if w.State == nil {
			t.Fatal("Work missing state")
		}
		states[w.State.Name]++
	}
	want := map[string]int{"waiting": boardSize - 3, "complete": terminal}
	if terminal == 1 {
		want["waiting"]++
		want["init"] = 1
	}
	if !reflect.DeepEqual(states, want) {
		t.Fatalf("state distribution=%v, want %v", states, want)
	}
	for _, id := range []string{"B", "C"} {
		w := works[id]
		if w.Relations == nil || len(*w.Relations) != 1 || w.Tags == nil || (*w.Tags)["witness"] != "§ —" {
			t.Fatalf("DAG %s lost tags/relations: %#v", id, w)
		}
	}
}

func assertSameBoard(t *testing.T, before, after map[string]factoryapi.Work) {
	t.Helper()
	for id, old := range before {
		w, ok := after[id]
		if !ok || !reflect.DeepEqual(old.State, w.State) || !reflect.DeepEqual(old.Content, w.Content) ||
			!reflect.DeepEqual(old.Payload, w.Payload) || !reflect.DeepEqual(old.Tags, w.Tags) ||
			!reflect.DeepEqual(old.Relations, w.Relations) || !reflect.DeepEqual(old.RequestId, w.RequestId) {
			t.Fatalf("recovered Work %s changed: before=%#v after=%#v", id, old, w)
		}
	}
}

func logRecordingSize(t *testing.T, repo string) {
	t.Helper()
	var reference struct{ ArtifactReference string }
	readJSONFile(t, filepath.Join(repo, ".you-agent-factory", "current-board.json"), &reference)
	info, err := os.Stat(reference.ArtifactReference)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("confirmed recording bytes=%d workload=%d inert=1997 DAG=3; resource target peak RSS <=1GiB; zero paid calls", info.Size(), boardSize)
}

func workID(i int) string { return fmt.Sprintf("inert-%04d", i) }
