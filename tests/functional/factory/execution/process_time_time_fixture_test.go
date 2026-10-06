package execution_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/services/automations"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

type timeWait struct {
	delay   time.Duration
	channel <-chan time.Time
}

// Wall observations intentionally expose no replay SetTick capability. Recorded
// runtimes own their logical ticks; the journey owns process wall advancement.
type journeyWall struct{ source platformclock.Source }

func (w journeyWall) Now() time.Time { return w.source.Now() }

// Only startup readiness polls are driven at registration. Source After waits
// are registered separately and held until the journey advances their source.
type journeyScheduler struct {
	*platformclock.Deterministic
	waits chan timeWait
	base  time.Time
}

func newJourneyScheduler(base time.Time) *journeyScheduler {
	return &journeyScheduler{Deterministic: platformclock.NewDeterministic(base, time.Second), waits: make(chan timeWait, 64), base: base}
}

func (s *journeyScheduler) NewTimer(delay time.Duration) platformclock.Timer {
	if delay == 10*time.Millisecond {
		readiness := platformclock.NewDeterministic(s.Now(), delay)
		timer := readiness.NewTimer(delay)
		readiness.SetTick(1)
		return timer
	}
	return s.Deterministic.NewTimer(delay)
}

func (s *journeyScheduler) After(delay time.Duration) <-chan time.Time {
	channel := s.Deterministic.After(delay)
	s.waits <- timeWait{delay: delay, channel: channel}
	return channel
}

func (s *journeyScheduler) await(t *testing.T, delay time.Duration) timeWait {
	t.Helper()
	select {
	case wait := <-s.waits:
		if wait.delay != delay {
			t.Fatalf("registered delay = %s, want %s", wait.delay, delay)
		}
		return wait
	case <-time.After(30 * time.Second):
		t.Fatal("source wait not registered")
		return timeWait{}
	}
}

type journeyReply struct {
	status int
	body   string
	header http.Header
}
type journeyCall struct {
	request *http.Request
	reply   chan journeyReply
	done    chan struct{}
}
type journeyRoute struct{ calls chan journeyCall }
type journeyHTTP struct{ routes map[string]*journeyRoute }

func (h journeyHTTP) Do(request *http.Request) (*http.Response, error) {
	key := strings.TrimPrefix(request.Header.Get("Authorization"), "source-time-credential-")
	if key == "" {
		key = strings.TrimPrefix(request.URL.Path, "/")
	}
	route := h.routes[key]
	if route == nil {
		return nil, fmt.Errorf("unexpected source route")
	}
	call := journeyCall{request: request, reply: make(chan journeyReply, 1), done: make(chan struct{})}
	defer close(call.done)
	select {
	case route.calls <- call:
	case <-request.Context().Done():
		return nil, request.Context().Err()
	}
	select {
	case reply := <-call.reply:
		return &http.Response{StatusCode: reply.status, Header: reply.header, Body: io.NopCloser(strings.NewReader(reply.body)), Request: request}, nil
	case <-request.Context().Done():
		return nil, request.Context().Err()
	}
}

func (r *journeyRoute) await(t *testing.T) journeyCall {
	t.Helper()
	select {
	case call := <-r.calls:
		return call
	case <-time.After(30 * time.Second):
		t.Fatal("hosted HTTP not reached")
		return journeyCall{}
	}
}

func (r *journeyRoute) assertHeld(t *testing.T) {
	t.Helper()
	select {
	case <-r.calls:
		t.Fatal("HTTP progressed before selected scheduling")
	default:
	}
}

type timeCohort struct {
	url             string
	wall            *platformclock.Deterministic
	process, hosted *journeyScheduler
	routes          map[string]*journeyRoute
	dirs            map[string]string
	lateAdmissions  atomic.Int32
	logs            *observer.ObservedLogs
	cli             support.Process
	env             []string
	webhook         *journeyScheduler
	webhookEffects  *webhookEffects
}

