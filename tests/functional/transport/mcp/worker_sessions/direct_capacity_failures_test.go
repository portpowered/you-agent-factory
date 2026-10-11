package workersessions_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestDirectCapacityOpeningFailureReleases(t *testing.T) {
	t.Parallel()
	client := newSelectedHostClientProcess(t)
	runner := &joinedCapacityProvider{outcome: "success", started: make(chan struct{}, 2), canceled: make(chan struct{}), release: make(chan struct{}), replacement: make(chan struct{})}
	writer := &capacityOpeningWriter{}
	host, dir, _ := newCapacityProfileHost(t, runner, `{"defaults":{"directSessionCapacity":1}}`, writer)
	t.Cleanup(runner.unblock)
	session, ctx := startMCP(t, client, host.URL())
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	failed := callTool(t, ctx, session, "you.subagent", capacityLifetimeArgs("opening-failure", dir))
	assertToolError(t, failed, "worker_session.unavailable", true)
	response := postHostJSON(t, ctx, host.URL()+"/worker-sessions", capacityLifetimeArgs("opening-failure", dir), 503).(map[string]any)
	if response["code"] != "WORKER_SESSION_START_OPENING_FAILED" || runner.calls.Load() != 0 {
		t.Fatalf("opening failure code=%v provider calls=%d", response["code"], runner.calls.Load())
	}
	assertSubagentAdmission(t, callTool(t, ctx, session, "you.subagent", capacityLifetimeArgs("replacement", dir)), "replacement")
	waitCapacityGate(t, ctx, runner.started)
	assertToolError(t, callTool(t, ctx, session, "you.subagent", capacityLifetimeArgs("excess", dir)), "worker_session.capacity_exceeded", true)
	runner.unblock()
	followCapacityTerminal(t, ctx, client, host.URL(), dir, "replacement", "COMPLETED")
}

func TestDirectCapacityInvalidSavedSetting(t *testing.T) {
	t.Parallel()
	client := newSelectedHostClientProcess(t)
	var calls atomic.Int32
	runner := capacityProvider{launched: make(chan platformprocess.CommandRequest, 1), released: make(chan struct{}), calls: &calls}
	host, dir, home := newCapacityProfileHost(t, runner, `{"defaults":{"directSessionCapacity":1}}`, nil)
	// Startup also validates the document. Publish the invalid saved setting
	// after readiness, before this host's first direct admission resolution.
	if err := os.WriteFile(filepath.Join(home, ".you-agent-factory", "config.json"), []byte(`{"defaults":{"directSessionCapacity":0}}`), 0600); err != nil {
		t.Fatal(err)
	}
	baseline := capacityFactoryIDs(t, host.URL())
	session, ctx := startMCP(t, client, host.URL())
	args := capacityLifetimeArgs("invalid-setting", dir)
	assertToolError(t, callTool(t, ctx, session, "you.subagent", args), "worker_session.invalid_request", false)
	response := postHostJSON(t, ctx, host.URL()+"/worker-sessions", args, 400).(map[string]any)
	if response["code"] != "SUBAGENT_ROUTE_INVALID" {
		t.Fatal("invalid document lost typed refusal")
	}
	assertToolError(t, callAction(t, ctx, session, "READ", map[string]any{"workerSessionId": "invalid-setting"}), "worker_session.not_found", false)
	if calls.Load() != 0 || !reflect.DeepEqual(baseline, capacityFactoryIDs(t, host.URL())) {
		t.Fatal("invalid saved capacity created execution or Factory effects")
	}
}

