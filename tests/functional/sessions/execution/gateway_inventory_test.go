package execution_test

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// Inventory is an API-owned contract. One root-built process records two idle
// Factory runs through Execute, then hosts live and durable sessions. Real
// local storage and serializers preserve the history boundary; no provider
// runs because the recorded factories contain no submitted Work. The global
// inventory observation owns an isolated home and Factory directory.
func TestGatewaySessionInventoriesPreserveIdentityAndHistory(t *testing.T) {
	t.Parallel()
	acquireExecutionFixtureSlot(t)
	dir := scaffoldFSCP03ProbeFactory(t)
	home := t.TempDir()
	directories := &gatewayHistoryDirectoryFault{root: filepath.Join(home, ".you-agent-factory", "recordings")}
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir,
		Env:        append(os.Environ(), "HOME="+home, "USERPROFILE="+home),
		Edges: serviceedges.Edges{
			FactorySessionResolveHomeDirectory: func() (string, error) { return home, nil },
			BrowserOpener:                      func(context.Context, string) error { return nil },
			RecordingReadDirectory:             directories.readDirectory,
		},
		BeforeStart: func(tb testing.TB, process support.Process, input root.Input) {
			for _, name := range []string{"second.json", "first.json"} {
				path := filepath.Join(home, ".you-agent-factory", "recordings", "2026", "10", "05", name)
				inputs := support.FakeInputs(tb.Context(), []string{"you", "run", "--dir", dir, "--record", path})
				inputs.Env = input.Env
				inputs.WorkingDirectory = dir
				if err := process.Execute(inputs.Input); err != nil {
					tb.Fatalf("record idle Factory: %v\n%s", err, inputs.Stderr())
				}
			}
		},
	})
	t.Cleanup(func() { server.Close(t) })
	durable := startGatewaySnapshotSession(t, server.URL(), "gateway-inventory")
	assertGatewaySnapshotResult(t, server.URL(), durable.SessionId)

	defaultList := gatewayInventory(t, server.URL(), "")
	live := gatewayInventory(t, server.URL(), "live")
	assertGatewayLiveInventory(t, defaultList, live, dir)
	history := gatewayInventory(t, server.URL(), "history")
	assertGatewayRecordedInventory(t, history)
	persisted := gatewayInventory(t, server.URL(), "persisted")
	if len(persisted.Sessions) != 0 || persisted.RecordedSessions != nil || persisted.DurableSessions == nil || len(*persisted.DurableSessions) != 1 {
		t.Fatalf("persisted inventory = %#v, want only one durable row", persisted)
	}
	if row := (*persisted.DurableSessions)[0]; row.SessionId != durable.SessionId || string(row.Status) != "SUCCEEDED" {
		t.Fatalf("persisted row = %#v, want selected terminal session %q", row, durable.SessionId)
	}
	all := gatewayInventory(t, server.URL(), "all")
	if !sameGatewayLiveInventory(all.Sessions, live.Sessions) || !reflect.DeepEqual(all.DurableSessions, persisted.DurableSessions) || !reflect.DeepEqual(all.RecordedSessions, history.RecordedSessions) {
		t.Fatalf("all inventory lost or changed owner rows: %#v", all)
	}
	assertGatewayHistoryFailureRecovery(t, server.URL(), directories, live, history, durable.SessionId)
	assertGatewayEmptyHistoryKeepsPeers(t, server.URL(), home, live, persisted)
}

func assertGatewayEmptyHistoryKeepsPeers(t *testing.T, baseURL, home string, live, persisted factoryapi.ListFactorySessionsResponse) {
	t.Helper()
	// Remove only the two completed scenario-owned artifacts. The real dated
	// directory remains, so this crosses the empty-directory boundary rather
	// than substituting an empty inventory or a missing-directory error.
	for _, name := range []string{"first.json", "second.json"} {
		path := filepath.Join(home, ".you-agent-factory", "recordings", "2026", "10", "05", name)
		if err := os.Remove(path); err != nil {
			t.Fatalf("remove completed scenario recording: %v", err)
		}
	}
	history := gatewayInventory(t, baseURL, "history")
	if len(history.Sessions) != 0 || history.DurableSessions != nil || history.RecordedSessions != nil && len(*history.RecordedSessions) != 0 {
		t.Fatalf("empty history inventory = %#v, want no owner rows", history)
	}
	all := gatewayInventory(t, baseURL, "all")
	if !sameGatewayLiveInventory(all.Sessions, live.Sessions) || !reflect.DeepEqual(all.DurableSessions, persisted.DurableSessions) || all.RecordedSessions != nil && len(*all.RecordedSessions) != 0 {
		t.Fatalf("empty history changed live/durable peer rows: %#v", all)
	}
}

