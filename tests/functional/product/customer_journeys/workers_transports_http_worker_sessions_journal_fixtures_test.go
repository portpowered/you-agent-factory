package customer_journeys_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// A clean host has no admitted owner for this retained opening. A resumable
// durable token does not turn the incomplete archive into a live execution.
func assertIncompleteCapturedFollow(t *testing.T, config support.FunctionalAPIServerConfig, current factoryapi.WorkerSessionLogPage) {
	t.Helper()
	root := t.TempDir()
	path, journal := copyCurrentCapturedJournal(t, config, root)
	writeCapturedJournal(t, path, journal[:1])
	config.Edges.FactorySessionsWorkingDirectory = capturedRecordingDirectory(root)
	server := support.StartFunctionalAPIServer(t, config)
	page := assertRestoredLogsCLIHTTPParity(t, server, current.WorkerSessionId)
	if page.Health != factoryapi.INCOMPLETE || page.NextToken == nil || len(page.Events) != 1 {
		t.Fatalf("owner-lost prefix not truthfully resumable: %+v", page)
	}
	assertCapturedFollowFailure(t, server, current.WorkerSessionId, "", "WORKER_SESSION_LOGS_GAP", page.Events)
	assertCapturedFollowFailure(t, server, current.WorkerSessionId, *page.NextToken, "WORKER_SESSION_LOGS_GAP", nil)
}

func assertRestoredLogsCLIHTTPParity(t *testing.T, server *support.FunctionalAPIServer, id string) factoryapi.WorkerSessionLogPage {
	t.Helper()
	page := support.GetJSON[factoryapi.WorkerSessionLogPage](t, server.URL()+"/worker-sessions/"+url.PathEscape(id)+"/logs")
	inputs := support.FakeInputs(t.Context(), []string{"you", "worker-sessions", "read", "--worker-session-id", id, "--view", "logs", "--server", server.URL(), "--output", "json"})
	if err := server.Execute(t, inputs.Input); err != nil {
		t.Fatalf("CLI restored logs: %v %s", err, inputs.Stderr())
	}
	var cli factoryapi.WorkerSessionLogPage
	if err := json.Unmarshal([]byte(inputs.Stdout()), &cli); err != nil || !reflect.DeepEqual(cli, page) {
		t.Fatalf("restored CLI/HTTP page differs: error=%v", err)
	}
	return page
}

func assertRestoredCapturedEvents(t *testing.T, restored, current factoryapi.WorkerSessionLogPage) {
	t.Helper()
	if restored.RecordingGenerationId != current.RecordingGenerationId || restored.CommittedPosition != current.CommittedPosition || !reflect.DeepEqual(restored.Events, current.Events) {
		t.Fatal("restored journal changed the generation, captured times or ordered history")
	}
}

// Copy the current writer's customer-owned output; recovery scenarios change
// only their copy. No historical snapshot or synthesized identity is involved.
func copyCurrentCapturedJournal(t *testing.T, config support.FunctionalAPIServerConfig, root string) (string, []map[string]any) {
	t.Helper()
	sourceRoot, err := config.Edges.FactorySessionsWorkingDirectory.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join(sourceRoot, ".you-agent-factory", "worker-recordings", "*.worker.jsonl"))
	if err != nil || len(files) != 1 {
		t.Fatalf("current captured journals = %v, error=%v, want one", files, err)
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	var journal []map[string]any
	decoder := json.NewDecoder(bytes.NewReader(data))
	for {
		var row map[string]any
		err := decoder.Decode(&row)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read current journal: %v", err)
		}
		journal = append(journal, row)
	}
	if len(journal) == 0 {
		t.Fatal("current captured journal is empty")
	}
	path := filepath.Join(root, ".you-agent-factory", "worker-recordings", filepath.Base(files[0]))
	writeCapturedJournal(t, path, journal)
	return path, journal
}

func writeCapturedJournal(t *testing.T, path string, journal []map[string]any) {
	t.Helper()
	var data bytes.Buffer
	encoder := json.NewEncoder(&data)
	for _, row := range journal {
		if err := encoder.Encode(row); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}
