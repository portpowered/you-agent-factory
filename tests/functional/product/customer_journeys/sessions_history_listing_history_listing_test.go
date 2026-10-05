package customer_journeys_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// History is profile-wide: independent parallel cells own separate profiles and
// reuse their host's root process for the CLI and MCP parity observations.
func TestHistoryListing(t *testing.T) {
	t.Parallel()
	for _, cell := range []string{"healthy", "mixed", "empty", "absent", "all bad", "read failure", "scan failure"} {
		t.Run(cell, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			factory := support.ScaffoldSingleStepFactory(t, "history-listing")
			recordingRoot := filepath.Join(home, ".you-agent-factory", "recordings")
			rows, warnings := seedHistory(t, recordingRoot, cell)
			env := builtcliacceptance.ProcessEnvForIsolatedHome(home)
			edges := historyEdges(home, recordingRoot, cell)
			var process support.Process
			config := support.FunctionalAPIServerConfig{
				FactoryDir: factory, Env: env, Edges: edges,
				BeforeStart: func(_ testing.TB, p support.Process, _ root.Input) { process = p },
			}
			server := support.StartFunctionalAPIServer(t, config)
			assertHistorySurfaces(t, process, server.URL(), factory, env, home, rows, warnings, cell == "scan failure")
			if cell == "mixed" {
				// Fresh construction is needed to prove cold load; stop the old owner
				// before handing the exact same recording bytes to the next host.
				server.Close(t)
				server = support.StartFunctionalAPIServer(t, config)
				assertHistorySurfaces(t, process, server.URL(), factory, env, home, rows, warnings, false)
			}
			assertHistoryBytesUnchanged(t, recordingRoot, cell)
			if cell == "mixed" {
				endpoint := server.URL()
				server.Close(t)
				inputs := support.FakeInputs(t.Context(), []string{"you", "--server", endpoint, "--json", "session", "list", "--history-only"})
				inputs.Env, inputs.WorkingDirectory = env, factory
				if err := process.Execute(inputs.Input); err == nil {
					t.Fatal("unreachable selected host silently fell back")
				}
			}
		})
	}
}

func historyEdges(home, recordingRoot, cell string) serviceedges.Edges {
	edges := serviceedges.Edges{FactorySessionResolveHomeDirectory: func() (string, error) { return home, nil }}
	if cell == "read failure" {
		edges.RecordingOpenFile = func(path string) (io.ReadCloser, error) {
			if filepath.Base(path) == "unavailable.json" {
				return nil, errors.New("planted-secret " + home)
			}
			return os.Open(path)
		}
	}
	if cell == "scan failure" {
		edges.RecordingReadDirectory = func(path string) ([]fs.DirEntry, error) {
			if filepath.Clean(path) == recordingRoot {
				return nil, fs.ErrPermission
			}
			return os.ReadDir(path)
		}
	}
	return edges
}

func assertHistorySurfaces(t *testing.T, process support.Process, endpoint, factory string, env []string, home string, rows, warnings []string, fatal bool) {
	t.Helper()
	for _, scope := range []string{"history", "all", "live", "persisted", ""} {
		includesHistory := scope == "history" || scope == "all"
		body := assertHistoryHTTP(t, endpoint, scope, includesHistory, rows, warnings, fatal)
		stdout := assertHistoryCLI(t, process, endpoint, factory, env, scope, includesHistory, rows, warnings, fatal)
		if strings.Contains(body+stdout, home) || strings.Contains(body+stdout, "planted-secret") {
			t.Fatal("listing leaked host path or content")
		}
	}
	if fatal {
		return
	}
	assertHistoryMCP(t, process, factory, env, rows, warnings)
	if len(warnings) > 0 && len(rows) == 0 {
		inputs := support.FakeInputs(t.Context(), []string{"you", "--server", endpoint, "session", "list", "--history-only"})
		inputs.Env, inputs.WorkingDirectory = env, factory
		if err := process.Execute(inputs.Input); err != nil {
			t.Fatal(err)
		}
		for _, path := range warnings {
			if !strings.Contains(inputs.Stdout(), path) {
				t.Fatalf("human output concealed %s: %s", path, inputs.Stdout())
			}
		}
	}
}

func assertHistoryHTTP(t *testing.T, endpoint, scope string, includesHistory bool, rows, warnings []string, fatal bool) string {
	t.Helper()
	response, err := http.Get(endpoint + "/factory-sessions?scope=" + scope)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if fatal && includesHistory {
		if response.StatusCode < 500 {
			t.Fatalf("scan failure status = %d: %s", response.StatusCode, body)
		}
		return string(body)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("HTTP %s = %d: %s", scope, response.StatusCode, body)
	}
	var result factoryapi.ListFactorySessionsResponse
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatal(err)
	}
	assertHistoryResult(t, result, includesHistory, rows, warnings)
	return string(body)
}

func assertHistoryCLI(t *testing.T, process support.Process, endpoint, factory string, env []string, scope string, includesHistory bool, rows, warnings []string, fatal bool) string {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), []string{"you", "--server", endpoint, "--json", "session", "list", "--scope", scope})
	inputs.Env, inputs.WorkingDirectory = env, factory
	err := process.Execute(inputs.Input)
	if fatal && includesHistory {
		if err == nil {
			t.Fatal("CLI concealed scan failure")
		}
		return inputs.Stdout()
	}
	if err != nil {
		t.Fatalf("CLI %s: %v; %s", scope, err, inputs.Stderr())
	}
	var result factoryapi.ListFactorySessionsResponse
	if err := json.Unmarshal([]byte(inputs.Stdout()), &result); err != nil {
		t.Fatalf("CLI JSON: %v: %s", err, inputs.Stdout())
	}
	assertHistoryResult(t, result, includesHistory, rows, warnings)
	return inputs.Stdout()
}

