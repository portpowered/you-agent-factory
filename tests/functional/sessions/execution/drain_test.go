package execution_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const incompleteDrainProcessTimeout = 15 * time.Second
const incompleteDrainListenAddress = "127.0.0.1:7437"

// TestWithServerDrainCannotReportSuccessWhileWorkIsNonTerminal proves that a
// finite hosted run returns a failure after its listener and runtime have
// joined when the queue drains around non-terminal customer Work.
func TestWithServerDrainCannotReportSuccessWhileWorkIsNonTerminal(t *testing.T) {
	t.Parallel()
	acquireExecutionFixtureSlot(t)

	for _, mode := range []struct {
		name        string
		flag        string
		wantBrowser int32
	}{
		{name: "server", flag: "--with-server"},
		{name: "site", flag: "--with-site", wantBrowser: 1},
	} {
		mode := mode
		t.Run(mode.name, func(t *testing.T) {
			runIncompleteDrainMode(t, mode.flag, mode.wantBrowser)
		})
	}
}

func runIncompleteDrainMode(t *testing.T, modeFlag string, wantBrowser int32) {
	t.Helper()
	factoryDir := scaffoldIncompleteDrainFactory(t)
	workFile := writeIncompleteDrainWork(t)

	var listenerStarts, listenerStops, browserCalls atomic.Int32
	api := support.NewProcessAPIServer()
	shutdownGate := make(chan struct{})
	workerRelease := make(chan struct{})
	var releaseListenerOnce, releaseWorkerOnce sync.Once
	releaseListener := func() {
		releaseListenerOnce.Do(func() { close(shutdownGate) })
	}
	releaseWorker := func() {
		releaseWorkerOnce.Do(func() { close(workerRelease) })
	}
	transportStopRequested := make(chan struct{})
	listenerJoined := make(chan struct{})
	api.HoldShutdownUntilSignaled(shutdownGate)
	edges := serviceedges.Edges{
		APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
			listenerStarts.Add(1)
			context.AfterFunc(ctx, func() { close(transportStopRequested) })
			bound := request.OnBound
			request.OnBound = func(binding platformhttpserver.Binding) {
				if bound != nil {
					bound(binding)
				}
				// The gated worker output is the observable ordering barrier: the
				// listener must bind before Work can reach its blocked state and
				// become eligible for finite drain classification.
				releaseWorker()
			}
			err := api.Start(ctx, request)
			listenerStops.Add(1)
			close(listenerJoined)
			return err
		},
		BrowserOpener: func(context.Context, string) error {
			browserCalls.Add(1)
			return nil
		},
	}
	support.ConfigureWorkerCommands(
		t, &edges,
		support.NewGatedSuccessCommandRunner("Done. COMPLETE", workerRelease),
		nil,
	)
	process := support.BuildProcess(t, edges)
	support.CleanupProcess(t, process)

	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run", "--dir", factoryDir, "--no-record", "--quiet",
		modeFlag, "--work", workFile,
	})
	inputs.WorkingDirectory = factoryDir
	homeDir := t.TempDir()
	inputs.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)

	command := support.StartProcessCommand(t, process, inputs.Input)
	t.Cleanup(releaseListener)
	baseURL := api.WaitForURL(t)
	observation, err := waitForIncompleteDrainObservation(baseURL)
	if err != nil {
		t.Fatalf("incomplete-drain observation: %v", err)
	}
	if len(observation.Work.Results) != 1 {
		t.Fatalf("observed Work count = %d, want 1", len(observation.Work.Results))
	}
	work := observation.Work.Results[0]
	if work.Name != "blocked-work" {
		t.Fatalf("observed Work name = %q, want blocked-work", work.Name)
	}
	if work.State == nil || work.State.Type != factoryapi.WorkStateTypePROCESSING {
		t.Fatalf("observed Work state = %#v, want PROCESSING", work.State)
	}
	if observation.Status.RuntimeStatus == "" || observation.Status.Categories.Processing != 1 {
		t.Fatalf("observed runtime status = %#v, want one processing Work", observation.Status)
	}

	waitForSignal(t, transportStopRequested, "finite hosted runtime did not request listener shutdown")
	select {
	case <-command.Done():
		t.Fatal("finite hosted command completed before the listener join gate was released")
	default:
	}
	releaseListener()
	waitForCommandDone(t, command, "finite hosted run did not return after the runtime drained")
	waitForSignal(t, listenerJoined, "finite hosted listener did not join before command completion")
	assertIncompleteDrainFailure(
		t, command, inputs, &listenerStarts, &listenerStops, &browserCalls, wantBrowser,
	)
}

