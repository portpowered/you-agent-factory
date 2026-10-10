package worklist_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/pkg/initializer/application"
	platformhttp "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	"github.com/portpowered/infinite-you/pkg/root"
	"github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// Customer capacity contract: one process, one non-dispatching 1,000-item board,
// ten complete 100-row HTTP reads and one malformed cursor, within five minutes.
// Fixture admission is excluded; the first page has no list warmup or retry.
func TestWorkListPageLargeBoard(t *testing.T) {
	baseURL, sessionID := largeBoardHost(t)
	endpoint := baseURL + "/factory-sessions/" + sessionID
	expected := seedLargeBoard(t, endpoint)
	walkLargeBoard(t, endpoint, expected)
	assertMalformedCursor(t, endpoint)
}

func seedLargeBoard(t *testing.T, endpoint string) map[string]bool {
	t.Helper()
	works := make([]map[string]any, 1000)
	expected := make(map[string]bool, len(works))
	for i := range works {
		id := fmt.Sprintf("large-board-%04d", i)
		works[i] = map[string]any{"workId": id, "name": id, "workTypeName": "task", "payload": map[string]int{"index": i}}
		expected[id] = true
	}
	body, err := json.Marshal(map[string]any{"requestId": "large-board", "type": "FACTORY_REQUEST_BATCH", "works": works})
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	status, admittedBody, _ := boardRequest(t, http.MethodPut, endpoint+"/work-requests/large-board", body)
	if status != http.StatusCreated && status != http.StatusOK {
		t.Fatalf("admission HTTP %d: %s", status, admittedBody)
	}
	t.Logf("public batch admission rows=1000 elapsed=%s", time.Since(started))
	return expected
}

func walkLargeBoard(t *testing.T, endpoint string, expected map[string]bool) {
	t.Helper()
	seen := make(map[string]bool, len(expected))
	var cursor string
	var returnedCursorIDs []string
	walkStarted := time.Now()
	for page := 1; page <= 10; page++ {
		query := url.Values{"includeSuperseded": {"true"}, "counts": {"true"}, "maxResults": {"100"}}
		if cursor != "" {
			query.Set("nextToken", cursor)
		}
		status, data, elapsed := boardRequest(t, http.MethodGet, endpoint+"/work?"+query.Encode(), nil)
		if status != http.StatusOK {
			t.Fatalf("page %d HTTP %d", page, status)
		}
		var result factoryapi.ListWorkResponse
		if err := json.Unmarshal(data, &result); err != nil {
			t.Fatal(err)
		}
		assertLargeBoardPage(t, page, result, expected, seen)
		for _, item := range result.Results {
			var index int
			_, _ = fmt.Sscanf(*item.WorkId, "large-board-%d", &index)
			returnedCursorIDs = append(returnedCursorIDs, fmt.Sprintf("tok-task-%d", index+1))
		}
		next := ""
		if result.PaginationContext.NextToken != nil {
			next = *result.PaginationContext.NextToken
		}
		assertPageCursor(t, page, next, cursor, returnedCursorIDs[len(returnedCursorIDs)-1])
		cursor = next
		t.Logf("page=%d rows=%d total=%d bytes=%d complete_body=%s", page, len(result.Results), result.Counts.Total, len(data), elapsed)
		// Explicit customer latency contract; dedicated load lane only.
		if elapsed >= 2*time.Second {
			t.Errorf("page %d took %s; must be <2s", page, elapsed)
		}
	}
	t.Logf("whole HTTP walk=%s unique_ids=%d", time.Since(walkStarted), len(seen))
	if len(seen) != len(expected) {
		t.Fatalf("walk returned %d unique IDs", len(seen))
	}
	ordered := slices.Clone(returnedCursorIDs)
	sort.Strings(ordered)
	if !slices.Equal(returnedCursorIDs, ordered) {
		t.Fatal("same-state pages are not in canonical cursor order")
	}
}