func assertGatewayHistoryFailureRecovery(t *testing.T, baseURL string, directories *gatewayHistoryDirectoryFault, live, history factoryapi.ListFactorySessionsResponse, peerID string) {
	t.Helper()
	directories.setFailure(true)
	for _, scope := range []string{"history", "all"} {
		status, body := gatewaySnapshotRequest(t, http.MethodGet, baseURL+"/factory-sessions?scope="+scope, nil)
		// The HTTP boundary sanitizes the wrapped filesystem cause.
		if status != http.StatusInternalServerError || !strings.Contains(string(body), `"code":"INTERNAL_ERROR"`) || !strings.Contains(string(body), `"message":"failed to list factory sessions"`) {
			t.Fatalf("failed %s inventory = %d %s, want visible history read error", scope, status, body)
		}
		if strings.Contains(string(body), `"sessions":`) || strings.Contains(string(body), `"durableSessions":`) {
			t.Fatalf("failed %s inventory returned partial success: %s", scope, body)
		}
	}
	if !sameGatewayLiveInventory(gatewayInventory(t, baseURL, "live").Sessions, live.Sessions) {
		t.Fatal("history read failure changed the live peer inventory")
	}
	assertGatewaySnapshotResult(t, baseURL, peerID)
	assertGatewaySnapshotInventory(t, baseURL, peerID)
	directories.setFailure(false)
	recovered := gatewayInventory(t, baseURL, "history")
	if !reflect.DeepEqual(recovered.RecordedSessions, history.RecordedSessions) {
		t.Fatalf("history recovery = %#v, want retained rows %#v", recovered, history)
	}
	all := gatewayInventory(t, baseURL, "all")
	if !sameGatewayLiveInventory(all.Sessions, live.Sessions) || !reflect.DeepEqual(all.RecordedSessions, history.RecordedSessions) {
		t.Fatalf("combined inventory recovery lost peer/history rows: %#v", all)
	}
}

// Only the scenario-owned history root fails. The switch is synchronized with
// concurrent server reads; clearing it restores the same real artifacts.
type gatewayHistoryDirectoryFault struct {
	root string
	mu   sync.Mutex
	fail bool
}

func (d *gatewayHistoryDirectoryFault) readDirectory(path string) ([]fs.DirEntry, error) {
	d.mu.Lock()
	fail := d.fail && filepath.Clean(path) == d.root
	d.mu.Unlock()
	if fail {
		return nil, errors.New("injected history directory read failure")
	}
	return os.ReadDir(path)
}

func (d *gatewayHistoryDirectoryFault) setFailure(fail bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.fail = fail
}

func assertGatewayLiveInventory(t *testing.T, defaultList, live factoryapi.ListFactorySessionsResponse, dir string) {
	t.Helper()
	if !sameGatewayLiveInventory(defaultList.Sessions, live.Sessions) || len(live.Sessions) != 1 {
		t.Fatalf("default/live inventories = %#v / %#v, want the same live session", defaultList, live)
	}
	if _, err := uuid.Parse(live.Sessions[0].Id); err != nil {
		t.Fatalf("live canonical ID = %q: %v", live.Sessions[0].Id, err)
	}
	if !live.Sessions[0].IsDefault || live.Sessions[0].FactoryDir != dir {
		t.Fatalf("live placement = %#v, want selected default Factory", live.Sessions[0])
	}
	if live.RecordedSessions != nil || live.DurableSessions != nil {
		t.Fatalf("live inventory leaked other owners: %#v", live)
	}
}

// Live runtime observations include changing timestamps. Compare the stable
// customer identity and placement across separate reads, rather than requiring
// the runtime to stop observing between requests.
func sameGatewayLiveInventory(left, right []factoryapi.FactorySessionSummary) bool {
	if len(left) != len(right) {
		return false
	}
	for i, row := range left {
		other := right[i]
		if row.Id != other.Id || row.IsDefault != other.IsDefault || row.FactoryDir != other.FactoryDir || row.FolderPath != other.FolderPath || row.Project != other.Project || !reflect.DeepEqual(row.Target, other.Target) {
			return false
		}
	}
	return true
}

func gatewayInventory(t *testing.T, baseURL, scope string) factoryapi.ListFactorySessionsResponse {
	t.Helper()
	endpoint := baseURL + "/factory-sessions"
	if scope != "" {
		endpoint += "?scope=" + scope
	}
	return support.GetJSON[factoryapi.ListFactorySessionsResponse](t, endpoint)
}

func assertGatewayRecordedInventory(t *testing.T, listed factoryapi.ListFactorySessionsResponse) {
	t.Helper()
	if len(listed.Sessions) != 0 || listed.DurableSessions != nil || listed.RecordedSessions == nil || len(*listed.RecordedSessions) != 2 {
		t.Fatalf("history inventory = %#v, want only two recorded rows", listed)
	}
	keys := make([]string, 0, 2)
	refs := make([]string, 0, 2)
	for _, row := range *listed.RecordedSessions {
		// Whole-file recordings retain the original logical default identity;
		// their artifact reference distinguishes successive runs.
		if row.SessionId != "~default" {
			t.Fatalf("recorded identity = %q, want original logical default", row.SessionId)
		}
		if row.Source != factoryapi.FactorySessionRecordedSourceHistory || row.Format != factoryapi.FactorySessionRecordedFormatV1JSON {
			t.Fatalf("recorded provenance = %#v, want whole-file history", row)
		}
		keys = append(keys, row.SessionId+"/"+row.ArtifactReference)
		refs = append(refs, row.ArtifactReference)
	}
	if !sort.StringsAreSorted(keys) {
		t.Fatalf("history ordering = %#v, want session then artifact", keys)
	}
	sort.Strings(refs)
	if !reflect.DeepEqual(refs, []string{"2026/10/05/first.json", "2026/10/05/second.json"}) {
		t.Fatalf("history references = %#v, want root-relative recorded artifacts", refs)
	}
}
