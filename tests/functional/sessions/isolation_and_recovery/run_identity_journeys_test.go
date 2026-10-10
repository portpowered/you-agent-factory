package isolation_and_recovery_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const identityJourneyOutput = "session identity completion COMPLETE"

type identityJourney struct {
	id, dir, home, record string
	port                  int
	api                   *support.ProcessAPIServer
	calls                 atomic.Int32
}

// Explicit sessions share an immutable process. The default cohort has its own
// process because ~default is a single process-wide customer selector. Recovery
// deliberately constructs a fresh graph to prove reconstruction from recording.
func testRunSessionIdentityJourneys(t *testing.T) {
	t.Parallel()
	scenarios := []*identityJourney{}
	for index := range 2 {
		s := newIdentityJourney(t, 26001+index)
		scenarios = append(scenarios, s)
	}
	process := support.BuildProcess(t, identityJourneyEdges(t, scenarios))
	t.Run("explicit UUID and recording recovery", func(t *testing.T) {
		t.Parallel()
		testIdentityJourneyDispatch(t, process, scenarios[0], false, true)
	})
	t.Run("API generated identity and selector parity", func(t *testing.T) {
		t.Parallel()
		testIdentityJourneyParity(t, process, scenarios[1])
	})
	t.Run("omitted selector", func(t *testing.T) {
		t.Parallel()
		s := newIdentityJourney(t, 26003)
		defaultProcess := support.BuildProcess(t, identityJourneyEdges(t, []*identityJourney{s}))
		testIdentityJourneyDispatch(t, defaultProcess, s, true, false)
	})
}

func newIdentityJourney(t *testing.T, port int) *identityJourney {
	t.Helper()
	config := processExecuteRuntimeOpeningFactoryConfig()
	config["workTypes"].([]map[string]any)[0]["handlingBehavior"] = []string{"DEFAULT"}
	dir := support.ScaffoldFactory(t, config)
	support.WriteAgentConfig(t, dir, "worker-a", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
	return &identityJourney{id: uuid.NewString(), dir: dir, home: t.TempDir(), record: filepath.Join(dir, "source.jsonl"), port: port, api: support.NewProcessAPIServer()}
}

func identityJourneyEdges(t *testing.T, scenarios []*identityJourney) serviceedges.Edges {
	t.Helper()
	edges := serviceedges.Edges{APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
		for _, s := range scenarios {
			if request.Port == s.port {
				return s.api.Start(ctx, request)
			}
		}
		return fmt.Errorf("unowned listener port %d", request.Port)
	}}
	support.ConfigureWorkerCommands(t, &edges, identityJourneyRunner{scenarios: scenarios}, nil)
	return edges
}

type identityJourneyRunner struct{ scenarios []*identityJourney }

func (r identityJourneyRunner) Run(_ context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	for _, s := range r.scenarios {
		// Working directories are scenario-owned even when the API allocates a
		// fresh identity that was not known at process construction.
		if filepath.Clean(request.WorkDir) == filepath.Clean(s.dir) {
			s.calls.Add(1)
			return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout(identityJourneyOutput)}, nil
		}
	}
	return platformprocess.CommandResult{}, fmt.Errorf("unowned provider directory %q", request.WorkDir)
}

func identityJourneyInputs(t *testing.T, s *identityJourney, args []string) *support.CapturedInputs {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = append(os.Environ(), "HOME="+s.home, "USERPROFILE="+s.home)
	inputs.Input.WorkingDirectory = s.dir
	return inputs
}