func startTimeCohort(t *testing.T, specialized bool) *timeCohort {
	t.Helper()
	base := time.Date(2040, 1, 1, 0, 0, 0, 0, time.UTC)
	c := &timeCohort{wall: platformclock.NewDeterministic(base, time.Second), process: newJourneyScheduler(base.Add(24 * time.Hour)), routes: map[string]*journeyRoute{}, dirs: map[string]string{}}
	c.hosted = c.process
	edges := serviceedges.Edges{Clock: journeyWall{source: c.wall}, ProcessScheduler: c.process}
	core, logs := observer.New(zap.InfoLevel)
	c.logs = logs
	edges.ProcessLogger = zap.New(core)
	if specialized {
		c.hosted = newJourneyScheduler(base.Add(48 * time.Hour))
		edges.HostedClock = c.hosted
		edges.FactoryDefinitionClock = platformclock.NewDeterministic(base.Add(72*time.Hour), time.Second)
	}
	configureWebhookEffects(t, c, &edges, specialized)
	for _, key := range []string{"periodic", "retry", "blocked", "backoff", "peer"} {
		c.routes[key] = &journeyRoute{calls: make(chan journeyCall, 4)}
		config := hostedTimeConfig(key)
		c.dirs[key] = support.ScaffoldFactory(t, config)
		support.ClearSeedInputs(t, c.dirs[key])
	}
	edges.HostedHTTPClient = journeyHTTP{routes: c.routes}
	edges.SubmissionRecorder = func(record work.FactorySubmissionRecord) {
		if record.Request.WorkID == "linear:issue-closed-only" {
			c.lateAdmissions.Add(1)
		}
	}
	edges.HostedLinearEndpoint = "https://source-time.invalid/graphql"
	edges.HostedSecretResolver = func(_ context.Context, _ automations.HostedRuntimePaths, ref string) (string, error) {
		return "source-time-credential-" + ref, nil
	}
	return startTimeHost(t, c, edges)
}

func startTimeHost(t *testing.T, c *timeCohort, edges serviceedges.Edges) *timeCohort {
	t.Helper()
	idle := support.ScaffoldFactory(t, idleTimeConfig())
	support.ClearSeedInputs(t, idle)
	api := support.NewProcessAPIServer()
	edges.APIServerStarter = api.Start
	process := support.BuildProcess(t, edges)
	c.cli = process
	home := t.TempDir()
	inputs := support.FakeInputs(t.Context(), []string{"you", "run", "--dir", idle, "--session", uuid.NewString(), "--continuously", "--with-server", "--quiet", "--no-record"})
	inputs.WorkingDirectory = idle
	inputs.Env = []string{"HOME=" + home, "USERPROFILE=" + home, "APPDATA=" + filepath.Join(home, "appdata"), "LOCALAPPDATA=" + filepath.Join(home, "localappdata"), "XDG_CONFIG_HOME=" + filepath.Join(home, "config"), "XDG_CACHE_HOME=" + filepath.Join(home, "cache"), "XDG_STATE_HOME=" + filepath.Join(home, "state"), "XDG_DATA_HOME=" + filepath.Join(home, "data")}
	c.env = inputs.Env

	command := support.StartProcessCommand(t, process, inputs.Input)
	t.Cleanup(func() { command.Stop(t) })
	c.url = api.WaitForURL(t)
	return c
}

func idleTimeConfig() map[string]any {
	return map[string]any{"name": "source-time", "workTypes": []map[string]any{{"name": "story", "states": []map[string]string{{"name": "init", "type": "INITIAL"}, {"name": "queued", "type": "TERMINAL"}, {"name": "failed", "type": "FAILED"}}}}}
}

func hostedTimeConfig(key string) map[string]any {
	config := idleTimeConfig()
	config["workers"] = []map[string]any{{"name": "linear-poller", "type": "HOSTED_WORKER", "provider": "LINEAR", "auth": map[string]string{"secretRef": key}, "linear": map[string]any{"pollInterval": "10s", "mapping": map[string]string{"workType": "story", "state": "queued"}}}}
	config["workstations"] = []map[string]any{{"name": "poll-linear", "behavior": "POLLER", "worker": "linear-poller", "inputs": []map[string]string{{"workType": "story", "state": "init"}}, "outputs": []map[string]string{{"workType": "story", "state": "queued"}}, "onFailure": []map[string]string{{"workType": "story", "state": "failed"}}}}
	return config
}

func (c *timeCohort) open(t *testing.T, key string) string {
	t.Helper()
	id := support.OpenFactorySessionAt(t, c.url, c.dirs[key]).Session.Id
	if id == "~default" {
		t.Fatal("expected explicit session")
	}
	return id
}

func factoryURL(base, session string) string {
	return base + "/factory-sessions/" + url.PathEscape(session) + "/factory"
}

func putTimeFactory(t *testing.T, base, session string, factory factoryapi.Factory, status int) []byte {
	t.Helper()
	body, err := json.Marshal(factoryapi.SaveFactoryForSessionRequest{Factory: factory})
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPut, factoryURL(base, session), bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != status {
		t.Fatalf("save status = %d, want %d: %s", response.StatusCode, status, data)
	}
	return data
}
