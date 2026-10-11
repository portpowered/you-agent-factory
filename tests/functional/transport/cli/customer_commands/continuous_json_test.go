package customer_commands_test

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"go.uber.org/zap"
)

// Continuous local commands own Current Factory/~default. Only this smallest
// cohort is ordered; its independent explicit-session peer invocations overlap.
func TestCLIContinuousJSONJourneys(t *testing.T) {
	t.Parallel()
	initializeOutputFixture(t)
	fixture := sharedMachineOutputFixture(t)
	starters := make(chan platformhttpserver.Starter, 1)
	browserTargets := make(chan string, 1)
	var browserUnavailable atomic.Bool
	process := support.BuildProcess(t, serviceedges.Edges{
		ProcessLogger: fixture.logger,
		APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
			select {
			case start := <-starters:
				return start(ctx, request)
			case <-ctx.Done():
				return ctx.Err()
			}
		},
		BrowserOpener: func(_ context.Context, target string) error {
			browserTargets <- target
			if browserUnavailable.Load() {
				return errors.New("controlled browser unavailable")
			}
			return nil
		},
	})
	for _, name := range []string{"J01 work and stop", "J02 idle default recording", "J03 cancel", "J06 live JSON with peers", "J05 normal disabled", "J05 normal open", "J05 normal unavailable"} {
		browserUnavailable.Store(strings.Contains(name, "unavailable"))
		t.Run(name, func(t *testing.T) {
			continuousJSONJourney(t, process, starters, name)
			if strings.HasPrefix(name, "J01") || strings.Contains(name, "open") || strings.Contains(name, "unavailable") {
				select {
				case target := <-browserTargets:
					if !strings.HasSuffix(target, "/dashboard/ui") {
						t.Fatalf("browser target=%s", target)
					}
				default:
					t.Fatal("browser effect missing")
				}
			} else if len(browserTargets) != 0 {
				t.Fatal("disabled browser opened")
			}
		})
	}
	if !fixture.logger.Core().Enabled(zap.DebugLevel) {
		t.Fatal("invocation policy mutated shared logger level")
	}
	fixture.logger.Debug("continuous JSON shared logger witness", zap.String("phase", "after-cohort"))
	if entries := fixture.diagnostics.FilterMessage("continuous JSON shared logger witness").All(); len(entries) != 1 || entries[0].ContextMap()["phase"] != "after-cohort" {
		t.Fatal("invocation policy changed shared logging backend")
	}
	t.Run("J04 listener failure", func(t *testing.T) {
		continuousJSONListenerFailure(t, process, starters)
	})
	t.Run("J07 selectors reject before listener", func(t *testing.T) {
		continuousJSONSelectionFailures(t, process)
	})
}

func continuousJSONListenerFailure(t *testing.T, process support.Process, starters chan<- platformhttpserver.Starter) {
	t.Helper()
	cause := errors.New(`listener unavailable token="PRIVATE_TOKEN" prompt="PRIVATE_PROMPT" body="PRIVATE_BODY"`)
	starters <- func(context.Context, platformhttpserver.StartRequest) error { return cause }
	home := newCLIHome(t)
	result := home.run(t, process, nil, "--json", "run", "--dir", startupEffectFactory(t), "--continuously", "--with-server")
	if !errors.Is(result.Err, cause) || result.Stdout != "" || strings.Contains(result.Stderr, "PRIVATE_") || !strings.Contains(result.Stderr, "listener unavailable") {
		t.Fatalf("listener failure: %#v", result)
	}
}

func continuousJSONSelectionFailures(t *testing.T, process support.Process) {
	t.Helper()
	for _, flags := range [][]string{{"--quiet", "--json"}, {"--quiet", "--output", "primary"}, {"--output", "response-stream"}} {
		home := newCLIHome(t)
		args := append([]string{"run", "--dir", startupEffectFactory(t), "--continuously", "--with-server"}, flags...)
		result := home.run(t, process, nil, args...)
		var typed *runcli.InvocationError
		if !errors.As(result.Err, &typed) || result.Stdout != "" {
			t.Fatalf("selection %v: %#v", flags, result)
		}
		response := decodeSingleJSONErrorResponse(t, result.Stderr)
		if string(response.Code) != typed.Code {
			t.Fatalf("selection code: %#v", response)
		}
	}
}

