package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testpath"
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/cli"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/replay"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

// TestAdHocPrepare runs the prepare command directly for ad hoc testing.
// Set AGENT_FACTORY_ADHOC_RUN=1 and run:
//
//	go test -v ./tests/adhoc -run TestAdHocPrepare
func TestAdHocPrepare(t *testing.T) {
	if os.Getenv("AGENT_FACTORY_ADHOC_RUN") != "1" {
		t.Skip("adhoc test - set AGENT_FACTORY_ADHOC_RUN=1 to run manually")
	}

	factoryDir := getenv("AGENT_FACTORY_ADHOC_DIR", adhocFixtureDir(t))
	args := []string{"run", "--dir", factoryDir, "-d", "--continuously", "--record", "./adhoc-recording-batch-2.json", "--with-mock-workers"}
	// if recordPath := os.Getenv("AGENT_FACTORY_ADHOC_RECORD"); recordPath != "" {
	// 	args = append(args, "--record", recordPath)
	// }
	// if replayPath := os.Getenv("AGENT_FACTORY_ADHOC_REPLAY"); replayPath != "" {
	// 	args = append(args, "--replay", replayPath)
	// }
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current working directory: %v", err)
	}
	fmt.Printf("current working directory: %s\n", wd)
	rootCmd := cli.NewRootCommand()

	var stdout, stderr bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stderr)
	rootCmd.SetArgs(args)

	t.Logf("running: factory %v", args)

	if err := rootCmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("prepare failed: %v\nstderr: %s", err, stderr.String())
	}

	t.Logf("stdout:\n%s", stdout.String())
	if stderr.Len() > 0 {
		t.Logf("stderr:\n%s", stderr.String())
	}
}

// TestAdHocRecordReplaySmoke records the checked-in adhoc fixture with a mock
// provider and then replays the generated artifact from its embedded
// configuration.
func TestAdHocRecordReplaySmoke(t *testing.T) {
	sourceFixture := getenv("AGENT_FACTORY_ADHOC_DIR", adhocFixtureDir(t))
	runDir := testutil.CopyFixtureDir(t, sourceFixture)
	clearAdhocInputs(t, runDir)

	artifactPath := getenv("AGENT_FACTORY_ADHOC_ARTIFACT", filepath.Join(t.TempDir(), "adhoc-record-replay.json"))
	testutil.WriteSeedRequest(t, runDir, interfaces.SubmitRequest{
		WorkTypeID: "task",
		WorkID:     "adhoc-replay-task",
		TraceID:    "adhoc-replay-trace",
		Name:       "adhoc-record-replay-design",
		Payload:    []byte("exercise record/replay smoke path"),
	})

	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "Processed by adhoc record smoke. <COMPLETE>"},
		interfaces.InferenceResponse{Content: "Reviewed by adhoc record smoke. <COMPLETE>"},
	)
	recordHarness := testutil.NewServiceTestHarness(t, runDir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithRecordPath(artifactPath),
	)
	recordHarness.RunUntilComplete(t, 30*time.Second)
	if got := provider.CallCount(); got != 2 {
		t.Fatalf("mock provider call count = %d, want 2", got)
	}

	artifact := testutil.LoadReplayArtifact(t, artifactPath)
	dispatchCount := replayEventCount(artifact, factoryapi.FactoryEventTypeDispatchRequest)
	completionCount := replayEventCount(artifact, factoryapi.FactoryEventTypeDispatchResponse)
	if dispatchCount == 0 {
		t.Fatalf("recorded artifact %s has no dispatches", artifactPath)
	}
	if completionCount == 0 {
		t.Fatalf("recorded artifact %s has no completions", artifactPath)
	}

	replayHarness := testutil.NewReplayHarness(t, artifactPath)
	if err := replayHarness.RunUntilComplete(30 * time.Second); err != nil {
		var divergence *replay.DivergenceError
		if errors.As(err, &divergence) {
			t.Fatalf("record/replay smoke diverged for %s: %#v", artifactPath, divergence.Report)
		}
		t.Fatalf("record/replay smoke replay failed for %s: %v", artifactPath, err)
	}
	replayHarness.Service.Assert().
		HasTokenInPlace("task:complete").
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:in-review").
		HasNoTokenInPlace("task:failed")

	t.Logf("record/replay smoke artifact: %s", artifactPath)
	t.Logf("record/replay smoke result: replay succeeded with %d dispatches and %d completions", dispatchCount, completionCount)
}

