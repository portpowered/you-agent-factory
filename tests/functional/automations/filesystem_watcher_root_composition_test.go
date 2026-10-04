package automations

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// The shared root-built host opens this scenario's explicit Factory Session.
// Real test-owned filesystem input crosses watcher admission and normal worker
// execution; only the script command effect is controlled through Edges.
func newWatcherIngressFactory(t *testing.T, channel string) string {
	t.Helper()
	dir := support.ScaffoldFactory(t, filesystemWatcherFactoryConfig())
	support.ClearSeedInputs(t, dir)
	writeWatcherIngressFile(t, dir, channel, "preseed", "preseed item")
	return dir
}

func assertWatcherSessionIngress(t *testing.T, baseURL, dir, preseedChannel, liveChannel string) {
	t.Helper()
	sessionID := support.OpenFactorySessionAt(t, baseURL, dir).Session.Id
	t.Cleanup(func() { support.CloseFactorySessionAt(t, baseURL, sessionID) })
	if sessionID == "" || sessionID == "~default" {
		t.Fatalf("watcher session = %q, want explicit identity", sessionID)
	}
	seeded := readAutomationCompletedWork(t, baseURL, sessionID, "complete", "preseed")
	assertWatcherIngressWork(t, seeded, map[string]string{"preseed": "preseed item"})
	assertWatcherIngressCorrelation(t, seeded, map[string]string{"preseed": preseedChannel})

	// Public completion acknowledges the preseed before introducing live input.
	// Dispatch completion plus the public Work state are the synchronization
	// signals, with no fixed delay or private handled-file ledger inspection.
	writeWatcherIngressFile(t, dir, liveChannel, "live", "live item")
	completed := readAutomationCompletedWork(t, baseURL, sessionID, "complete", "live")
	assertWatcherIngressWork(t, completed, map[string]string{
		"preseed": "preseed item", "live": "live item",
	})
	assertWatcherIngressCorrelation(t, completed, map[string]string{
		"preseed": preseedChannel, "live": liveChannel,
	})
}

func assertWatcherIngressCorrelation(t *testing.T, listed factoryapi.ListWorkResponse, channels map[string]string) {
	t.Helper()
	for _, item := range listed.Results {
		id := support.StringPointerValue(item.WorkId)
		if item.RequestId == nil || *item.RequestId != "watcher-"+id {
			t.Fatalf("watcher Work %q request = %v, want original watcher request", id, item.RequestId)
		}
		// Work's public tags retain the execution-directory correlation. The
		// default channel has no execution ID; a named channel keeps its exact ID
		// through admission and normal worker completion.
		var executionID string
		if item.Tags != nil {
			executionID = (*item.Tags)["_execution_id"]
		}
		want := channels[id]
		if want == interfaces.DefaultChannelName {
			want = ""
		}
		if executionID != want {
			t.Fatalf("watcher Work %q execution tag = %q, want %q", id, executionID, want)
		}
	}
}

func assertWatcherIngressWork(t *testing.T, listed factoryapi.ListWorkResponse, expected map[string]string) {
	t.Helper()
	if len(listed.Results) != len(expected) {
		t.Fatalf("watcher Work = %#v, want exactly %d items", listed.Results, len(expected))
	}
	for id := range expected {
		if !support.HasWorkAtCustomerState(listed, id, support.WorkCustomerLocation("task", "complete")) {
			t.Fatalf("watcher Work = %#v, want %q at task:complete", listed.Results, id)
		}
	}
	for _, item := range listed.Results {
		id := support.StringPointerValue(item.WorkId)
		title, exists := expected[id]
		if !exists {
			t.Fatalf("watcher Work = %#v, want owned identity at task:complete", item)
		}
		payload, ok := item.Payload.(map[string]any)
		if !ok || payload["title"] != title {
			t.Fatalf("watcher Work %q payload = %#v, want title %q", id, item.Payload, title)
		}
		if item.State == nil || item.State.Type != factoryapi.WorkStateTypeTERMINAL {
			t.Fatalf("watcher Work %q state = %#v, want terminal", id, item.State)
		}
	}
}

func assertWatcherDuplicateRestart(t *testing.T, baseURL, dir string) {
	t.Helper()
	opened := support.OpenFactorySessionAt(t, baseURL, dir)
	sessionID := opened.Session.Id
	t.Cleanup(func() {
		if sessionID != "" {
			support.CloseFactorySessionAt(t, baseURL, sessionID)
		}
	})
	assertWatcherIngressWork(t, readAutomationCompletedWork(t, baseURL, sessionID, "complete", "preseed"),
		map[string]string{"preseed": "preseed item"})
	// Recreate the same retained input, preserving its file, request and logical
	// Work identities. The observable contract is one logical Work per session.
	path := filepath.Join(dir, interfaces.InputsDir, "task", interfaces.DefaultChannelName, "preseed.json")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	writeWatcherIngressFile(t, dir, interfaces.DefaultChannelName, "preseed", "preseed item")
	writeWatcherIngressFile(t, dir, interfaces.DefaultChannelName, "live", "live item")
	// The next owned file's public completion provides a positive observation
	// barrier for the watcher, rather than a sleep waiting for absent Work.
	assertWatcherIngressWork(t, readAutomationCompletedWork(t, baseURL, sessionID, "complete", "live"),
		map[string]string{"preseed": "preseed item", "live": "live item"})
	events := support.GetFactoryEventsForSessionAt(t, baseURL, sessionID)
	for _, id := range []string{"preseed", "live"} {
		support.AssertSingleWorkRequestEvent(t, events, "watcher-"+id, id, "task")
	}

	support.CloseFactorySessionAt(t, baseURL, sessionID)
	priorSessionID := sessionID
	sessionID = ""
	reopened := support.OpenFactorySessionAt(t, baseURL, dir)
	sessionID = reopened.Session.Id
	if sessionID == priorSessionID || sessionID == "~default" ||
		reopened.Session.FactoryDir != opened.Session.FactoryDir || reopened.Session.FolderPath != opened.Session.FolderPath {
		t.Fatalf("reopened watcher session = %#v, want same logical target and new live identity", reopened.Session)
	}
	writeWatcherIngressFile(t, dir, interfaces.DefaultChannelName, "restart", "restart item")
	// Both files remain on disk across supported close / reopen. Startup admits
	// retained inputs into the new live session; each original logical identity
	// must occur exactly once alongside the fresh Work proving live activation.
	assertWatcherIngressWork(t, readAutomationCompletedWork(t, baseURL, sessionID, "complete", "preseed", "live", "restart"),
		map[string]string{"preseed": "preseed item", "live": "live item", "restart": "restart item"})
	events = support.GetFactoryEventsForSessionAt(t, baseURL, sessionID)
	for _, id := range []string{"preseed", "live", "restart"} {
		support.AssertSingleWorkRequestEvent(t, events, "watcher-"+id, id, "task")
	}
}

