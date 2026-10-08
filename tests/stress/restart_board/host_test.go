package restart_board_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sync/atomic"
	"testing"
	"time"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

type boardHost struct {
	process    interface{ Execute(root.Input) error }
	repo, home string
	bound      chan string
	done       chan error
	runner     *boardRunner
	client     *http.Client
}

func newBoardHost(t *testing.T, repo, home string) *boardHost {
	t.Helper()
	h := &boardHost{repo: repo, home: home, bound: make(chan string, 1), runner: &boardRunner{}, client: &http.Client{Timeout: 30 * time.Second}}
	p, err := root.BuildProcess(t.Context(), serviceedges.Edges{
		ProviderCommandRunner: h.runner,
		BrowserOpener:         func(context.Context, string) error { return nil },
		APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
			server := httptest.NewServer(request.Handler)
			defer server.Close()
			if request.OnBound != nil {
				request.OnBound(platformhttpserver.Binding{Port: request.Port})
			}
			h.bound <- server.URL
			<-ctx.Done()
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	h.process = p
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := p.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	return h
}

func (h *boardHost) start(t *testing.T) (string, time.Time) {
	t.Helper()
	h.done = make(chan error, 1)
	started := time.Now()
	go func() {
		h.done <- h.process.Execute(root.Input{
			Args:    []string{"you", "run", "--continuously", "--with-server"},
			Env:     append(os.Environ(), "HOME="+h.home, "USERPROFILE="+h.home),
			Context: t.Context(), WorkingDirectory: h.repo,
			Stdin: bytes.NewReader(nil), Stdout: io.Discard, Stderr: io.Discard,
		})
	}()
	select {
	case baseURL := <-h.bound:
		return baseURL, started
	case err := <-h.done:
		t.Fatalf("command ended before serving: %+v", err)
	case <-time.After(2 * time.Minute):
		t.Fatal("no serving signal")
	}
	return "", started
}

func (h *boardHost) shutdown(t *testing.T, baseURL string) {
	t.Helper()
	h.request(t, http.MethodPost, baseURL+"/shutdown", []byte(`{}`), nil)
	select {
	case err := <-h.done:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("graceful shutdown did not join")
	}
}

func (h *boardHost) request(t *testing.T, method, endpoint string, body []byte, result any) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, endpoint, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := h.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		detail, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		t.Fatalf("%s %s: %d %s", method, endpoint, response.StatusCode, detail)
	}
	if result != nil {
		if err := json.NewDecoder(response.Body).Decode(result); err != nil {
			t.Fatal(err)
		}
	}
}

func (h *boardHost) readBoard(t *testing.T, baseURL string) map[string]factoryapi.Work {
	t.Helper()
	works := make(map[string]factoryapi.Work)
	cursor := ""
	for {
		var page factoryapi.ListWorkResponse
		h.request(t, http.MethodGet, baseURL+"/factory-sessions/~default/work?maxResults=250&nextToken="+url.QueryEscape(cursor), nil, &page)
		for _, w := range page.Results {
			if w.WorkId == nil {
				t.Fatal("Work missing ID")
			}
			if _, duplicate := works[*w.WorkId]; duplicate {
				t.Fatalf("duplicate Work %s", *w.WorkId)
			}
			works[*w.WorkId] = w
		}
		if page.PaginationContext == nil || page.PaginationContext.NextToken == nil {
			return works
		}
		next := *page.PaginationContext.NextToken
		if next == "" {
			return works
		}
		if next == cursor || len(page.Results) == 0 {
			t.Fatal("pagination did not advance")
		}
		cursor = next
	}
}

func (h *boardHost) waitConfirmed(t *testing.T, baseURL string, terminal int) map[string]factoryapi.Work {
	t.Helper()
	deadline := time.Now().Add(3 * time.Minute)
	// Confirmation is asynchronous. Observe the public boundary without sleeps;
	// neither provider completion nor admission alone proves durable Work facts.
	for time.Now().Before(deadline) {
		works := h.readBoard(t, baseURL)
		confirmed, completed := len(works) == boardSize, 0
		for _, w := range works {
			confirmed = confirmed && w.ConfirmationState != nil && *w.ConfirmationState == factoryapi.CONFIRMED
			if w.State != nil && w.State.Name == "complete" {
				completed++
			}
		}
		if confirmed && completed == terminal {
			return works
		}
	}
	t.Fatal("board did not reach CONFIRMED with expected terminal count")
	return nil
}

func (h *boardHost) assertReadyBoard(t *testing.T, baseURL string) {
	t.Helper()
	var status factoryapi.StatusResponse
	h.request(t, http.MethodGet, baseURL+"/factory-sessions/~default/status", nil, &status)
	var page factoryapi.ListWorkResponse
	h.request(t, http.MethodGet, baseURL+"/factory-sessions/~default/work?maxResults=1&counts=true", nil, &page)
	if status.TotalTokens != boardSize || status.Categories.Terminal != 1 || page.Counts == nil || page.Counts.Total != boardSize || len(page.Results) != 1 {
		t.Fatalf("serving host reported an incomplete board: status=%#v page=%#v", status, page)
	}
}

func (h *boardHost) logRetainedEvents(t *testing.T, baseURL string) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, baseURL+"/factory-sessions/~default/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := h.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	count := response.Header.Get(factorysessions.SessionEventStreamRetainedCountHeader)
	if response.StatusCode != http.StatusOK || count == "" {
		t.Fatal("missing public retained event count")
	}
	t.Logf("confirmed retained events=%s", count)
}

type boardRunner struct{ calls atomic.Int32 }

func (r *boardRunner) Run(_ context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	r.calls.Add(1)
	return platformprocess.CommandResult{Stdout: []byte("{\"type\":\"turn.started\"}\n{\"type\":\"item.completed\",\"item\":{\"id\":\"capacity\",\"type\":\"agent_message\",\"text\":\"COMPLETE § —\"}}\n{\"type\":\"turn.completed\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}\n")}, nil
}
