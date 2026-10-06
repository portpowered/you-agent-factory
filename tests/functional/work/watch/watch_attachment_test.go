package watch_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// Only the loopback connection is substituted. All responses and retained
// cursors are emitted by the real Factory Session handlers and recording ledger.
const workWatchAttachmentTimeout = 30 * time.Second
const workWatchEventPath = "/factory-sessions/~default/events"

type selectedWatchDisconnectGate struct {
	server      *httptest.Server
	command     *support.ProcessCommand
	diagnostics *ledgerOutput
	requests    chan url.Values
	attached    chan struct{}
	count       atomic.Int64
	mu          sync.Mutex
	body        *selectedWatchBody
}

func newSelectedWatchDisconnectGate(t *testing.T, endpoint, session string) *selectedWatchDisconnectGate {
	t.Helper()
	target, err := url.Parse(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	gate := &selectedWatchDisconnectGate{requests: make(chan url.Values, 8), attached: make(chan struct{}, 8)}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ModifyResponse = func(response *http.Response) error {
		if response.Request.URL.Path == "/factory-sessions/"+session+"/events" && response.StatusCode == http.StatusOK {
			body := &selectedWatchBody{ReadCloser: response.Body}
			gate.mu.Lock()
			gate.body = body
			gate.mu.Unlock()
			response.Body = body
			gate.attached <- struct{}{}
		}
		return nil
	}
	gate.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/factory-sessions/"+session+"/events" {
			gate.count.Add(1)
			gate.requests <- r.URL.Query()
		}
		proxy.ServeHTTP(w, r)
	}))
	t.Cleanup(gate.server.Close)
	return gate
}

type selectedWatchBody struct {
	io.ReadCloser
	disconnected atomic.Bool
}

func (b *selectedWatchBody) Read(data []byte) (int, error) {
	n, err := b.ReadCloser.Read(data)
	if b.disconnected.Load() {
		return n, io.EOF
	}
	return n, err
}

func (g *selectedWatchDisconnectGate) disconnect() {
	g.mu.Lock()
	body := g.body
	g.mu.Unlock()
	body.disconnected.Store(true)
	_ = body.Close()
}

func (g *selectedWatchDisconnectGate) next(t *testing.T) url.Values {
	t.Helper()
	select {
	case <-g.command.Done():
		t.Fatalf("watch ended before attachment: %v: %s", g.command.Err(), g.diagnostics.String())
		return nil
	case query := <-g.requests:
		return query
	case <-time.After(selectedWatchCeiling):
		t.Fatal("public event stream did not attach")
		return nil
	}
}

func (g *selectedWatchDisconnectGate) assertCount(t *testing.T, want int64) {
	t.Helper()
	if got := g.count.Load(); got != want {
		t.Fatalf("event stream requests=%d want=%d", got, want)
	}
}

func runSelectedDurationIsolation(t *testing.T, selected, legacy *selectedWatchHost) {
	t.Helper()
	arrived, release := make(chan struct{}, 2), make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		arrived <- struct{}{}
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"works":[]}`)
	}))
	t.Cleanup(server.Close)
	commands := make([]*support.ProcessCommand, 0, 2)
	outputs := make([]*ledgerOutput, 0, 2)
	for _, host := range []*selectedWatchHost{selected, legacy} {
		stdout, stderr := newLedgerOutput(), newLedgerOutput()
		input := controlledWatchInput(t, t.Context(), server.URL, false, stdout, stderr)
		input.Args = []string{"you", "--server", server.URL, "--verbose", "--json", "work", "list", "--session", "duration-observation"}

		commands = append(commands, support.StartProcessCommand(t, host.process, input))
		outputs = append(outputs, stderr)
	}
	for range commands {
		select {
		case <-arrived:
		case <-time.After(selectedWatchCeiling):
			t.Fatal("duration request did not arrive")
		}
	}
	selectedWatchSource.millis.Add(37)
	close(release)
	for index, command := range commands {
		select {
		case <-command.Done():
		case <-time.After(selectedWatchCeiling):
			t.Fatal("duration command did not finish")
		}
		if err := command.Err(); err != nil {
			t.Fatalf("duration command: %v\n%s", err, outputs[index].String())
		}
		want := "durationMillis=37"
		if index == 1 {
			want = "durationMillis=0"
		}
		if !strings.Contains(outputs[index].String(), want) {
			t.Fatalf("selected process duration want %s: %s", want, outputs[index].String())
		}
	}
}