func assertHistoryResult(t *testing.T, result factoryapi.ListFactorySessionsResponse, includesHistory bool, rows, warnings []string) {
	t.Helper()
	var actualRows, actualWarnings []string
	if result.RecordedSessions != nil {
		for _, row := range *result.RecordedSessions {
			actualRows = append(actualRows, row.SessionId)
		}
	}
	if result.Warnings != nil {
		for _, warning := range *result.Warnings {
			if warning.Code != "UNREADABLE_RECORDING" || warning.Reason == "" || filepath.IsAbs(warning.ArtifactReference) {
				t.Fatalf("unsafe warning = %#v", warning)
			}
			actualWarnings = append(actualWarnings, warning.ArtifactReference)
		}
	}
	for _, live := range result.Sessions {
		if live.Id == "00000000-0000-4000-8000-000000000001" || live.Id == "00000000-0000-4000-8000-000000000002" {
			t.Fatal("recorded identity gained live authority")
		}
	}
	if !includesHistory {
		rows, warnings = nil, nil
	}
	if !reflect.DeepEqual(actualRows, rows) || !reflect.DeepEqual(actualWarnings, warnings) {
		t.Fatalf("rows=%v warnings=%v; want %v %v", actualRows, actualWarnings, rows, warnings)
	}
}

// H6 owns a populated durable inventory beside a live workspace and degraded
// history. This cell does not execute workers or call any provider.
func TestHistoryListingPreservesPopulatedScopes(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	factory := support.ScaffoldSingleStepFactory(t, "history-populated-scopes")
	recordingRoot := filepath.Join(home, ".you-agent-factory", "recordings")
	_, warnings := seedHistory(t, recordingRoot, "mixed")
	env := builtcliacceptance.ProcessEnvForIsolatedHome(home)
	var process support.Process
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: factory, Env: env, Edges: historyEdges(home, recordingRoot, "mixed"),
		BeforeStart: func(_ testing.TB, p support.Process, _ root.Input) { process = p },
	})
	body := `{"requestId":"history-durable-scope","source":{"kind":"INLINE_WORKFLOW","inlineWorkflow":{"inlineSource":{"encoding":"utf-8","inline":"return \"history-scope-result\";"}}}}`
	response, err := http.Post(server.URL()+"/factory-sessions/sync", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("durable start = %d %s %v", response.StatusCode, data, err)
	}
	var completed factoryapi.FactorySessionSyncExecutionResponse
	if err := json.Unmarshal(data, &completed); err != nil {
		t.Fatal(err)
	}
	if completed.Status != "SUCCEEDED" {
		t.Fatalf("durable status = %s", completed.Status)
	}
	for _, scope := range []string{"live", "persisted", "history", "all"} {
		response, err := http.Get(server.URL() + "/factory-sessions?scope=" + scope)
		if err != nil {
			t.Fatal(err)
		}
		var httpResult factoryapi.ListFactorySessionsResponse
		err = json.NewDecoder(response.Body).Decode(&httpResult)
		response.Body.Close()
		if err != nil || response.StatusCode != http.StatusOK {
			t.Fatalf("HTTP %s: %v %d", scope, err, response.StatusCode)
		}
		assertPopulatedScope(t, httpResult, scope, completed.SessionId, warnings)
		inputs := support.FakeInputs(t.Context(), []string{"you", "--server", server.URL(), "--json", "session", "list", "--scope", scope})
		inputs.Env, inputs.WorkingDirectory = env, factory
		if err := process.Execute(inputs.Input); err != nil {
			t.Fatalf("CLI %s: %v %s", scope, err, inputs.Stderr())
		}
		var cliResult factoryapi.ListFactorySessionsResponse
		if err := json.NewDecoder(bytes.NewBufferString(inputs.Stdout())).Decode(&cliResult); err != nil {
			t.Fatal(err)
		}
		assertPopulatedScope(t, cliResult, scope, completed.SessionId, warnings)
	}
}

func assertPopulatedScope(t *testing.T, result factoryapi.ListFactorySessionsResponse, scope, durableID string, warnings []string) {
	t.Helper()
	hasDurable := false
	if result.DurableSessions != nil {
		for _, row := range *result.DurableSessions {
			if row.SessionId == durableID {
				hasDurable = true
			}
		}
	}
	if hasDurable != (scope == "persisted" || scope == "all") {
		t.Fatalf("scope %s durable=%v: %#v", scope, hasDurable, result)
	}
	if (len(result.Sessions) > 0) != (scope == "live" || scope == "all") {
		t.Fatalf("scope %s live rows = %d", scope, len(result.Sessions))
	}
	assertPopulatedHistoryScope(t, result, scope, warnings)
}

func assertPopulatedHistoryScope(t *testing.T, result factoryapi.ListFactorySessionsResponse, scope string, warnings []string) {
	t.Helper()
	if scope == "history" || scope == "all" {
		if result.RecordedSessions == nil || len(*result.RecordedSessions) < 2 {
			t.Fatalf("scope %s lost healthy history", scope)
		}
		if result.Warnings == nil || len(*result.Warnings) != len(warnings) {
			t.Fatalf("scope %s lost warnings", scope)
		}
		for i, warning := range *result.Warnings {
			if warning.ArtifactReference != warnings[i] {
				t.Fatalf("warning %d = %#v", i, warning)
			}
		}
	} else if result.Warnings != nil || result.RecordedSessions != nil {
		t.Fatalf("scope %s included history", scope)
	}
}