func assertMalformedCursor(t *testing.T, endpoint string) {
	t.Helper()
	status, data, elapsed := boardRequest(t, http.MethodGet, endpoint+"/work?includeSuperseded=true&counts=true&maxResults=100&nextToken=%25%25%25", nil)
	var failure factoryapi.ErrorResponse
	if err := json.Unmarshal(data, &failure); err != nil {
		t.Fatal(err)
	}
	if status != http.StatusBadRequest || string(failure.Code) != "BAD_REQUEST" || string(failure.Family) != "BAD_REQUEST" || !strings.Contains(failure.Message, "nextToken is invalid") {
		t.Fatalf("invalid cursor: HTTP %d %#v", status, failure)
	}
	t.Logf("malformed cursor status=%d code=%s complete_body=%s", status, failure.Code, elapsed)
	if elapsed >= 2*time.Second {
		t.Errorf("malformed cursor took %s; must be <2s", elapsed)
	}
}

func assertLargeBoardPage(t *testing.T, page int, result factoryapi.ListWorkResponse, expected, seen map[string]bool) {
	t.Helper()
	if len(result.Results) != 100 || result.Counts == nil || result.Counts.Total != 1000 || result.PaginationContext == nil || result.PaginationContext.MaxResults != 100 {
		t.Fatalf("page %d wrong cardinality/count/pagination", page)
	}
	for _, item := range result.Results {
		if item.WorkId == nil || !expected[*item.WorkId] || seen[*item.WorkId] {
			t.Fatalf("page %d unexpected or duplicate identity", page)
		}
		seen[*item.WorkId] = true
	}
}

func boardRequest(t *testing.T, method, endpoint string, body []byte) (int, []byte, time.Duration) {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), method, endpoint, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	started := time.Now()
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(response.Body)
	response.Body.Close()
	elapsed := time.Since(started)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode >= 500 {
		t.Fatalf("HTTP %d: %s", response.StatusCode, data)
	}
	return response.StatusCode, data, elapsed
}

func largeBoardHost(t *testing.T) (string, string) {
	t.Helper()
	dir, home, sessionID := t.TempDir(), t.TempDir(), uuid.NewString()
	config := `{"name":"large-board","workTypes":[{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"},{"name":"failed","type":"FAILED"}]}],"workers":[],"workstations":[]}`
	if err := os.WriteFile(filepath.Join(dir, "factory.json"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	ready := make(chan http.Handler, 1)
	process, err := root.BuildProcess(t.Context(), edges.Edges{
		FactorySessionResolveHomeDirectory: func() (string, error) { return home, nil },
		APIServerStarter: func(ctx context.Context, req platformhttp.StartRequest) error {
			ready <- req.Handler
			if req.OnBound != nil {
				req.OnBound(platformhttp.Binding{Port: req.Port})
			}
			<-ctx.Done()
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = process.Close(context.Background()) })
	env := builtcliacceptance.ProcessEnvForIsolatedHome(home)
	// Public preflight initializes the isolated profile before hosting.
	_ = process.Execute(application.Input{Args: []string{"you", "run", "--no-record", "--factory", filepath.Join(dir, "missing.json")}, Env: env, WorkingDirectory: dir, Context: t.Context(), Stdout: io.Discard, Stderr: io.Discard})
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	var hostErr error
	go func() {
		hostErr = process.Execute(application.Input{Args: []string{"you", "run", "--no-record", "--dir", dir, "--session", sessionID, "--continuously", "--with-server", "--quiet"}, Env: env, WorkingDirectory: dir, Context: ctx, Stdout: io.Discard, Stderr: io.Discard})
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
		if hostErr != nil && !errors.Is(hostErr, context.Canceled) {
			t.Errorf("host cleanup: %v", hostErr)
		}
	})
	select {
	case handler := <-ready:
		server := httptest.NewServer(handler)
		t.Cleanup(server.Close)
		return server.URL, sessionID
	case <-done:
		t.Fatalf("host exited before readiness: %v", hostErr)
	case <-t.Context().Done():
		t.Fatal("host readiness canceled")
	}
	return "", ""
}

func assertPageCursor(t *testing.T, page int, next, previous, lastID string) {
	t.Helper()
	if page == 10 {
		if next != "" {
			t.Fatal("final page has continuation")
		}
		return
	}
	decoded, err := base64.StdEncoding.DecodeString(next)
	if err != nil || next == "" || next == previous || string(decoded) != lastID {
		t.Fatalf("page %d invalid continuation", page)
	}
}
