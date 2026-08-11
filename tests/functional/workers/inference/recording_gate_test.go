package inference_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingswire "github.com/portpowered/infinite-you/pkg/services/recordings/wire"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestWSRFT004DurableOpeningGatesProviderHandoff proves the opening barrier
// through the canonical root-built process. The durable writer is a real
// sidecar writer wrapped only to observe the accepted order and inject one
// opening persistence failure; the ProviderCommandRunner remains the only
// provider effect at this boundary.
//
// WSR-FT-004: durable opening before the first provider call and zero provider
// calls when opening durability fails.
func TestWSRFT004DurableOpeningGatesProviderHandoff(t *testing.T) {
	t.Run("durable opening precedes provider call", func(t *testing.T) {
		probe := newWSRFT004RecordingProbe(t, false)
		runner := newWSRFT004ProviderRunner(t, probe)
		dir := wsrFT004Factory(t)

		_, listed := runWSRFT004Factory(t, dir, serviceedges.Edges{
			ProviderCommandRunner: runner,
			WorkerRecordingWriter: probe,
		})

		if got := runner.CallCount(); got != 1 {
			t.Fatalf("provider command calls = %d, want exactly one call after durable opening", got)
		}
		probe.assertOpeningBeforeProvider(t)
		if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
			t.Fatalf("completed Work = %d, want one; listed=%#v", got, listed)
		}
	})

	t.Run("opening durability failure prevents provider call", func(t *testing.T) {
		probe := newWSRFT004RecordingProbe(t, true)
		runner := newWSRFT004ProviderRunner(t, probe)
		dir := wsrFT004Factory(t)

		_, listed := runWSRFT004Factory(t, dir, serviceedges.Edges{
			ProviderCommandRunner: runner,
			WorkerRecordingWriter: probe,
		})

		if got := runner.CallCount(); got != 0 {
			t.Fatalf("provider command calls = %d, want zero after opening durability failure", got)
		}
		probe.assertOpeningFailure(t)
		if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
			t.Fatalf("failed Work = %d, want one; listed=%#v", got, listed)
		}
	})
}