func assertIncompleteDrainFailure(
	t *testing.T,
	command *support.ProcessCommand,
	inputs *support.CapturedInputs,
	listenerStarts, listenerStops, browserCalls *atomic.Int32,
	wantBrowser int32,
) {
	t.Helper()
	command.AcceptError()
	err := command.Err()
	if err == nil {
		t.Fatalf("Process.Execute() error = %v, want incomplete-drain failure", err)
	}
	support.RequireSafeCLIDiagnostic(t, inputs.Stderr())
	if stdout := inputs.Stdout(); stdout != "" {
		t.Fatalf("stdout = %q, want no success or completion output", stdout)
	}
	if got := listenerStarts.Load(); got != 1 {
		t.Fatalf("listener starts = %d, want 1; err=%v stdout=%q stderr=%q", got, err, inputs.Stdout(), inputs.Stderr())
	}
	if got := listenerStops.Load(); got != 1 {
		t.Fatalf("listener stops = %d, want one joined listener", got)
	}
	if got := browserCalls.Load(); got != wantBrowser {
		t.Fatalf("browser calls = %d, want %d", got, wantBrowser)
	}
}

type incompleteDrainObservation struct {
	Work   factoryapi.ListWorkResponse
	Status factoryapi.StatusResponse
}

func waitForIncompleteDrainObservation(baseURL string) (incompleteDrainObservation, error) {
	// Work and status are asynchronously committed public projections and do
	// not expose one shared readiness channel. WaitForObservation therefore
	// accepts only the observable admitted/non-terminal state; its timeout is
	// solely a hung-projection guard, not a success window.
	return support.WaitForObservation(
		incompleteDrainProcessTimeout,
		func() (incompleteDrainObservation, error) {
			work, err := readHostedWork(baseURL)
			if err != nil {
				return incompleteDrainObservation{}, err
			}
			status, err := readHostedStatus(baseURL)
			if err != nil {
				return incompleteDrainObservation{}, err
			}
			return incompleteDrainObservation{Work: work, Status: status}, nil
		},
		func(observation incompleteDrainObservation) bool {
			if len(observation.Work.Results) != 1 {
				return false
			}
			work := observation.Work.Results[0]
			return work.State != nil &&
				work.State.Type == factoryapi.WorkStateTypePROCESSING &&
				observation.Status.RuntimeStatus != "" &&
				observation.Status.Categories.Processing == 1
		},
	)
}

func readHostedWork(baseURL string) (factoryapi.ListWorkResponse, error) {
	return readHostedJSON[factoryapi.ListWorkResponse](support.DefaultSessionWorkURL(baseURL, "/work"))
}

func readHostedStatus(baseURL string) (factoryapi.StatusResponse, error) {
	return readHostedJSON[factoryapi.StatusResponse](strings.TrimSuffix(baseURL, "/") + "/status")
}

