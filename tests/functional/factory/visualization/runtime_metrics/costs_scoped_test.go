package runtime_metrics_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformmetrics "github.com/portpowered/infinite-you/pkg/platform/metrics"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	costscli "github.com/portpowered/infinite-you/pkg/services/costs/transports/cli"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	generatedclient "github.com/portpowered/infinite-you/pkg/transports/http/client"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// One inert process supplies all these parallel sessions. Each invocation owns
// its listener, profile, factory and fault route; no global command lock is held.
func TestCostsScopedReportsAndQueryRecovery(t *testing.T) {
	t.Parallel()
	group := &costsProcessGroup{listeners: make(map[int]net.Listener), ready: make(map[int]chan struct{}), files: support.NewCostsSettingsFiles(platformfilesystem.Local{})}
	group.process = support.BuildProcess(t, serviceedges.Edges{
		APIServerStarter: group.serve, ProviderCommandRunner: costsProviderRunner{}, OperatorSettingsFileSystem: group.files,
	})
	support.CleanupProcess(t, group.process)
	for _, tc := range []struct{ name, model, status, amount string }{
		{"built-in", endToEndPricedModel, "PRICED", "21.25"},
		{"unknown-model", endToEndUnpricedModel, "UNPRICED", ""},
		{"operator-zero", endToEndPricedModel, "PRICED", "0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			group.checkPricing(t, tc.name, tc.model, tc.status, tc.amount)
		})
	}
	t.Run("settings failure and recovery", func(t *testing.T) {
		t.Parallel()
		group.checkSettingsRecovery(t)
	})
	t.Run("metrics root failure and recovery", func(t *testing.T) {
		t.Parallel()
		group.checkMetricsRecovery(t)
	})
}

func (group *costsProcessGroup) checkPricing(t *testing.T, name, model, status, amount string) {
	home := t.TempDir()
	if name == "operator-zero" {
		writeReplayOperatorPriceTable(t, home, operatorsettings.PriceTableModel{Provider: "CODEX", Model: model, InputPerMillionTokens: "0", OutputPerMillionTokens: "0"})
	}
	selected := group.start(t, home, model, platformmetrics.RuntimeMetricsRoot(home))
	foreign := group.start(t, t.TempDir(), endToEndPricedModel, platformmetrics.RuntimeMetricsRoot(home))
	selected.completeWork(t)
	foreign.completeWork(t)
	foreign.parity(t)
	report := selected.parity(t)
	if repeated := selected.parity(t); !reflect.DeepEqual(report, repeated) {
		t.Fatalf("non-deterministic selected report")
	}
	all := support.GetJSON[generatedclient.CostsReport](t, selected.url+"/metrics/costs")
	if all.Coverage.EncounteredRows != 2 {
		t.Fatalf("all-session report = %#v, want both rows", all)
	}
	if string(report.Status) != status || report.Coverage.EncounteredRows != 1 || len(report.LineItems) != 1 {
		t.Fatalf("selected report = %#v", report)
	}
	if report.Scope.FactorySessionId == nil || *report.Scope.FactorySessionId != selected.id || report.LineItems[0].FactorySessionId == nil || *report.LineItems[0].FactorySessionId != selected.id {
		t.Fatalf("foreign session leaked into selected scope: %#v", report)
	}
	if report.TokenTotals.TotalTokens == nil || *report.TokenTotals.TotalTokens != 3_000_000 {
		t.Fatalf("selected token totals = %#v", report.TokenTotals)
	}
	if report.TokenTotals.InputTokens == nil || *report.TokenTotals.InputTokens != 1_000_000 || report.TokenTotals.OutputTokens == nil || *report.TokenTotals.OutputTokens != 2_000_000 {
		t.Fatalf("token classes=%#v", report.TokenTotals)
	}
	if name == "built-in" && (report.LineItems[0].PriceSource == nil || string(*report.LineItems[0].PriceSource) != "BUILT_IN") {
		t.Fatalf("built-in source=%#v", report.LineItems[0])
	}
	if amount == "" {
		if report.KnownCost != nil {
			t.Fatalf("unpriced cost = %v", report.KnownCost)
		}
	} else if report.KnownCost == nil || *report.KnownCost != amount {
		t.Fatalf("exact cost = %v, want %s", report.KnownCost, amount)
	}
	if name == "operator-zero" && (report.LineItems[0].PriceSource == nil || string(*report.LineItems[0].PriceSource) != "OPERATOR_SUPPLIED") {
		t.Fatalf("explicit zero source = %#v", report.LineItems[0])
	}
	selected.assertFailure(t, "missing-session", http.StatusNotFound, "METRICS_SESSION_NOT_FOUND")
	selected.parity(t)
}
func (group *costsProcessGroup) checkSettingsRecovery(t *testing.T) {
	home := t.TempDir()
	fixture := group.start(t, home, endToEndPricedModel, platformmetrics.RuntimeMetricsRoot(home))
	fixture.completeWork(t)
	fixture.parity(t)
	path := filepath.Join(fixture.home, ".you-agent-factory", "config.json")
	group.files.SetReadFailure(path, errors.New("controlled settings failure secret-token private-path"))
	t.Cleanup(func() { group.files.SetReadFailure(path, nil) })
	fixture.assertFailure(t, fixture.id, http.StatusInternalServerError, "COSTS_QUERY_FAILED")
	group.files.SetReadFailure(path, nil)
	fixture.parity(t)
}
func (group *costsProcessGroup) checkMetricsRecovery(t *testing.T) {
	home := t.TempDir()
	fixture := group.start(t, home, endToEndPricedModel, platformmetrics.RuntimeMetricsRoot(home))
	empty := fixture.parity(t) // This host owns no Work and no active provider writer.
	if string(empty.Status) != "NO_USAGE" || empty.KnownCost != nil || empty.Coverage.EncounteredRows != 0 {
		t.Fatalf("empty report=%#v", empty)
	}
	root := platformmetrics.RuntimeMetricsRoot(fixture.home)
	backup := root + ".saved"
	if err := os.Rename(root, backup); err != nil {
		if runtime.GOOS == "windows" && errors.Is(err, os.ErrPermission) {
			t.Skipf("M08 unavailable: Windows denies renaming the active no-work metrics root: %v", err)
		}
		t.Fatal(err)
	}
	restored := false
	t.Cleanup(func() {
		if !restored {
			_ = os.Remove(root)
			_ = os.Rename(backup, root)
		}
	})
	if err := os.WriteFile(root, []byte("unavailable directory"), 0600); err != nil {
		t.Fatal(err)
	}
	fixture.assertFailure(t, fixture.id, http.StatusInternalServerError, "COSTS_QUERY_FAILED")
	if err := os.Remove(root); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(backup, root); err != nil {
		t.Fatal(err)
	}
	restored = true
	if recovered := fixture.parity(t); !reflect.DeepEqual(empty, recovered) {
		t.Fatalf("root recovery changed empty report: %#v", recovered)
	}
}