func wsrFT004Factory(t *testing.T) string {
	t.Helper()
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.ClearSeedInputs(t, dir)
	loaded := loadOpeningRecordFixture(t, "codex", "success")
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, loaded.Process.Model))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"WSR-FT-004 durable opening"}`))
	return dir
}

func runWSRFT004Factory(
	t *testing.T,
	dir string,
	edges serviceedges.Edges,
) (factoryapi.FactorySession, factoryapi.ListWorkResponse) {
	t.Helper()
	loaded := loadOpeningRecordFixture(t, "codex", "success")
	exitCode := 0
	if loaded.Process.ExitCode != nil {
		exitCode = *loaded.Process.ExitCode
	}
	providerRunner, ok := edges.ProviderCommandRunner.(*wsrFT004ProviderRunner)
	if !ok {
		t.Fatalf("provider runner type = %T, want WSR-FT-004 probe runner", edges.ProviderCommandRunner)
	}
	providerRunner.delegate.Queue(platformprocess.CommandResult{
		Stdout:   append([]byte(nil), loaded.Stdout.Raw...),
		Stderr:   []byte(loaded.Stderr),
		ExitCode: exitCode,
	})

	api := support.NewProcessAPIServer()
	edges.APIServerStarter = api.Start
	process := support.BuildProcess(t, edges)
	support.CleanupProcess(t, process)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	recordPath := filepath.Join(t.TempDir(), "wsr-ft-004.json")
	inputs := support.FakeInputs(ctx, []string{
		"you", "run", "--dir", dir, "--continuously", "--with-server", "--quiet", "--record", recordPath,
	})
	inputs.Input.WorkingDirectory = dir
	daemon := support.StartProcessCommand(t, process, inputs.Input)
	baseURL := api.WaitForURL(t)
	support.WaitForSessionTerminalStatus(t, baseURL, factorysessions.DefaultSessionID, 30*time.Second)
	session := support.GetDefaultSession(t, baseURL)
	listed := support.ListDefaultSessionWork(t, baseURL)
	daemon.Stop(t)
	if err := daemon.Err(); err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("recorded factory Process.Execute: %v\nstdout:\n%s\nstderr:\n%s", err, inputs.Stdout(), inputs.Stderr())
	}
	return session, listed
}

type wsrFT004RecordingProbe struct {
	delegate    recordings.WorkerRecordingWriter
	failOpening bool

	mu          sync.Mutex
	events      []string
	failure     *recordings.WorkerRecordingFailure
	failureOnce sync.Once
}

func newWSRFT004RecordingProbe(t *testing.T, failOpening bool) *wsrFT004RecordingProbe {
	t.Helper()
	writer, err := recordingswire.NewWorkerRecordingFileWriter(
		platformreplay.NewLocal(runtime.GOOS),
		t.TempDir(),
	)
	if err != nil {
		t.Fatalf("construct Worker recording writer: %v", err)
	}
	return &wsrFT004RecordingProbe{delegate: writer, failOpening: failOpening}
}

func (probe *wsrFT004RecordingProbe) PersistWorkerRecord(
	ctx context.Context,
	record recordings.WorkerRecordingRecord,
) error {
	if probe.failOpening && record.Record.ID.Position == 1 {
		probe.mu.Lock()
		probe.events = append(probe.events, "opening-rejected")
		probe.mu.Unlock()
		return errors.New("injected opening durability failure")
	}
	if err := probe.delegate.PersistWorkerRecord(ctx, record); err != nil {
		return err
	}
	probe.mu.Lock()
	probe.events = append(probe.events, fmt.Sprintf("durable:%d", record.Record.ID.Position))
	probe.mu.Unlock()
	return nil
}

func (probe *wsrFT004RecordingProbe) PersistWorkerRecordingFailure(
	ctx context.Context,
	failure recordings.WorkerRecordingFailure,
) error {
	writer, ok := probe.delegate.(recordings.WorkerRecordingFailureWriter)
	if !ok {
		return errors.New("durable Worker writer has no failure side")
	}
	if err := writer.PersistWorkerRecordingFailure(ctx, failure); err != nil {
		return err
	}
	probe.failureOnce.Do(func() {
		probe.mu.Lock()
		defer probe.mu.Unlock()
		copy := failure
		probe.failure = &copy
		probe.events = append(probe.events, "failure:"+failure.Code)
	})
	return nil
}

func (probe *wsrFT004RecordingProbe) recordProviderCall() {
	probe.mu.Lock()
	defer probe.mu.Unlock()
	probe.events = append(probe.events, "provider-call")
}

func (probe *wsrFT004RecordingProbe) assertOpeningBeforeProvider(t *testing.T) {
	t.Helper()
	probe.mu.Lock()
	defer probe.mu.Unlock()
	opening, provider := eventIndexes(probe.events, "durable:1", "provider-call")
	if opening < 0 || provider < 0 || opening >= provider {
		t.Fatalf("durable Worker opening/provider order = %#v, want durable:1 before provider-call", probe.events)
	}
}

func (probe *wsrFT004RecordingProbe) assertOpeningFailure(t *testing.T) {
	t.Helper()
	probe.mu.Lock()
	defer probe.mu.Unlock()
	if probe.failure == nil || probe.failure.Code != "PERSISTENCE_FAILED" {
		t.Fatalf("durable opening failure = %#v, want PERSISTENCE_FAILED classification", probe.failure)
	}
	if probe.failure.RecordingID == "" || probe.failure.WorkerSessionID == "" || probe.failure.Topic == "" {
		t.Fatalf("durable opening failure identity = %#v, want recording, Worker Session, and topic", probe.failure)
	}
	if opening, provider := eventIndexes(probe.events, "durable:1", "provider-call"); opening >= 0 || provider >= 0 {
		t.Fatalf("opening failure events = %#v, want no durable opening or provider call", probe.events)
	}
}

func eventIndexes(events []string, first, second string) (int, int) {
	firstIndex, secondIndex := -1, -1
	for index, event := range events {
		if event == first && firstIndex < 0 {
			firstIndex = index
		}
		if event == second && secondIndex < 0 {
			secondIndex = index
		}
	}
	return firstIndex, secondIndex
}

type wsrFT004ProviderRunner struct {
	delegate *testutil.ProviderCommandRunner
	probe    *wsrFT004RecordingProbe
}

func newWSRFT004ProviderRunner(t *testing.T, probe *wsrFT004RecordingProbe) *wsrFT004ProviderRunner {
	t.Helper()
	return &wsrFT004ProviderRunner{delegate: testutil.NewProviderCommandRunner(), probe: probe}
}

func (runner *wsrFT004ProviderRunner) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	runner.probe.recordProviderCall()
	return runner.delegate.Run(ctx, request)
}

func (runner *wsrFT004ProviderRunner) CallCount() int {
	return runner.delegate.CallCount()
}
