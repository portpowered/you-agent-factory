package customer_journeys_test

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const healthyHistory = `{"schemaVersion":"replay.v1","events":[{"context":{"sessionId":"00000000-0000-4000-8000-000000000001"},"id":"event-1","payload":{}}]}`
const healthyHeader = `{"recordType":"header","schemaVersion":"agent-factory.replay.v2","recordedAt":"2026-10-03T00:00:00Z","sessionId":"00000000-0000-4000-8000-000000000002","factoryIdentity":{"id":"factory","name":"factory","factoryDirectory":"factory","sourceDirectory":"factory"},"hashes":{"factory_hash":"sha256:factory","workers_hash":"sha256:workers","workstations_hash":"sha256:workstations","runtime_config_hash":"sha256:runtime"}}` + "\n"

func historyFixtureBytes(cell string) map[string]string {
	files := map[string]string{}
	if cell == "absent" || cell == "empty" {
		return files
	}
	if cell != "all bad" {
		files["2026/10/03/healthy.json"] = healthyHistory
		files["2026/10/03/00000000-0000-4000-8000-000000000002.jsonl"] = healthyHeader
	}
	if cell == "mixed" || cell == "all bad" {
		files["2026/10/03/empty.json"] = ""
		files["2026/10/03/truncated.json"] = `{"schemaVersion":"replay.v1","private":"planted-secret","events":[`
	}
	if cell == "read failure" {
		files["2026/10/03/unavailable.json"] = healthyHistory
	}
	return files
}

func seedHistory(t *testing.T, recordingRoot, cell string) ([]string, []string) {
	t.Helper()
	if cell != "absent" {
		if err := os.MkdirAll(recordingRoot, 0700); err != nil {
			t.Fatal(err)
		}
	}
	for path, data := range historyFixtureBytes(cell) {
		full := filepath.Join(recordingRoot, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if cell == "mixed" {
		sibling := filepath.Join(t.TempDir(), ".you-agent-factory", "recordings", "2026", "10", "03")
		if err := os.MkdirAll(sibling, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sibling, "sibling-private.json"), []byte(healthyHistory), 0600); err != nil {
			t.Fatal(err)
		}
	}
	var rows, warnings []string
	if cell != "empty" && cell != "absent" && cell != "all bad" {
		rows = []string{"00000000-0000-4000-8000-000000000001", "00000000-0000-4000-8000-000000000002"}
	}
	if cell == "mixed" || cell == "all bad" {
		warnings = []string{"2026/10/03/empty.json", "2026/10/03/truncated.json"}
	}
	if cell == "read failure" {
		warnings = []string{"2026/10/03/unavailable.json"}
	}
	return rows, warnings
}

func assertHistoryBytesUnchanged(t *testing.T, root, cell string) {
	t.Helper()
	for path, expected := range historyFixtureBytes(cell) {
		actual, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil || string(actual) != expected {
			t.Fatalf("listing mutated %s: %q, %v", path, actual, err)
		}
	}
}

func assertHistoryMCP(t *testing.T, process support.Process, factory string, env, rows, warnings []string) {
	t.Helper()
	inputReader, inputWriter := io.Pipe()
	outputReader, outputWriter := io.Pipe()
	inputs := support.FakeInputs(t.Context(), []string{"you", "server", "mcp", "--project-root", factory})
	inputs.Env, inputs.WorkingDirectory, inputs.Input.Stdin, inputs.Input.Stdout = env, factory, inputReader, outputWriter
	done := make(chan error, 1)
	go func() { done <- process.Execute(inputs.Input); outputWriter.Close() }()
	t.Cleanup(func() { inputWriter.Close(); inputReader.Close(); outputReader.Close(); outputWriter.Close() })
	encoder, decoder := json.NewEncoder(inputWriter), json.NewDecoder(outputReader)
	call := func(id int, method string, params any) json.RawMessage {
		t.Helper()
		if err := encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}); err != nil {
			t.Fatal(err)
		}
		var response struct {
			ID     int             `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  json.RawMessage `json:"error"`
		}
		if err := decoder.Decode(&response); err != nil {
			t.Fatalf("MCP %s: %v; %s", method, err, inputs.Stderr())
		}
		if response.ID != id || len(response.Error) > 0 {
			t.Fatalf("MCP response = %#v", response)
		}
		return response.Result
	}
	call(1, "initialize", map[string]any{"protocolVersion": "2024-11-05", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "history-test", "version": "1"}})
	for i, scope := range []string{"history", "all"} {
		raw := call(i+2, "tools/call", map[string]any{"name": "you.factory_session.list", "arguments": map[string]any{"scope": scope}})
		var tool struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		}
		if err := json.Unmarshal(raw, &tool); err != nil {
			t.Fatal(err)
		}
		if tool.IsError || len(tool.Content) == 0 {
			t.Fatalf("MCP tool = %s", raw)
		}
		var envelope struct {
			Result factoryapi.ListFactorySessionsResponse `json:"result"`
			Error  json.RawMessage                        `json:"error"`
		}
		if err := json.Unmarshal([]byte(tool.Content[0].Text), &envelope); err != nil {
			t.Fatal(err)
		}
		if len(envelope.Error) > 0 && string(envelope.Error) != "null" {
			t.Fatalf("MCP error: %s", tool.Content[0].Text)
		}
		if strings.Contains(tool.Content[0].Text, "planted-secret") {
			t.Fatal("MCP leaked recording content")
		}
		assertHistoryResult(t, envelope.Result, true, rows, warnings)
	}
	inputWriter.Close()
	if err := <-done; err != nil {
		t.Fatalf("MCP shutdown: %v", err)
	}
}
