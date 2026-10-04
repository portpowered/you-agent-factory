package root_composition_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// HTTP/CLI parity is the contract here. Each leaf owns its session, profile,
// recording destination and listener; one immutable process serves all leaves.
// Before/after reads are ordered within a leaf to prove the durability invariant.
func TestWorkListConfirmsStateAfterRecordingFlush(t *testing.T) {
	t.Parallel()
	cases := newFlushCases(t)
	runner := support.NewStaticSuccessCommandRunner("Processed. COMPLETE")
	process, err := support.BuildProcessWithContext(t.Context(), serviceedges.Edges{
		ProviderCommandRunner: flushCommandRunner{run: func(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
			for _, scenario := range cases {
				if strings.Contains(string(request.Stdin), scenario.sessionID) {
					scenario.dispatches.Add(1)
					scenario.hold.Store(true)
				}
			}
			return runner.Run(ctx, request)
		}},
		APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
			for _, scenario := range cases {
				if request.Port == scenario.port {
					return scenario.api.Start(ctx, request)
				}
			}
			return fmt.Errorf("unexpected listener port %d", request.Port)
		},
		RecordingWriteFile: func(path string, data []byte) error {
			for _, scenario := range cases {
				if strings.HasPrefix(path, scenario.dir+string(os.PathSeparator)) {
					return scenario.write(path, data)
				}
			}
			return fmt.Errorf("unexpected recording destination %q", path)
		},
	})
	if err != nil {
		t.Fatalf("BuildProcess: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := process.Close(ctx); err != nil && !errors.Is(err, recordings.ErrRecordingSnapshotWrite) {
			t.Errorf("close process: %v", err)
		}
	})
	for _, scenario := range cases {
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()
			runFlushCase(t, process, scenario)
		})
	}
}

func runFlushCase(t *testing.T, process support.Process, scenario *flushCase) {
	t.Helper()
	env := recoveryActivationHomeEnvironment(scenario.home)
	support.InitializeCustomerHomeWithProcess(t, process, env, scenario.dir)
	args := []string{"you", "run", "--dir", scenario.dir, "--session", scenario.sessionID,
		"--continuously", "--with-server", "--server", "http://127.0.0.1:" + strconv.Itoa(scenario.port), "--quiet"}
	if scenario.name == "disabled storage" {
		args = append(args, "--no-record")
	} else {
		args = append(args, "--record", filepath.Join(scenario.dir, "recording.json"))
	}
	ctx, cancel := context.WithCancel(t.Context())
	input := support.FakeInputs(ctx, args)
	input.Input.Env, input.Input.WorkingDirectory = env, scenario.dir
	command := support.StartProcessCommand(t, process, input.Input)
	// Release the filesystem gate before joining the invocation, including failure paths.
	t.Cleanup(func() {
		scenario.releaseWrite()
		cancel()
		if scenario.name == "failed storage" {
			waitForFlushSignal(t, command.Done(), "failed-recording invocation shutdown")
			if err := command.Err(); !errors.Is(err, recordings.ErrRecordingSnapshotWrite) {
				t.Errorf("shutdown error = %v, want recording write failure", err)
			}
			command.AcceptError()
		}
		command.Stop(t)
	})
	baseURL := scenario.api.WaitForURL(t)
	if scenario.name == "disabled storage" {
		assertInvalidAdmissionBeforeDispatch(t, process, scenario, baseURL)
	}
	name := "flush-characterization"
	submitted := support.SubmitSessionWorkAt(t, baseURL, scenario.sessionID, factoryapi.SubmitWorkRequest{
		Name: &name, WorkTypeName: "task", Payload: map[string]string{"title": scenario.sessionID},
	})
	if submitted.WorkId == nil || *submitted.WorkId == "" {
		t.Fatalf("submission has no Work identity: %#v", submitted)
	}
	workID := *submitted.WorkId
	waitForFlushWork(t, baseURL, scenario.sessionID, workID, factoryapi.UNCONFIRMED)
	if scenario.dispatches.Load() == 0 {
		t.Fatal("completed valid Work did not reach the controlled provider command")
	}
	assertFlushListParity(t, process, scenario, baseURL, workID, factoryapi.UNCONFIRMED)
	if scenario.name == "disabled storage" {
		return
	}
	waitForFlushSignal(t, scenario.entered, "recording write entry")
	scenario.releaseWrite()
	waitForFlushSignal(t, scenario.completed, "recording write completion")
	want := factoryapi.UNCONFIRMED
	if scenario.name == "successful storage" {
		want = factoryapi.CONFIRMED
		waitForFlushWork(t, baseURL, scenario.sessionID, workID, want)
	}
	assertFlushListParity(t, process, scenario, baseURL, workID, want)
}

func waitForFlushWork(t *testing.T, baseURL, sessionID, workID string, want factoryapi.ConfirmationState) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	// Writer return precedes watermark publication and runtime projection updates.
	// Public reads are the observer; no private artifact decoding or fixed delay
	// can establish that publication has reached the customer boundary.
	for ctx.Err() == nil {
		listed := support.GetJSON[factoryapi.ListWorkResponse](t, support.SessionWorkURL(baseURL, sessionID, "/work"))
		for _, item := range listed.Results {
			if item.WorkId != nil && *item.WorkId == workID && item.State != nil &&
				item.State.Name == "complete" && item.ConfirmationState != nil && *item.ConfirmationState == want {
				return
			}
		}
	}
	t.Fatalf("Work %s did not reach complete/%s: %v", workID, want, ctx.Err())
}

func assertFlushListParity(t *testing.T, process support.Process, scenario *flushCase, baseURL, workID string, want factoryapi.ConfirmationState) {
	t.Helper()
	httpList := support.GetJSON[factoryapi.ListWorkResponse](t, support.SessionWorkURL(baseURL, scenario.sessionID, "/work"))
	input := support.FakeInputs(t.Context(), []string{"you", "--server", baseURL, "--json", "work", "list", "--session", scenario.sessionID})
	input.Input.Env, input.Input.WorkingDirectory = recoveryActivationHomeEnvironment(scenario.home), scenario.dir
	input.Input.Stdin = strings.NewReader("")
	if err := process.Execute(input.Input); err != nil {
		t.Fatalf("CLI list: %v; stderr=%s", err, input.Stderr())
	}
	var cliList factoryapi.ListWorkResponse
	if err := json.Unmarshal([]byte(input.Stdout()), &cliList); err != nil {
		t.Fatalf("CLI list JSON: %v; stdout=%s", err, input.Stdout())
	}
	for boundary, listed := range map[string]factoryapi.ListWorkResponse{"HTTP": httpList, "CLI": cliList} {
		if len(listed.Results) != 1 {
			t.Fatalf("%s list = %#v, want exactly the owned Work", boundary, listed)
		}
		item := listed.Results[0]
		if item.WorkId == nil || *item.WorkId != workID || item.State == nil || item.State.Name != "complete" ||
			item.ConfirmationState == nil || *item.ConfirmationState != want {
			t.Fatalf("%s Work = %#v, want %s complete/%s", boundary, item, workID, want)
		}
	}
}

func waitForFlushSignal(t *testing.T, signal <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(30 * time.Second):
		t.Fatalf("timed out waiting for %s", name)
	}
}
