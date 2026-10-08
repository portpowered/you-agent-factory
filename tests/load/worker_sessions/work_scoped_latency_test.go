package workersessions_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// This dedicated load lane consumes the invoking build's artifact. Fixture
// setup is excluded; each read includes launch and fully consumed output.
func TestWorkScopedCLIHTTPParityLatency(t *testing.T) {
	binary := os.Getenv("INFINITE_YOU_INTEGRATION_BINARY")
	if binary == "" {
		if os.Getenv("INFINITE_YOU_REQUIRE_PREBUILT_ARTIFACT") == "1" {
			t.Fatal("invoking build must supply INFINITE_YOU_INTEGRATION_BINARY")
		}
		t.Skip("requires prebuilt CLI artifact")
	}
	if !filepath.IsAbs(binary) {
		t.Fatal("prebuilt artifact path must be absolute")
	}
	artifact, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("artifact=%s sha256=%x", binary, sha256.Sum256(artifact))
	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Minute)
	defer cancel()
	f := newScopedLatencyFixture(t, ctx)
	target, err := url.Parse(f.serverURL)
	if err != nil {
		t.Fatal(err)
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	var mu sync.Mutex
	var calls []string
	observer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls = append(calls, r.Method+" "+r.URL.RequestURI())
		mu.Unlock()
		proxy.ServeHTTP(w, r)
	}))
	defer observer.Close()
	path := "/factory-sessions/" + f.sessionID + "/worker-sessions?workId=" + url.QueryEscape(f.workID)
	for sample := range 4 {
		mu.Lock()
		calls = nil
		mu.Unlock()
		started := time.Now()
		command := exec.CommandContext(ctx, binary, "--server", observer.URL, "--json", "--debug", "worker-sessions", "list", "--session", f.sessionID, "--work-id", f.workID, "--output", "json")
		command.Dir, command.Env = f.dir, f.environment
		var stdout, stderr bytes.Buffer
		command.Stdout, command.Stderr = &stdout, &stderr
		if err := command.Run(); err != nil {
			t.Fatalf("compiled list: %v stderr=%s", err, stderr.String())
		}
		elapsed := time.Since(started)
		mu.Lock()
		observed := append([]string(nil), calls...)
		mu.Unlock()
		if !reflect.DeepEqual(observed, []string{"GET " + path}) {
			t.Fatalf("all CLI HTTP calls=%v", observed)
		}
		httpRaw, httpElapsed, headerWait := scopedLatencyHTTP(t, ctx, f.serverURL+path)
		assertScopedLatencyParity(t, stdout.Bytes(), httpRaw, f.workID)
		t.Logf("sample=%d coldFirst=%t status=200 rows=200 calls=%v CLI-total=%s HTTP-total=%s HTTP-headers=%s delta=%s bytes=%d diagnostics=%s",
			sample, sample == 0, observed, elapsed, httpElapsed, headerWait, elapsed-httpElapsed, stdout.Len(), stderr.String())
		if elapsed > time.Second || elapsed-httpElapsed > time.Second {
			t.Errorf("sample=%d exceeded unchanged 1s budget: CLI=%s HTTP=%s delta=%s", sample, elapsed, httpElapsed, elapsed-httpElapsed)
		}
	}
}

func scopedLatencyHTTP(t *testing.T, ctx context.Context, endpoint string) ([]byte, time.Duration, time.Duration) {
	t.Helper()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	headerWait := time.Since(started)
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("scoped HTTP status=%d read=%v", response.StatusCode, err)
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	return raw, time.Since(started), headerWait
}

func assertScopedLatencyParity(t *testing.T, cli, httpRaw []byte, workID string) {
	t.Helper()
	var list factoryapi.ListWorkerSessionsResponse
	if err := json.Unmarshal(cli, &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Sessions) != 200 {
		t.Fatalf("rows=%d, want complete 200-attempt Work", len(list.Sessions))
	}
	for _, row := range list.Sessions {
		if row.WorkId == nil || *row.WorkId != workID || row.TokenUsage == nil {
			t.Fatalf("lost scoped captured facts: %#v", row)
		}
	}
	// CLI includes stable null keys; compare every other field recursively.
	var cliValue, httpValue any
	if err := json.Unmarshal(cli, &cliValue); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(httpRaw, &httpValue); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(withoutScopedNulls(cliValue), withoutScopedNulls(httpValue)) {
		t.Fatal("complete ordered CLI/HTTP observations differ")
	}
}

func withoutScopedNulls(value any) any {
	switch v := value.(type) {
	case map[string]any:
		for key, child := range v {
			if child == nil {
				delete(v, key)
			} else {
				v[key] = withoutScopedNulls(child)
			}
		}
	case []any:
		for index := range v {
			v[index] = withoutScopedNulls(v[index])
		}
	}
	return value
}

type scopedLatencyCommand struct{ next int }

func (c *scopedLatencyCommand) stdout() []byte {
	c.next++
	return []byte(fmt.Sprintf("{\"type\":\"thread.started\",\"thread_id\":\"load-attempt-%d\"}\n{\"type\":\"item.completed\",\"item\":{\"id\":\"answer\",\"type\":\"agent_message\",\"text\":\"COMPLETE\"}}\n{\"type\":\"turn.completed\",\"usage\":{\"input_tokens\":8,\"output_tokens\":12}}\n", c.next))
}

func scopedLatencyFactory(t *testing.T, dir string) {
	t.Helper()
	states := []any{map[string]any{"name": "ready", "type": "INITIAL"}, map[string]any{"name": "complete", "type": "TERMINAL"}, map[string]any{"name": "failed", "type": "FAILED"}}
	stations := make([]any, 0, 200)
	input := "ready"
	for index := range 200 {
		output := "complete"
		if index < 199 {
			output = fmt.Sprintf("stage-%d", index)
			states = append(states, map[string]any{"name": output, "type": "PROCESSING"})
		}
		name := fmt.Sprintf("step-%d", index)
		stations = append(stations, map[string]any{"name": name, "worker": "processor", "inputs": []any{map[string]any{"workType": "task", "state": input}}, "outputs": []any{map[string]any{"workType": "task", "state": output}}, "onFailure": []any{map[string]any{"workType": "task", "state": "failed"}}})
		path := filepath.Join(dir, "workstations", name)
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "AGENTS.md"), []byte("---\ntype: MODEL_WORKSTATION\n---\nComplete the Work.\n"), 0600); err != nil {
			t.Fatal(err)
		}
		input = output
	}
	raw, err := json.Marshal(map[string]any{"name": "scoped-latency", "workTypes": []any{map[string]any{"name": "task", "states": states}}, "workers": []any{map[string]any{"name": "processor"}}, "workstations": stations})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "factory.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	worker := filepath.Join(dir, "workers", "processor")
	if err := os.MkdirAll(worker, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worker, "AGENTS.md"), []byte(strings.Join([]string{"---", "type: MODEL_WORKER", "model: gpt-5-codex", "modelProvider: CODEX", "executorProvider: CODEX", "stopToken: COMPLETE", "---", "Complete the Work.", ""}, "\n")), 0600); err != nil {
		t.Fatal(err)
	}
}
