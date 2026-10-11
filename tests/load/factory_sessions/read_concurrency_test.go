package factory_sessions_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// Explicit latency lane: one root host/session, no workers/builds, three
// readers, tiny history. The recorder gate remains held throughout the reads.
func TestSessionReadsWhileRecorderReplayBlocked(t *testing.T) {
	dir, home := t.TempDir(), t.TempDir()
	writeColdFile(t, filepath.Join(dir, "factory.json"), `{"name":"read-concurrency","workTypes":[{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"},{"name":"failed","type":"FAILED"}]}],"workers":[],"workstations":[]}`)
	var ledger recordings.Ledger
	handler := coldListingHost(t, dir, home, func(service recordings.Service) {
		var ok bool
		ledger, ok = service.(recordings.Ledger)
		if !ok {
			t.Fatal("constructed Recordings root does not expose its existing Ledger")
		}
	})
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client := &http.Client{Timeout: time.Second}
	var listing factoryapi.ListFactorySessionsResponse
	readLatencyJSON(t, client, server.URL+"/factory-sessions", &listing)
	if len(listing.Sessions) != 1 {
		t.Fatalf("isolated fixture sessions = %d", len(listing.Sessions))
	}
	id := listing.Sessions[0].Id
	workID := submitLatencyWork(t, server.URL, id, "prefix")
	// Warm readiness before injecting the operation, not before each read.
	var before factoryapi.ListWorkResponse
	readLatencyJSON(t, client, server.URL+"/factory-sessions/"+id+"/work", &before)
	entered, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	var recordedMu sync.Mutex
	var recorded []int
	go func() {
		ledger.AddEventRecorder(func(event factorydefinitions.FactoryEvent) {
			if event.Context.SessionID != nil {
				once.Do(func() { close(entered); <-release })
			}
			recordedMu.Lock()
			recorded = append(recorded, event.Context.Sequence)
			recordedMu.Unlock()
		})
		close(finished)
	}()
	var releaseOnce sync.Once
	finishReplay := func() { releaseOnce.Do(func() { close(release) }); <-finished }
	defer finishReplay()
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("session recorder replay did not enter gate")
	}
	for _, path := range []string{"/factory-sessions", "/factory-sessions/" + id, "/factory-sessions/" + id + "/work?maxResults=1"} {
		t.Run(path, func(t *testing.T) {
			assertLatencyRead(t, client, server.URL, path, id, workID)
		})
	}
	finishReplay()
	submitLatencyWork(t, server.URL, id, "live")
	recordedMu.Lock()
	defer recordedMu.Unlock()
	for index, sequence := range recorded {
		if sequence != index {
			t.Fatalf("recorder replay/live order[%d] = %d", index, sequence)
		}
	}
}

func assertLatencyRead(t *testing.T, client *http.Client, base, path, id, workID string) {
	t.Helper()
	switch path {
	case "/factory-sessions":
		var result factoryapi.ListFactorySessionsResponse
		readLatencyJSON(t, client, base+path, &result)
		for _, session := range result.Sessions {
			if session.Id == id {
				return
			}
		}
		t.Fatal("owned session missing from successful list")
	case "/factory-sessions/" + id:
		var result factoryapi.FactorySessionSummary
		readLatencyJSON(t, client, base+path, &result)
		if result.Id != id {
			t.Fatal("successful detail returned wrong session")
		}
	default:
		var result factoryapi.ListWorkResponse
		readLatencyJSON(t, client, base+path, &result)
		if len(result.Results) != 1 || result.Results[0].WorkId == nil || *result.Results[0].WorkId != workID {
			t.Fatal("successful Work read omitted admitted Work")
		}
	}
}

func submitLatencyWork(t *testing.T, base, id, name string) string {
	t.Helper()
	response, err := http.Post(base+"/factory-sessions/"+id+"/work", "application/json", strings.NewReader(`{"name":"`+name+`","workTypeName":"task","payload":{"message":"synthetic"}}`))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var result factoryapi.SubmitWorkResponse
	if response.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("Work admission HTTP %d: %s", response.StatusCode, body)
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if !result.Accepted || result.WorkId == nil {
		t.Fatal("Work admission did not succeed")
	}
	return *result.WorkId
}

func readLatencyJSON(t *testing.T, client *http.Client, endpoint string, target any) {
	t.Helper()
	started := time.Now()
	response, err := client.Get(endpoint)
	if err != nil {
		t.Fatalf("session read blocked behind recorder replay: GET %s: %v; bound is strictly under 1s", endpoint, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	elapsed := time.Since(started)
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: HTTP %d error %v body %s", endpoint, response.StatusCode, err, body)
	}
	if err := json.Unmarshal(body, target); err != nil {
		t.Fatal(err)
	}
	t.Logf("GET %s successful in %s", endpoint, elapsed)
	if elapsed >= time.Second {
		t.Fatalf("session read blocked behind recorder replay: GET %s took %s; bound is strictly under 1s", endpoint, elapsed)
	}
}
