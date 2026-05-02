package functional_test

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

// --- Factory.json stop_words tests ---

// TestWorkstationStopWords_FactoryJSON_Success verifies that a workstation with
// stop_words declared in factory.json accepts when the provider output contains
// a matching stop word.
func TestWorkstationStopWords_FactoryJSON_Success(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "workstation_stopwords_factory_dir"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "factory stop word success"}`))

	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "Work completed successfully. COMPLETE"},
	)
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)
	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		HasTokenInPlace("task:complete").
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:failed")
}

// TestWorkstationStopWords_FactoryJSON_SecondWord verifies that the second
// stop word declared in factory.json also triggers acceptance.
func TestWorkstationStopWords_FactoryJSON_SecondWord(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "workstation_stopwords_factory_dir"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "factory stop word second"}`))

	// Content contains "DONE" (second stop word). Worker stop_token is "COMPLETE"
	// so worker rejects, but workstation stop_words override to ACCEPTED.
	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "All tasks finished. DONE"},
	)
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)
	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		HasTokenInPlace("task:complete").
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:failed")
}

// TestWorkstationStopWords_FactoryJSON_Failure verifies that when the provider
// output does NOT contain any stop word declared in factory.json, the
// workstation routes to the failure place.
func TestWorkstationStopWords_FactoryJSON_Failure(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "workstation_stopwords_factory_dir"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "factory stop word failure"}`))

	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "I tried but could not finish the work"},
	)
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)
	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		HasTokenInPlace("task:failed").
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:complete")
}

// --- AGENTS.md frontmatter stop_words tests ---

// TestWorkstationStopWords_Frontmatter_Success verifies that a workstation with
// stop_words declared in the AGENTS.md frontmatter accepts when the provider
// output contains a matching stop word.
func TestWorkstationStopWords_Frontmatter_Success(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "workstation_stopwords_frontmatter_dir"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "frontmatter stop word success"}`))

	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "Work completed successfully. COMPLETE"},
	)
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)
	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		HasTokenInPlace("task:complete").
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:failed")
}

// TestWorkstationStopWords_Frontmatter_SecondWord verifies that the second
// stop word declared in the AGENTS.md frontmatter also triggers acceptance.
func TestWorkstationStopWords_Frontmatter_SecondWord(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "workstation_stopwords_frontmatter_dir"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "frontmatter stop word second"}`))

	// Content contains "DONE" (second stop word). Worker stop_token is "COMPLETE"
	// so worker rejects, but workstation stop_words override to ACCEPTED.
	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "All tasks finished. DONE"},
	)
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)
	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		HasTokenInPlace("task:complete").
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:failed")
}

// TestWorkstationStopWords_Frontmatter_Failure verifies that when the provider
// output does NOT contain any stop word declared in the AGENTS.md frontmatter,
// the workstation routes to the failure place.
func TestWorkstationStopWords_Frontmatter_Failure(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "workstation_stopwords_frontmatter_dir"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "frontmatter stop word failure"}`))

	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "I tried but could not finish the work"},
	)
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)
	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		HasTokenInPlace("task:failed").
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:complete")
}

// --- Workstation stop_words overriding worker stop_token tests ---
//
// The override fixture uses:
//   - Worker stop_token: "WORKER_COMPLETE" (worker-level, evaluated in AgentExecutor)
//   - Workstation stop_words: ["STATION_COMPLETE", "STATION_DONE"] (factory.json, evaluated in TransitionerSubsystem)
//
// The workstation stop_words are evaluated AFTER the worker stop_token and override the outcome.
// With MockProvider, the full pipeline is exercised: Provider → AgentExecutor → stop_token
// evaluation → WorkResult.Output set → TransitionerSubsystem.processResult → stop_words override.

// TestWorkstationStopWords_Override_StationAcceptsWorkerRejects verifies that
// when the provider output contains a workstation stop word but NOT the worker
// stop token, the workstation-level evaluation overrides the worker REJECTED
// outcome to ACCEPTED.
func TestWorkstationStopWords_Override_StationAcceptsWorkerRejects(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "workstation_stopwords_override_dir"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "station overrides worker"}`))

	// Output contains workstation stop word "STATION_COMPLETE" but NOT worker stop token "WORKER_COMPLETE".
	// Worker evaluateOutcome → REJECTED (no worker stop token match).
	// Workstation evaluateStopWords → ACCEPTED (station stop word found) — overrides.
	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "The work is finished. STATION_COMPLETE"},
	)
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)
	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		HasTokenInPlace("task:complete").
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:failed")
}

// TestWorkstationStopWords_Override_StationRejectsWorkerAccepts verifies that
// when the provider output contains the worker stop token but NOT any workstation
// stop word, the workstation-level evaluation overrides the worker ACCEPTED
// outcome to FAILED.
func TestWorkstationStopWords_Override_StationRejectsWorkerAccepts(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "workstation_stopwords_override_dir"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "station rejects worker accepts"}`))

	// Output contains worker stop token "WORKER_COMPLETE" but NOT any workstation stop word.
	// Worker evaluateOutcome → ACCEPTED (worker stop token found).
	// Workstation evaluateStopWords → FAILED (no station stop word found) — overrides.
	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "The work is done. WORKER_COMPLETE"},
	)
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)
	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		HasTokenInPlace("task:failed").
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:complete")
}

// TestWorkstationStopWords_Override_BothMatch verifies that when the provider
// output contains both the worker stop token and a workstation stop word,
// the workstation-level evaluation confirms ACCEPTED (both agree).
func TestWorkstationStopWords_Override_BothMatch(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "workstation_stopwords_override_dir"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "both match"}`))

	// Output contains both worker stop token and workstation stop word.
	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "WORKER_COMPLETE and STATION_COMPLETE"},
	)
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)
	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		HasTokenInPlace("task:complete").
		HasNoTokenInPlace("task:failed")
}

// TestWorkstationStopWords_Override_NeitherMatch verifies that when the provider
// output contains neither the worker stop token nor any workstation stop word,
// the workstation-level evaluation confirms FAILED (both agree on failure).
func TestWorkstationStopWords_Override_NeitherMatch(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "workstation_stopwords_override_dir"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "neither match"}`))

	// Output contains neither worker stop token nor workstation stop word.
	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "I tried but could not finish the work"},
	)
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)
	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		HasTokenInPlace("task:failed").
		HasNoTokenInPlace("task:complete")
}
