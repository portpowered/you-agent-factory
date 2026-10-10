package execution_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type browserHostRoute struct {
	ready       chan string
	opened      chan string
	failHost    bool
	failBrowser bool
}

// These continuous hosts share a process, but each owns its session, port selector,
// listener, browser outcome and output. Readiness and completion are observed
// before shutdown; the browser failure must leave customer Work runnable.
func TestHostedReadinessAndBrowserOutcome(t *testing.T) {
	t.Parallel()
	var routes sync.Map
	process := buildBrowserOutcomeProcess(t, &routes)
	for index, name := range []string{"B10-S", "B10-F", "B10-O", "B10-H"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			route := &browserHostRoute{ready: make(chan string, 1), opened: make(chan string, 2), failHost: name == "B10-H", failBrowser: name == "B10-F"}
			selector := 21000 + index
			routes.Store(selector, route)
			t.Cleanup(func() { routes.Delete(selector) })
			id, home, dir := uuid.NewString(), t.TempDir(), scaffoldBTRCOneShotFactory(t)
			flag := "--with-site"
			if name == "B10-O" {
				flag = "--with-server"
			}
			inputs := support.FakeInputs(t.Context(), []string{"you", "run", "--factory", dir + "/factory.json", "--session", id, "--continuously", "--no-record", flag, "--listen", fmt.Sprintf("127.0.0.1:%d", selector)})
			inputs.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
			inputs.WorkingDirectory = dir
			command := support.StartProcessCommand(t, process, inputs.Input)
			if route.failHost {
				assertRejectedBrowserHost(t, command, inputs.Stdout, inputs.Stderr)
				if len(route.ready) != 0 || len(route.opened) != 0 {
					t.Fatal("rejected host bound or opened a dashboard")
				}
				assertRejectedHostSessionReleased(t, process, &routes, selector+1000, id, dir, home)
				return
			}
			assertReadyBrowserHost(t, process, command, route, id, name, inputs)
		})
	}
}

func assertRejectedHostSessionReleased(t *testing.T, process support.Process, routes *sync.Map, selector int, rejectedID, dir, home string) {
	t.Helper()
	route := &browserHostRoute{ready: make(chan string, 1), opened: make(chan string, 1)}
	routes.Store(selector, route)
	t.Cleanup(func() { routes.Delete(selector) })
	inputs := support.FakeInputs(t.Context(), []string{"you", "run", "--dir", dir, "--session", uuid.NewString(), "--continuously", "--with-server", "--no-record", "--listen", fmt.Sprintf("127.0.0.1:%d", selector)})
	inputs.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	inputs.WorkingDirectory = dir
	command := support.StartProcessCommand(t, process, inputs.Input)
	var baseURL string
	select {
	case baseURL = <-route.ready:
	case <-command.Done():
		t.Fatalf("recovery host failed: %v", command.Err())
	case <-time.After(30 * time.Second):
		t.Fatal("recovery host did not bind")
	}
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, baseURL+"/factory-sessions/"+rejectedID+"/status", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("rejected session status = %d, want removed live owner", response.StatusCode)
	}
	command.Stop(t)
}

func assertRejectedBrowserHost(t *testing.T, command *support.ProcessCommand, stdout, stderr func() string) {
	t.Helper()
	select {
	case <-command.Done():
	case <-time.After(30 * time.Second):
		t.Fatal("rejected host did not terminate")
	}
	command.AcceptError()
	if command.Err() == nil {
		t.Fatal("rejected host succeeded")
	}
	diagnostic := requireStartupCLIDiagnostic(t, stderr())
	if string(diagnostic.Code) != "SERVER_START_FAILED" || !strings.Contains(diagnostic.Message, "requested server did not start") {
		t.Fatalf("host failure = %#v", diagnostic)
	}
	if stdout() != "" {
		t.Fatalf("rejected host announced readiness: %s", stdout())
	}
}