func startIdentityJourney(t *testing.T, process support.Process, s *identityJourney, useDefault bool, extra ...string) (*support.ProcessCommand, string, string) {
	t.Helper()
	args := []string{"you", "run", "--dir", s.dir, "--continuously", "--with-server", "--server", "http://127.0.0.1:" + strconv.Itoa(s.port), "--quiet", "--record", s.record}
	if !useDefault {
		args = append(args, "--session", s.id)
	}
	args = append(args, extra...)
	inputs := identityJourneyInputs(t, s, args)
	command := support.StartProcessCommand(t, process, inputs.Input)
	ready := make(chan string, 1)
	go func() {
		base, _ := s.api.WaitForBaseURL(60 * time.Second)
		ready <- base
	}()
	var base string
	select {
	case <-command.Done():
		t.Fatalf("host exited before readiness: %v stderr=%s", command.Err(), inputs.Stderr())
	case base = <-ready:
		if base == "" {
			t.Fatalf("host did not become ready: %v stderr=%s", command.Err(), inputs.Stderr())
		}
	}
	id := s.id
	if useDefault {
		session := support.GetDefaultSession(t, base)
		id = session.Id
		if !session.IsDefault || !factorysessions.SessionIdentity(id).Valid() || id == factorysessions.DefaultSessionID {
			t.Fatalf("default session identity = %#v", session)
		}
	}
	return command, base, id
}

func testIdentityJourneyDispatch(t *testing.T, process support.Process, s *identityJourney, useDefault, recover bool) {
	t.Helper()
	command, base, id := startIdentityJourney(t, process, s, useDefault)
	stream := support.OpenFactoryEventStreamAt(t, support.SessionEventsURL(base, id))
	defer stream.Close()
	batch := `{"requestId":"identity-request","type":"FACTORY_REQUEST_BATCH","works":[{"workId":"identity-work","name":"identity","workTypeName":"task","state":"init","content":[{"type":"text","text":"complete identity work"}]}]}`
	inputs := identityJourneyInputs(t, s, []string{"you", "--server", base, "submit", "batch", "--session", id, batch})
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("submit: %v stderr=%s", err, inputs.Stderr())
	}
	events := readRootProcessEventsUntilDispatchResponse(t, stream)
	dispatches := support.ObserveDispatchEvents(t, events)
	if len(dispatches) != 1 || dispatches[0].Response == nil || dispatches[0].Response.Outcome != factoryapi.WorkOutcomeAccepted {
		t.Fatalf("dispatch = %#v", dispatches)
	}
	assertIdentityJourneyWork(t, base, id)
	if s.calls.Load() != 1 || command.Err() != nil {
		t.Fatalf("calls=%d host error=%v", s.calls.Load(), command.Err())
	}
	stream.Close()
	command.Stop(t)
	if recover {
		testIdentityJourneyRecovery(t, s)
	}
}

func assertIdentityJourneyWork(t *testing.T, base, id string) {
	t.Helper()
	works := support.GetJSON[factoryapi.ListWorkResponse](t, support.SessionWorkURL(base, id, "/work"))
	if !support.HasWorkAtCustomerState(works, "identity-work", "task:complete") {
		t.Fatalf("completed Work = %#v", works)
	}
	payload, err := json.Marshal(works)
	if err != nil || !strings.Contains(string(payload), identityJourneyOutput) {
		t.Fatalf("Work output=%s err=%v", payload, err)
	}
}

func testIdentityJourneyRecovery(t *testing.T, source *identityJourney) {
	t.Helper()
	prefix, err := os.ReadFile(source.record)
	if err != nil || len(prefix) == 0 {
		t.Fatalf("source recording: %v", err)
	}
	s := &identityJourney{id: uuid.NewString(), dir: source.dir, home: source.home, record: filepath.Join(source.dir, "successor.jsonl"), port: 26004, api: support.NewProcessAPIServer()}
	edges := identityJourneyEdges(t, []*identityJourney{s})
	edges.FactorySessionsWorkingDirectory = platformfilesystem.Local{WorkingDirectory: source.dir}
	process := support.BuildProcess(t, edges)
	command, base, id := startIdentityJourney(t, process, s, false, "--resume", source.record)
	assertIdentityJourneyWork(t, base, id)
	command.Stop(t)
	if s.calls.Load() != 0 {
		t.Fatalf("recovery redispatched %d completed Work", s.calls.Load())
	}
	after, err := os.ReadFile(source.record)
	if err != nil || !bytes.Equal(prefix, after) {
		t.Fatalf("resume modified source: %v", err)
	}
	if successor, err := os.ReadFile(s.record); err != nil || len(successor) == 0 {
		t.Fatalf("successor recording: %v", err)
	}
}