func assertWatcherIndependentStop(t *testing.T, baseURL, sourceDir, peerDir string, stoppedAdmissions *atomic.Int32) {
	t.Helper()
	opened := support.OpenFactorySessionAt(t, baseURL, sourceDir)
	sessionID := opened.Session.Id
	t.Cleanup(func() {
		if sessionID != "" {
			support.CloseFactorySessionAt(t, baseURL, sessionID)
		}
	})
	peerID := support.OpenFactorySessionAt(t, baseURL, peerDir).Session.Id
	t.Cleanup(func() { support.CloseFactorySessionAt(t, baseURL, peerID) })
	if sessionID == peerID || sessionID == "~default" || peerID == "~default" {
		t.Fatalf("watcher sessions = %q/%q, want distinct explicit identities", sessionID, peerID)
	}
	for _, id := range []string{sessionID, peerID} {
		assertWatcherIngressWork(t, readAutomationCompletedWork(t, baseURL, id, "complete", "preseed"),
			map[string]string{"preseed": "preseed item"})
	}
	// Public close waits for stopped status and DELETE acknowledges deactivation
	// and join. Publish source input only after that full control completes.
	support.CloseFactorySessionAt(t, baseURL, sessionID)
	priorSessionID := sessionID
	sessionID = ""
	writeWatcherIngressFile(t, sourceDir, interfaces.DefaultChannelName, "stopped-only", "retained while stopped")
	writeWatcherIngressFile(t, peerDir, interfaces.DefaultChannelName, "peer-live", "independent peer item")
	assertWatcherIngressWork(t, readAutomationCompletedWork(t, baseURL, peerID, "complete", "peer-live"),
		map[string]string{"preseed": "preseed item", "peer-live": "independent peer item"})
	// The declared submission effect observes admission even after the old live
	// session is deleted. Peer completion is a positive barrier, not a sleep.
	if count := stoppedAdmissions.Load(); count != 0 {
		t.Fatalf("stopped watcher admitted retained input %d times before restart", count)
	}
	reopened := support.OpenFactorySessionAt(t, baseURL, sourceDir)
	sessionID = reopened.Session.Id
	if sessionID == priorSessionID || reopened.Session.FactoryDir != opened.Session.FactoryDir ||
		reopened.Session.FolderPath != opened.Session.FolderPath {
		t.Fatalf("reopened watcher = %#v, want same target and new live identity", reopened.Session)
	}
	writeWatcherIngressFile(t, sourceDir, interfaces.DefaultChannelName, "reactivated", "eligible live item")
	assertWatcherIngressWork(t, readAutomationCompletedWork(t, baseURL, sessionID, "complete", "preseed", "stopped-only", "reactivated"),
		map[string]string{"preseed": "preseed item", "stopped-only": "retained while stopped", "reactivated": "eligible live item"})
	if count := stoppedAdmissions.Load(); count != 1 {
		t.Fatalf("reactivated watcher admissions = %d, want one eligible retained input", count)
	}
}

func writeWatcherIngressFile(t *testing.T, dir, channel, id, title string) {
	t.Helper()
	inputDir := filepath.Join(dir, interfaces.InputsDir, "task", channel)
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content, err := json.Marshal(map[string]any{
		"requestId": "watcher-" + id, "type": "FACTORY_REQUEST_BATCH",
		"works": []map[string]any{{
			"name": id, "workId": id, "workTypeName": "task",
			"payload": map[string]string{"title": title},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Atomically publish complete input to the real watcher, avoiding partial
	// writes on Windows. The temporary extension is ignored by watcher policy.
	temporary := filepath.Join(inputDir, id+".tmp")
	if err := os.WriteFile(temporary, content, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(temporary, filepath.Join(inputDir, id+".json")); err != nil {
		t.Fatal(err)
	}
}

func filesystemWatcherFactoryConfig() map[string]any {
	return map[string]any{
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
			},
		}},
		"workers": []map[string]string{{
			"name": "processor", "type": "SCRIPT_WORKER", "command": "watcher-process",
		}},
		"workstations": []map[string]any{{
			"name": "process", "worker": "processor",
			"inputs":  []map[string]string{{"workType": "task", "state": "init"}},
			"outputs": []map[string]string{{"workType": "task", "state": "complete"}},
		}},
	}
}