type costsProviderRunner struct{}

func (costsProviderRunner) Run(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return platformprocess.CommandResult{Stdout: support.CodexSuccessStdoutWithUsage("COMPLETE", 1_000_000, 2_000_000)}, nil
}

type costsProcessGroup struct {
	process   support.Process
	mu        sync.Mutex
	listeners map[int]net.Listener
	ready     map[int]chan struct{}
	files     *support.CostsSettingsFiles
}

func (group *costsProcessGroup) serve(ctx context.Context, request platformhttpserver.StartRequest) error {
	group.mu.Lock()
	listener, ready := group.listeners[request.Port], group.ready[request.Port]
	group.mu.Unlock()
	if listener == nil {
		return fmt.Errorf("missing owned listener for %d", request.Port)
	}
	if request.OnBound != nil {
		request.OnBound(platformhttpserver.Binding{Host: "127.0.0.1", Port: request.Port})
	}
	close(ready)
	return platformhttpserver.Serve(ctx, request.Handler, listener, request.Logger)
}

type costsSessionFixture struct {
	group              *costsProcessGroup
	id, home, dir, url string
	clientEnv          []string
}

func (group *costsProcessGroup) start(t *testing.T, home, model, metricsRoot string) costsSessionFixture {
	t.Helper()
	dir := support.ScaffoldSingleStepFactory(t, "scoped-costs")
	support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig("codex", model))
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	ready := make(chan struct{})
	group.mu.Lock()
	group.listeners[port] = listener
	group.ready[port] = ready
	group.mu.Unlock()
	t.Cleanup(func() {
		_ = listener.Close()
		group.mu.Lock()
		delete(group.listeners, port)
		delete(group.ready, port)
		group.mu.Unlock()
	})
	id := uuid.NewString()
	env := costsHomeEnvironment(home)
	inputs := support.FakeInputs(t.Context(), []string{"you", "run", "--dir", dir, "--session", id, "--continuously", "--with-server", "--listen", fmt.Sprintf("127.0.0.1:%d", port), "--quiet", "--no-record", "--runtime-metrics-dir", metricsRoot})
	inputs.Env = env
	inputs.WorkingDirectory = dir
	support.InitializeCustomerHomeWithProcess(t, group.process, env, dir)
	support.StartProcessCommand(t, group.process, inputs.Input)
	select {
	case <-ready:
	case <-time.After(30 * time.Second):
		t.Fatalf("host did not bind: %s", inputs.Stderr())
	}
	fixture := costsSessionFixture{group: group, id: id, home: home, dir: dir, url: fmt.Sprintf("http://127.0.0.1:%d", port), clientEnv: costsHomeEnvironment(t.TempDir())}
	support.InitializeCustomerHomeWithProcess(t, runtimeMetricsCLIProcess, fixture.clientEnv, dir)
	// A successful scoped status read establishes the public runtime boundary.
	support.GetJSON[factoryapi.StatusResponse](t, fixture.url+"/factory-sessions/"+id+"/status")
	return fixture
}
func (fixture costsSessionFixture) completeWork(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, support.SessionEventsURL(fixture.url, fixture.id), nil)
	stream, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Body.Close()
	if stream.StatusCode != http.StatusOK {
		t.Fatalf("event stream status=%d", stream.StatusCode)
	}
	response, err := http.Post(support.SessionWorkURL(fixture.url, fixture.id, "/work"), "application/json", strings.NewReader(`{"workTypeName":"task","payload":{"title":"costs"}}`))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("submit status=%d", response.StatusCode)
	}
	scanner := bufio.NewScanner(stream.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var event factoryapi.FactoryEvent
		if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &event); err != nil {
			t.Fatal(err)
		}
		if event.Type == factoryapi.FactoryEventTypeDispatchResponse {
			payload, err := event.Payload.AsDispatchResponseEventPayload()
			if err != nil || payload.Outcome != factoryapi.WorkOutcomeAccepted {
				t.Fatalf("completion=%#v error=%v", payload, err)
			}
			return
		}
	}
	t.Fatalf("no accepted completion: %v", scanner.Err())
}
func (fixture costsSessionFixture) cli(t *testing.T, id string) (string, error) {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), []string{"you", "--json", "--server", fixture.url, "metrics", "costs", "--session", id})
	// Client setup uses its own readable profile; faults target only server data.
	inputs.Env = fixture.clientEnv
	inputs.WorkingDirectory = fixture.dir
	err := runtimeMetricsCLIProcess.Execute(inputs.Input)
	return inputs.Stdout(), err
}
func (fixture costsSessionFixture) parity(t *testing.T) generatedclient.CostsReport {
	t.Helper()
	output, err := fixture.cli(t, fixture.id)
	if err != nil {
		t.Fatal(err)
	}
	var cli generatedclient.CostsReport
	if err := json.Unmarshal([]byte(output), &cli); err != nil {
		t.Fatal(err)
	}
	api := support.GetJSON[generatedclient.CostsReport](t, fixture.url+"/metrics/costs?session_id="+fixture.id)
	if !reflect.DeepEqual(cli, api) {
		t.Fatalf("CLI/HTTP differ: CLI=%#v HTTP=%#v", cli, api)
	}
	return cli
}
func (fixture costsSessionFixture) assertFailure(t *testing.T, id string, status int, code string) {
	t.Helper()
	response, err := http.Get(fixture.url + "/metrics/costs?session_id=" + id)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var body bytes.Buffer
	if _, err := body.ReadFrom(response.Body); err != nil {
		t.Fatal(err)
	}
	var failure factoryapi.ErrorResponse
	if err := json.Unmarshal(body.Bytes(), &failure); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != status || string(failure.Code) != code {
		t.Fatalf("failure status=%d body=%s", response.StatusCode, body.String())
	}
	if strings.Contains(body.String(), "secret-token") || strings.Contains(body.String(), "private-path") || strings.Contains(body.String(), "line_items") {
		t.Fatalf("unsafe/partial error body=%s", body.String())
	}
	output, err := fixture.cli(t, id)
	var typed *costscli.CostsError
	if !errors.As(err, &typed) || typed.Code != code || strings.TrimSpace(output) != "" {
		t.Fatalf("CLI failure=%v output=%q", err, output)
	}
}

func costsHomeEnvironment(home string) []string {
	env := []string{}
	for _, entry := range os.Environ() {
		key := strings.SplitN(entry, "=", 2)[0]
		if !strings.EqualFold(key, "HOME") && !strings.EqualFold(key, "USERPROFILE") && !strings.EqualFold(key, "HOMEDRIVE") && !strings.EqualFold(key, "HOMEPATH") {
			env = append(env, entry)
		}
	}
	drive := filepath.VolumeName(home)
	env = append(env, "HOME="+home, "USERPROFILE="+home, "HOMEDRIVE="+drive, "HOMEPATH="+strings.TrimPrefix(home, drive))
	return env
}