func TestAdHocApril11ReplayDrainsNonTerminalWork(t *testing.T) {
	artifactPath := testpath.MustRepoPathFromCaller(t, 0, "tests", "adhoc", "factory-recording-04-11-02.json")
	if _, err := os.Stat(artifactPath); err != nil {
		t.Skipf("historical replay artifact not present in this checkout: %v", err)
	}
	artifact := testutil.LoadReplayArtifact(t, artifactPath)
	dispatchCount := replayEventCount(artifact, factoryapi.FactoryEventTypeDispatchRequest)
	completionCount := replayEventCount(artifact, factoryapi.FactoryEventTypeDispatchResponse)
	if dispatchCount == 0 {
		t.Fatal("expected April 11 replay artifact to contain dispatches")
	}
	if completionCount == 0 {
		t.Fatal("expected April 11 replay artifact to contain completions")
	}
	if missing := dispatchCount - completionCount; missing != 3 {
		t.Fatalf("April 11 replay artifact missing completions = %d, want 3", missing)
	}

	h := testutil.NewReplayHarness(t, artifactPath)
	if err := h.RunUntilComplete(10 * time.Second); err != nil {
		t.Fatalf("April 11 replay did not drain: %v", err)
	}

	snapshot, err := h.Service.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("get April 11 replay snapshot: %v", err)
	}
	if snapshot.RuntimeStatus != interfaces.RuntimeStatusFinished {
		t.Fatalf("April 11 replay runtime status = %q, want %q", snapshot.RuntimeStatus, interfaces.RuntimeStatusFinished)
	}
	stuck := nonTerminalWorkItems(snapshot)
	if len(stuck) > 0 {
		t.Fatalf("April 11 replay left non-terminal work item(s): %s", strings.Join(stuck, ", "))
	}
	if len(snapshot.Dispatches) != 0 || snapshot.InFlightCount != 0 {
		t.Fatalf("April 11 replay left active dispatch state: dispatches=%d in_flight=%d", len(snapshot.Dispatches), snapshot.InFlightCount)
	}
	for _, resource := range snapshot.Topology.Resources {
		placeID := state.PlaceID(resource.ID, interfaces.ResourceStateAvailable)
		if got := len(snapshot.Marking.TokensInPlace(placeID)); got != resource.Capacity {
			t.Fatalf("resource %s returned capacity = %d, want %d", placeID, got, resource.Capacity)
		}
	}
}

func replayEventCount(artifact *interfaces.ReplayArtifact, eventType factoryapi.FactoryEventType) int {
	count := 0
	for _, event := range artifact.Events {
		if event.Type == eventType {
			count++
		}
	}
	return count
}

func adhocFixtureDir(t *testing.T) string {
	t.Helper()
	return testpath.MustRepoPathFromCaller(t, 0, "tests", "adhoc", "factory")
}

func clearAdhocInputs(t *testing.T, dir string) {
	t.Helper()

	inputsDir := filepath.Join(dir, interfaces.InputsDir)
	if err := os.RemoveAll(inputsDir); err != nil {
		t.Fatalf("clear adhoc inputs: %v", err)
	}
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func nonTerminalWorkItems(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) []string {
	if snapshot == nil || snapshot.Topology == nil {
		return nil
	}

	var items []string
	for _, token := range snapshot.Marking.Tokens {
		if token == nil || token.Color.DataType != interfaces.DataTypeWork || token.Color.WorkID == "" {
			continue
		}
		category := snapshot.Topology.StateCategoryForPlace(token.PlaceID)
		if category == state.StateCategoryTerminal || category == state.StateCategoryFailed {
			continue
		}
		items = append(items, fmt.Sprintf("%s@%s", token.Color.WorkID, token.PlaceID))
	}
	for dispatchID, dispatch := range snapshot.Dispatches {
		if dispatch == nil {
			continue
		}
		for _, token := range dispatch.ConsumedTokens {
			if token.Color.DataType != interfaces.DataTypeWork || token.Color.WorkID == "" {
				continue
			}
			items = append(items, fmt.Sprintf("%s@active-dispatch:%s", token.Color.WorkID, dispatchID))
		}
	}
	sort.Strings(items)
	return items
}