func continuousJSONJourney(t *testing.T, process support.Process, starters chan<- platformhttpserver.Starter, name string) {
	t.Helper()
	server := support.NewProcessAPIServer()
	starters <- server.Start
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	home, inputs, recording := newContinuousJSONInputs(t, process, ctx, name)
	stdout := newContinuousStdoutCapture("Factory initiated:", "Dashboard URL:")
	inputs.Input.Stdout = stdout
	done := make(chan error, 1)
	go func() { done <- process.Execute(inputs.Input) }()
	joined := false
	t.Cleanup(func() {
		cancel()
		if !joined {
			select {
			case <-done:
			case <-time.After(time.Minute):
				t.Error("owned invocation did not join")
			}
		}
	})
	baseURL := server.WaitForURL(t)
	status := support.GetJSON[factoryapi.StatusResponse](t, baseURL+"/status")
	if status.RuntimeStatus == "" {
		t.Fatal("readiness has no runtime status")
	}
	if strings.HasPrefix(name, "J05") {
		waitForTextStreamStartup(t, stdout)
	}
	if strings.HasPrefix(name, "J01") {
		support.SubmitDefaultSessionWork(t, baseURL, factoryapi.SubmitWorkRequest{WorkTypeName: "task", Payload: map[string]string{"value": "useful"}})
		status = support.WaitForSessionTerminalStatus(t, baseURL, "~default", 10*time.Second)
		if status.Categories.Terminal != 1 || status.Categories.Failed != 0 {
			t.Fatalf("Work did not finish: %#v", status)
		}
	}
	if strings.HasPrefix(name, "J06") {
		runConcurrentOutputPolicyCohort(t)
	}
	if strings.HasPrefix(name, "J03") {
		cancel()
	} else {
		stopContinuousJSONHost(t, baseURL)
	}
	select {
	case err := <-done:
		joined = true
		requireContinuousInvocationResult(t, name, err, inputs.Stderr())
	case <-time.After(time.Minute):
		t.Fatal("owned invocation did not stop")
	}
	assertContinuousJSONOutput(t, name, baseURL, stdout.String())
	if strings.HasPrefix(name, "J02") {
		recording = findContinuousRecording(t, home.home)
	}
	requireContinuousRecording(t, process, starters, home, recording, strings.HasPrefix(name, "J01"))
}

func requireContinuousInvocationResult(t *testing.T, name string, err error, stderr string) {
	t.Helper()
	if strings.HasPrefix(name, "J03") {
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation identity: %v", err)
		}
	} else if err != nil {
		t.Fatalf("ordinary stop: %v; stderr=%s", err, stderr)
	}
}

func assertContinuousJSONOutput(t *testing.T, name, baseURL, output string) {
	t.Helper()
	if strings.HasPrefix(name, "J05") {
		if !strings.Contains(output, "Factory initiated:") || !strings.Contains(output, "Dashboard URL: "+baseURL+"/dashboard/ui") {
			t.Fatalf("normal startup: %q", output)
		}
		for _, text := range []string{"Opening dashboard:", "Dashboard auto-open unavailable:", "Dashboard auto-open disabled;"} {
			want := (strings.Contains(name, "open") && text == "Opening dashboard:") ||
				(strings.Contains(name, "unavailable") && text == "Dashboard auto-open unavailable:") ||
				(strings.Contains(name, "disabled") && text == "Dashboard auto-open disabled;")
			if strings.Contains(output, text) != want {
				t.Fatalf("browser presentation: %q", output)
			}
		}
	} else if output != "" {
		t.Fatalf("continuous JSON invented stdout: %q", output)
	}
}

// API stop is an API-owned parity cell: it stops this CLI-owned host.
func stopContinuousJSONHost(t *testing.T, baseURL string) {
	t.Helper()
	response, err := http.Post(baseURL+"/shutdown", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("stop status=%d", response.StatusCode)
	}
}