func TestDirectCapacitySelectedHostsAreIndependent(t *testing.T) {
	t.Parallel()
	client := newSelectedHostClientProcess(t)
	release := make(chan struct{})
	var callsA, callsB atomic.Int32
	launchA, launchB := make(chan platformprocess.CommandRequest, 1), make(chan platformprocess.CommandRequest, 2)
	hostA, dirA, _ := newCapacityProfileHost(t, capacityProvider{launched: launchA, released: release, calls: &callsA}, `{"defaults":{"directSessionCapacity":1}}`, nil)
	hostB, dirB, _ := newCapacityProfileHost(t, capacityProvider{launched: launchB, released: release, calls: &callsB}, `{"defaults":{"directSessionCapacity":2}}`, nil)
	t.Cleanup(func() { close(release) })
	a, ctx := startMCP(t, client, hostA.URL())
	b, _ := startMCP(t, client, hostB.URL())
	assertSubagentAdmission(t, callTool(t, ctx, a, "you.subagent", capacityLifetimeArgs("a", dirA)), "a")
	assertToolError(t, callTool(t, ctx, a, "you.subagent", capacityLifetimeArgs("a-excess", dirA)), "worker_session.capacity_exceeded", true)
	for _, id := range []string{"b-first", "b-second"} {
		assertSubagentAdmission(t, callTool(t, ctx, b, "you.subagent", capacityLifetimeArgs(id, dirB)), id)
	}
	assertCapacityParity(t, client, ctx, b, hostB.URL(), dirB, capacityLifetimeArgs("excess", dirB))
	select {
	case <-launchA:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	for range 2 {
		select {
		case <-launchB:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	if callsA.Load() != 1 || callsB.Load() != 2 {
		t.Fatalf("host routing effects A=%d B=%d", callsA.Load(), callsB.Load())
	}
}

func TestDirectCapacityDocumentUpdateAndFreshHost(t *testing.T) {
	t.Parallel()
	client := newSelectedHostClientProcess(t)
	runner := &joinedCapacityProvider{outcome: "success", started: make(chan struct{}, 2), canceled: make(chan struct{}), release: make(chan struct{}), replacement: make(chan struct{})}
	profile := `{"backendScopeID":"local-11111111-1111-4111-8111-111111111111","defaults":{"directSessionCapacity":1,"workerModelProvider":"codex","workerModel":"generic-before","subagent":{"provider":"codex","model":"controlled-model"},"futureDefault":{"enabled":true}},"futureRoot":{"retained":true},"runtime":{"futureRuntime":"retained"},"models":{},"messaging":{"enabled":false}}`
	host, dir, home := newCapacityProfileHost(t, runner, profile, nil)
	t.Cleanup(runner.unblock)
	inputs := support.FakeInputs(t.Context(), []string{"you", "init", "--provider", "claude", "--model", "generic-after"})
	inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	inputs.Input.WorkingDirectory = dir
	if err := host.Execute(t, inputs.Input); err != nil {
		t.Fatalf("public settings update: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(home, ".you-agent-factory", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved map[string]any
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	defaults := saved["defaults"].(map[string]any)
	if defaults["directSessionCapacity"] != float64(1) || defaults["workerModel"] != "generic-after" ||
		!reflect.DeepEqual(defaults["subagent"], map[string]any{"provider": "CODEX", "model": "controlled-model"}) ||
		!reflect.DeepEqual(defaults["futureDefault"], map[string]any{"enabled": true}) ||
		!reflect.DeepEqual(saved["futureRoot"], map[string]any{"retained": true}) ||
		saved["runtime"].(map[string]any)["futureRuntime"] != "retained" || saved["messaging"].(map[string]any)["enabled"] != false {
		t.Fatalf("saved capacity=%v model=%v route=%v futureDefault=%v futureRoot=%v runtime=%v messaging=%v", defaults["directSessionCapacity"], defaults["workerModel"], defaults["subagent"], defaults["futureDefault"], saved["futureRoot"], saved["runtime"], saved["messaging"])
	}
	host.Close(t)
	// Restart itself is the behavior: a fresh production owner must load the
	// saved bound and route, rather than reuse the original resolved values.
	fresh := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: support.ScaffoldSingleStepFactory(t, "capacity-reloaded"), WorkingDirectory: dir,
		Edges: serviceedges.Edges{FactorySessionResolveHomeDirectory: func() (string, error) { return home, nil }, ProviderCommandRunner: runner,
			ProviderCatalogProbe: func(_ context.Context, d providers.Descriptor) (providers.Descriptor, error) {
				d.Readiness = providers.ReadinessReady
				return d, nil
			}},
	})
	session, ctx := startMCP(t, client, fresh.URL())
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	args := capacityLifetimeArgs("replacement", dir)
	delete(args, "provider")
	delete(args, "model")
	assertSubagentAdmission(t, callTool(t, ctx, session, "you.subagent", args), "replacement")
	waitCapacityGate(t, ctx, runner.started)
	assertToolError(t, callTool(t, ctx, session, "you.subagent", capacityLifetimeArgs("excess", dir)), "worker_session.capacity_exceeded", true)
	runner.unblock()
	followCapacityTerminal(t, ctx, client, fresh.URL(), dir, "replacement", "COMPLETED")
}

// Profiles and bounds are process-wide. Each cohort has its own immutable
// root graph; clients reuse their process and select hosts explicitly.
func newCapacityProfileHost(t *testing.T, runner platformprocess.CommandRunner, profile string, writer *capacityOpeningWriter) (*support.FunctionalAPIServer, string, string) {
	t.Helper()
	home, dir := t.TempDir(), t.TempDir()
	path := filepath.Join(home, ".you-agent-factory", "config.json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(profile), 0600); err != nil {
		t.Fatal(err)
	}
	replacements := serviceedges.Edges{FactorySessionResolveHomeDirectory: func() (string, error) { return home, nil }, ProviderCommandRunner: runner,
		ProviderCatalogProbe: func(_ context.Context, d providers.Descriptor) (providers.Descriptor, error) {
			d.Readiness = providers.ReadinessReady
			return d, nil
		}}
	if writer != nil {
		replacements.WorkerRecordingWriter = writer
		replacements.WorkerRecordingStoreObserver = func(store recordings.WorkerRecordingStore) { writer.WorkerRecordingStore = store }
	}
	host := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: support.ScaffoldSingleStepFactory(t, "capacity-profile"), WorkingDirectory: dir,
		Env: append(os.Environ(), "HOME="+home, "USERPROFILE="+home), Edges: replacements,
	})
	return host, dir, home
}

type capacityOpeningWriter struct {
	recordings.WorkerRecordingStore
}

func (w *capacityOpeningWriter) PersistWorkerRecord(ctx context.Context, record recordings.WorkerRecordingRecord) error {
	if record.WorkerSessionID == "opening-failure" {
		return recordings.ErrWorkerRecordingPersistence
	}
	return w.WorkerRecordingStore.PersistWorkerRecord(ctx, record)
}