func testIdentityJourneyParity(t *testing.T, process support.Process, s *identityJourney) {
	t.Helper()
	command, base, id := startIdentityJourney(t, process, s, false)
	defer command.Stop(t)
	for _, selector := range []string{"validation-factory", "../escape", uuid.NewString()} {
		status := http.StatusBadRequest
		if factorysessions.SessionIdentity(selector).Valid() {
			status = http.StatusNotFound
		}
		endpoint := base + "/factory-sessions/" + url.PathEscape(selector) + "/invocations"
		response, err := http.Post(endpoint, "application/json", strings.NewReader(`{"sourceKind":"text","content":[{"type":"text","text":"identity invocation"}]}`))
		if err != nil {
			t.Fatal(err)
		}
		var diagnostic factoryapi.ErrorResponse
		err = json.NewDecoder(response.Body).Decode(&diagnostic)
		response.Body.Close()
		if err != nil || response.StatusCode != status {
			t.Fatalf("API selector=%q status=%d diagnostic=%#v err=%v", selector, response.StatusCode, diagnostic, err)
		}
		if status == http.StatusBadRequest && (diagnostic.Code != "BAD_REQUEST" || !strings.Contains(diagnostic.Message, "sessionId") || !strings.Contains(diagnostic.Message, factorysessions.SessionIdentityForm)) {
			t.Fatalf("API diagnostic = %#v", diagnostic)
		}
		inputs := identityJourneyInputs(t, s, []string{"you", "--remote", "--server", base, "run", "--factory", filepath.Join(s.dir, "factory.json"), "--session", selector, "identity invocation"})
		if err := process.Execute(inputs.Input); err == nil {
			t.Fatalf("remote selector %q admitted", selector)
		}
		if status == http.StatusBadRequest {
			if err := json.Unmarshal([]byte(inputs.Stderr()), &diagnostic); err != nil || diagnostic.Code != "BAD_REQUEST" || diagnostic.Message != "--session must be "+factorysessions.SessionIdentityForm {
				t.Fatalf("remote diagnostic = %q err=%v", inputs.Stderr(), err)
			}
		} else {
			// The existing remote CLI envelope reports the HTTP 404 inside its
			// remote-operation error; HTTP itself retains typed NOT_FOUND.
			if err := json.Unmarshal([]byte(inputs.Stderr()), &diagnostic); err != nil || diagnostic.Code != "REMOTE_DURABLE_START_FAILED" || !strings.Contains(diagnostic.Message, "(404): factory session not found") {
				t.Fatalf("missing selector diagnostic = %s err=%v", inputs.Stderr(), err)
			}
		}
	}
	works := support.GetJSON[factoryapi.ListWorkResponse](t, support.SessionWorkURL(base, id, "/work"))
	if len(works.Results) != 0 || s.calls.Load() != 0 {
		t.Fatalf("invalid/missing selectors caused Work/provider effects: %#v calls=%d", works, s.calls.Load())
	}
	opened := postSessionsJSON[factoryapi.OpenFactorySessionResponse](t, base+"/factory-sessions", factoryapi.OpenFactorySessionRequest{FolderPath: s.dir}, "open generated session")
	if opened.Session == nil || !factorysessions.SessionIdentity(opened.Session.Id).Valid() {
		t.Fatalf("API opened identity = %#v", opened)
	}
	result := postSessionsJSON[factoryapi.InvocationResponse](t, base+"/factory-sessions/"+opened.Session.Id+"/invocations", map[string]any{"sourceKind": "text", "timeoutMillis": 30000, "content": []map[string]string{{"type": "text", "text": "API identity invocation"}}}, "invoke generated session")
	if result.Status != factoryapi.InvocationTerminalStatusCompleted || s.calls.Load() != 1 {
		t.Fatalf("API result=%#v calls=%d", result, s.calls.Load())
	}
	assertInvocationPrimaryResultText(t, result, identityJourneyOutput)
	listed := support.GetJSON[factoryapi.ListWorkResponse](t, support.SessionWorkURL(base, opened.Session.Id, "/work"))
	if len(listed.Results) != 1 || listed.Results[0].State == nil || listed.Results[0].State.Name != "complete" {
		t.Fatalf("API generated completed Work = %#v", listed)
	}
}