func readHostedJSON[T any](endpoint string) (T, error) {
	var result T
	response, err := http.Get(endpoint)
	if err != nil {
		return result, fmt.Errorf("GET %s: %w", endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		payload, _ := io.ReadAll(response.Body)
		return result, fmt.Errorf(
			"GET %s status = %d: %s",
			endpoint,
			response.StatusCode,
			strings.TrimSpace(string(payload)),
		)
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return result, fmt.Errorf("decode GET %s: %w", endpoint, err)
	}
	return result, nil
}

func waitForSignal(t *testing.T, signal <-chan struct{}, message string) {
	t.Helper()
	timer := time.NewTimer(incompleteDrainProcessTimeout)
	defer timer.Stop()
	select {
	case <-signal:
	case <-timer.C:
		t.Fatal(message)
	}
}

func TestHostedFiniteRunsKeepEmptyAndTerminalSuccess(t *testing.T) {
	t.Parallel()
	acquireExecutionFixtureSlot(t)

	for _, scenario := range []struct {
		name     string
		workFile func(*testing.T) string
	}{
		{name: "empty", workFile: nil},
		{name: "terminal work", workFile: func(t *testing.T) string {
			return writeDrainWork(t, "complete")
		}},
	} {
		scenario := scenario
		for _, mode := range []struct {
			name        string
			flag        string
			wantBrowser int32
		}{
			{name: "server", flag: "--with-server"},
			{name: "site", flag: "--with-site", wantBrowser: 1},
		} {
			mode := mode
			t.Run(scenario.name+"/"+mode.name, func(t *testing.T) {
				factoryDir := scaffoldIncompleteDrainFactory(t)
				var workFile string
				if scenario.workFile != nil {
					workFile = scenario.workFile(t)
				}

				err, stdout, stderr, listenerStarts, listenerStops, browserCalls := runFiniteHostedCommand(
					t, factoryDir, workFile, mode.flag,
				)
				if err != nil {
					t.Fatalf("finite hosted run error = %v; stdout=%q stderr=%q", err, stdout, stderr)
				}
				wantStdout := ""
				if workFile != "" {
					wantStdout = "Batch completed successfully.\n"
				}
				if stdout != wantStdout || stderr != "" {
					t.Fatalf("finite success output = stdout:%q stderr:%q, want stdout:%q and quiet stderr", stdout, stderr, wantStdout)
				}
				if listenerStarts != 1 || listenerStops != 1 {
					t.Fatalf("listener lifecycle = starts:%d stops:%d, want one joined listener", listenerStarts, listenerStops)
				}
				if browserCalls != mode.wantBrowser {
					t.Fatalf("browser calls = %d, want %d", browserCalls, mode.wantBrowser)
				}
			})
		}
	}
}

func TestHostedContinuousRunsStayLiveWhileIdle(t *testing.T) {
	t.Parallel()
	acquireExecutionFixtureSlot(t)

	for _, mode := range []struct {
		name        string
		flag        string
		wantBrowser int32
	}{
		{name: "server", flag: "--with-server"},
		{name: "site", flag: "--with-site", wantBrowser: 1},
	} {
		mode := mode
		t.Run(mode.name, func(t *testing.T) {
			factoryDir := scaffoldIncompleteDrainFactory(t)
			var listenerStarts, listenerStops, browserCalls atomic.Int32
			transportReady := make(chan struct{})
			browserOpened := make(chan struct{}, 1)
			process := support.BuildProcess(t, serviceedges.Edges{
				APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
					listenerStarts.Add(1)
					if request.OnBound != nil {
						request.OnBound(platformhttpserver.Binding{Port: request.Port})
					}
					close(transportReady)
					<-ctx.Done()
					listenerStops.Add(1)
					return ctx.Err()
				},
				BrowserOpener: func(context.Context, string) error {
					browserCalls.Add(1)
					select {
					case browserOpened <- struct{}{}:
					default:
					}
					return nil
				},
			})
			support.CleanupProcess(t, process)

			inputs := support.FakeInputs(t.Context(), []string{
				"you", "run", "--dir", factoryDir, "--no-record", "--quiet",
				"--continuously", mode.flag,
			})
			inputs.WorkingDirectory = factoryDir
			homeDir := t.TempDir()
			inputs.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
			command := support.StartProcessCommand(t, process, inputs.Input)

			// The injected listener's binding channel is the deterministic startup
			// observation. The bounded fallback is only a test guard for a startup
			// regression; without it a broken Process.Execute would hang this test.
			listenerTimer := time.NewTimer(incompleteDrainProcessTimeout)
			defer listenerTimer.Stop()
			select {
			case <-transportReady:
			case <-listenerTimer.C:
				t.Fatal("continuous hosted run did not start its listener")
			}

			// Listener binding is the deterministic customer-visible readiness
			// boundary for this otherwise idle run. Once bound, the invocation must
			// still be live; spending an additional fixed observation interval here
			// does not strengthen that proof and only blocks the fixture.
			select {
			case <-command.Done():
				t.Fatalf("continuous hosted run exited while idle: err=%v stdout=%q stderr=%q", command.Err(), inputs.Stdout(), inputs.Stderr())
			default:
			}
			if mode.wantBrowser != 0 {
				browserTimer := time.NewTimer(incompleteDrainProcessTimeout)
				defer browserTimer.Stop()
				select {
				case <-browserOpened:
				case <-browserTimer.C:
					t.Fatal("continuous hosted site did not open the customer browser")
				}
			}

			command.Stop(t)
			if err := command.Err(); err != nil && !errors.Is(err, context.Canceled) {
				t.Fatalf("continuous hosted run cancellation error = %v", err)
			}
			if strings.Contains(inputs.Stderr(), "incomplete") {
				t.Fatalf("continuous stderr = %q, must not report finite incomplete drain", inputs.Stderr())
			}
			if listenerStarts.Load() != 1 || listenerStops.Load() != 1 {
				t.Fatalf("listener lifecycle = starts:%d stops:%d, want one joined listener", listenerStarts.Load(), listenerStops.Load())
			}
			if browserCalls.Load() != mode.wantBrowser {
				t.Fatalf("browser calls = %d, want %d", browserCalls.Load(), mode.wantBrowser)
			}
		})
	}
}