func buildBrowserOutcomeProcess(t *testing.T, routes *sync.Map) support.ApplicationProcess {
	t.Helper()
	var bound sync.Map
	return support.BuildProcess(t, serviceedges.Edges{
		ProviderCommandRunner: support.NewRecordingCommandRunner("hosted work COMPLETE"),
		APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
			value, ok := routes.Load(request.Port)
			if !ok {
				return fmt.Errorf("unknown hosted selector %d", request.Port)
			}
			route := value.(*browserHostRoute)
			if route.failHost {
				return errors.New("selected host rejected")
			}
			server := httptest.NewServer(request.Handler)
			defer server.Close()
			port := server.Listener.Addr().(*net.TCPAddr).Port
			bound.Store(port, route)
			defer bound.Delete(port)
			if request.OnBound != nil {
				request.OnBound(platformhttpserver.Binding{Port: port})
			}
			route.ready <- server.URL
			<-ctx.Done()
			return nil
		},
		BrowserOpener: func(_ context.Context, target string) error {
			parsed, err := url.Parse(target)
			if err != nil {
				return err
			}
			port, err := strconv.Atoi(parsed.Port())
			if err != nil {
				return err
			}
			value, ok := bound.Load(port)
			if !ok {
				return errors.New("browser opened before binding")
			}
			route := value.(*browserHostRoute)
			route.opened <- target
			if route.failBrowser {
				return errors.New("selected browser rejected")
			}
			return nil
		},
	})
}

func assertReadyBrowserHost(t *testing.T, process support.Process, command *support.ProcessCommand, route *browserHostRoute, id, name string, inputs *support.CapturedInputs) {
	t.Helper()
	var baseURL string
	select {
	case baseURL = <-route.ready:
	case <-command.Done():
		t.Fatalf("host ended before binding: %v", command.Err())
	case <-time.After(30 * time.Second):
		t.Fatal("host did not bind")
	}
	// A positional invocation owns its listener only until completion. Admit
	// through the ready continuous host so terminal reads precede owned shutdown.
	submitBrowserHostWork(t, process, baseURL, id, inputs)
	support.WaitForSessionTerminalStatus(t, baseURL, id, 30*time.Second)
	listed := support.GetJSON[factoryapi.ListWorkResponse](t, baseURL+"/factory-sessions/"+id+"/work")
	if support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "complete")) != 1 {
		t.Fatalf("hosted Work = %#v", listed.Results)
	}
	if name != "B10-O" {
		select {
		case target := <-route.opened:
			if target != baseURL+"/dashboard/ui" {
				t.Fatalf("browser target = %q, want %q", target, baseURL+"/dashboard/ui")
			}
		case <-time.After(30 * time.Second):
			t.Fatal("ready host did not open dashboard")
		}
	}
	command.Stop(t)
	if len(route.opened) != 0 {
		t.Fatal("unexpected extra browser launch")
	}
	output := inputs.Stdout() + inputs.Stderr()
	if !strings.Contains(output, baseURL) {
		t.Fatalf("startup omitted bound URL: %s", output)
	}
	if route.failBrowser && !strings.Contains(output, "Dashboard auto-open unavailable: selected browser rejected") {
		t.Fatalf("missing nonfatal browser diagnostic: %s", output)
	}
}

func submitBrowserHostWork(t *testing.T, process support.Process, baseURL, id string, host *support.CapturedInputs) {
	t.Helper()
	_, err := support.WaitForObservation(30*time.Second, func() (factoryapi.StatusResponse, error) {
		response, err := http.Get(baseURL + "/factory-sessions/" + id + "/status")
		if err != nil {
			return factoryapi.StatusResponse{}, err
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return factoryapi.StatusResponse{}, fmt.Errorf("session status: HTTP %d", response.StatusCode)
		}
		var status factoryapi.StatusResponse
		err = json.NewDecoder(response.Body).Decode(&status)
		return status, err
	}, func(status factoryapi.StatusResponse) bool { return status.FactoryState == "RUNNING" })
	if err != nil {
		t.Fatalf("hosted session did not become ready: %v", err)
	}
	path := filepath.Join(t.TempDir(), "payload.md")
	if err := os.WriteFile(path, []byte("hosted request"), 0o600); err != nil {
		t.Fatal(err)
	}
	inputs := support.FakeInputs(t.Context(), []string{"you", "--server", baseURL, "submit", "--session", id, "--name", "hosted-work", "--work-type-name", "task", "--payload", path})
	inputs.Env = host.Env
	inputs.WorkingDirectory = host.WorkingDirectory
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("submit hosted Work: %v; stderr=%s", err, inputs.Stderr())
	}
}