func newContinuousJSONInputs(t *testing.T, process support.Process, ctx context.Context, name string) (cliHome, *support.CapturedInputs, string) {
	t.Helper()
	home := newCLIHome(t)
	// Bootstrap the private profile before the readiness observation window.
	home.requireSuccess(t, process, "factory", "list")
	recording := filepath.Join(home.work, "owned.json")
	args := []string{"you", "--json", "run", "--dir", startupEffectFactory(t), "--continuously", "--with-server"}
	if strings.HasPrefix(name, "J05") {
		args = append(args[:1], args[2:]...)
	}
	if strings.HasPrefix(name, "J01") || strings.Contains(name, "open") || strings.Contains(name, "unavailable") {
		args = append(args, "--with-site")
	}
	if !strings.HasPrefix(name, "J02") {
		args = append(args, "--record", recording)
	}
	inputs := support.FakeInputs(ctx, args)
	inputs.Input.Env = builtcliacceptance.ProcessEnvForIsolatedHome(home.home)
	inputs.Input.WorkingDirectory = home.work
	inputs.Input.Stdin = strings.NewReader("")
	// Browser effects are controlled by Edges here, not the worker host's opt-out.
	inputs.Input.Env = append(inputs.Input.Env, "YOU_NO_BROWSER_OPEN=0")
	t.Logf("owned command args=%v", args)
	return home, inputs, recording
}

func findContinuousRecording(t *testing.T, home string) string {
	t.Helper()
	var paths []string
	err := filepath.WalkDir(filepath.Join(home, ".you-agent-factory", "recordings"), func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && strings.HasSuffix(path, ".json") {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil || len(paths) != 1 {
		t.Fatalf("default recording: %v, %v", paths, err)
	}
	return paths[0]
}

func requireContinuousRecording(t *testing.T, process support.Process, starters chan<- platformhttpserver.Starter, home cliHome, path string, work bool) {
	t.Helper()
	server := support.NewProcessAPIServer()
	starters <- server.Start
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	inputs := support.FakeInputs(ctx, []string{"you", "run", "--replay", path, "--with-server", "--continuously", "--no-record", "--quiet"})
	inputs.Input.Env = builtcliacceptance.ProcessEnvForIsolatedHome(home.home)
	inputs.Input.WorkingDirectory = home.work
	done := make(chan error, 1)
	go func() { done <- process.Execute(inputs.Input) }()
	defer func() {
		cancel()
		select {
		case err := <-done:
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Errorf("replay join: %v", err)
			}
		case <-time.After(time.Minute):
			t.Error("replay did not join")
		}
	}()
	baseURL := server.WaitForURL(t)
	events := support.GetFactoryEventsForSessionAt(t, baseURL, "~default")
	requireContinuousHistory(t, events, work)
}

func requireContinuousHistory(t *testing.T, events []factoryapi.FactoryEvent, work bool) {
	t.Helper()
	if len(events) < 2 {
		t.Fatal("recording has no complete history")
	}
	previous, completed := -1, false
	for _, event := range events {
		if !work && (event.Type == factoryapi.FactoryEventTypeWorkRequest || event.Type == factoryapi.FactoryEventTypeWorkStateChange) {
			t.Fatal("idle continuous invocation invented Work")
		}
		if event.Context.Sequence <= previous {
			t.Fatal("recording sequence is not ordered")
		}
		previous = event.Context.Sequence
		if event.Type == factoryapi.FactoryEventTypeWorkStateChange {
			payload, err := event.Payload.AsWorkStateChangeEventPayload()
			if err != nil {
				t.Fatal(err)
			}
			completed = completed || payload.ToState == "complete"
		}
	}
	requireContinuousTerminalEvent(t, events[len(events)-1])
	if work && !completed {
		t.Fatal("recording lost completed Work")
	}
}

func requireContinuousTerminalEvent(t *testing.T, last factoryapi.FactoryEvent) {
	t.Helper()
	if last.Type != factoryapi.FactoryEventTypeSessionCompleted {
		t.Fatalf("terminal history: %#v", last)
	}
	state, err := last.Payload.AsSessionCompletedEventPayload()
	if err != nil || state.FinalStatus != "SUCCEEDED" {
		t.Fatalf("terminal state: %#v, %v", state, err)
	}
}