func runFiniteHostedCommand(
	t *testing.T,
	factoryDir, workFile, mode string,
) (error, string, string, int32, int32, int32) {
	t.Helper()

	var listenerStarts, listenerStops, browserCalls atomic.Int32
	process := support.BuildProcess(t, serviceedges.Edges{
		APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
			listenerStarts.Add(1)
			if request.OnBound != nil {
				request.OnBound(platformhttpserver.Binding{Port: request.Port})
			}
			<-ctx.Done()
			listenerStops.Add(1)
			return ctx.Err()
		},
		BrowserOpener: func(context.Context, string) error {
			browserCalls.Add(1)
			return nil
		},
	})
	support.CleanupProcess(t, process)

	args := []string{
		"you", "run", "--dir", factoryDir, "--no-record", "--quiet", mode,
	}
	if workFile != "" {
		args = append(args, "--work", workFile)
	}
	inputs := support.FakeInputs(t.Context(), args)
	inputs.WorkingDirectory = factoryDir
	homeDir := t.TempDir()
	inputs.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	command := support.StartProcessCommand(t, process, inputs.Input)
	waitForCommandDone(t, command, "finite hosted run did not return")
	command.AcceptError()
	return command.Err(), inputs.Stdout(), inputs.Stderr(), listenerStarts.Load(), listenerStops.Load(), browserCalls.Load()
}

func waitForCommandDone(t *testing.T, command *support.ProcessCommand, message string) {
	t.Helper()
	// ProcessCommand.Done is the deterministic completion signal. The bounded
	// fallback is necessary only to turn a regression in the finite-termination
	// contract into a test failure instead of an indefinitely hung test.
	timer := time.NewTimer(incompleteDrainProcessTimeout)
	defer timer.Stop()
	select {
	case <-command.Done():
	case <-timer.C:
		t.Fatal(message)
	}
}

func scaffoldIncompleteDrainFactory(t *testing.T) string {
	t.Helper()

	cfg := simplePipelineConfig()
	workTypes := cfg["workTypes"].([]map[string]any)
	states := workTypes[0]["states"].([]map[string]string)
	workTypes[0]["states"] = append(states, map[string]string{
		"name": "blocked",
		"type": "PROCESSING",
	})
	workstations := cfg["workstations"].([]map[string]any)
	workstations[0]["outputs"] = []map[string]string{{
		"workType": "task",
		"state":    "blocked",
	}}
	return scaffoldInvocationFactory(t, map[string]any{
		"workTypes":    workTypes,
		"workstations": workstations,
	})
}

func writeIncompleteDrainWork(t *testing.T) string {
	t.Helper()
	return writeDrainWork(t, "init")
}

func writeDrainWork(t *testing.T, state string) string {
	t.Helper()

	data, err := json.Marshal(map[string]any{
		"requestId": "drained-incomplete",
		"type":      "FACTORY_REQUEST_BATCH",
		"works": []map[string]any{{
			"name":         "blocked-work",
			"workTypeName": "task",
			"state":        state,
			"payload":      map[string]string{"purpose": "drain classification"},
		}},
	})
	if err != nil {
		t.Fatalf("marshal incomplete-drain Work: %v", err)
	}
	path := filepath.Join(t.TempDir(), "incomplete-drain.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write incomplete-drain Work: %v", err)
	}
	return path
}
