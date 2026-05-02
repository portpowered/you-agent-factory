//go:build functionallong

package workflow

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestWorkstationStopWords_FactoryJSON_Success(t *testing.T) {
	support.SkipLongFunctional(t, "slow workstation stop-word factory-json success sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "workstation_stopwords_factory_dir"))
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

func TestWorkstationStopWords_FactoryJSON_SecondWord(t *testing.T) {
	support.SkipLongFunctional(t, "slow workstation stop-word factory-json alternate token sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "workstation_stopwords_factory_dir"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "factory stop word second"}`))

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

func TestWorkstationStopWords_FactoryJSON_Failure(t *testing.T) {
	support.SkipLongFunctional(t, "slow workstation stop-word factory-json failure sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "workstation_stopwords_factory_dir"))
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

func TestWorkstationStopWords_Frontmatter_Success(t *testing.T) {
	support.SkipLongFunctional(t, "slow workstation stop-word frontmatter success sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "workstation_stopwords_frontmatter_dir"))
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

func TestWorkstationStopWords_Frontmatter_SecondWord(t *testing.T) {
	support.SkipLongFunctional(t, "slow workstation stop-word frontmatter alternate token sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "workstation_stopwords_frontmatter_dir"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "frontmatter stop word second"}`))

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

func TestWorkstationStopWords_Frontmatter_Failure(t *testing.T) {
	support.SkipLongFunctional(t, "slow workstation stop-word frontmatter failure sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "workstation_stopwords_frontmatter_dir"))
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

func TestWorkstationStopWords_Override_StationAcceptsWorkerRejects(t *testing.T) {
	support.SkipLongFunctional(t, "slow workstation stop-word override station-accepts sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "workstation_stopwords_override_dir"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "station overrides worker"}`))

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

func TestWorkstationStopWords_Override_StationRejectsWorkerAccepts(t *testing.T) {
	support.SkipLongFunctional(t, "slow workstation stop-word override station-rejects sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "workstation_stopwords_override_dir"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "station rejects worker accepts"}`))

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

func TestWorkstationStopWords_Override_BothMatch(t *testing.T) {
	support.SkipLongFunctional(t, "slow workstation stop-word override both-match sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "workstation_stopwords_override_dir"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "both match"}`))

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

func TestWorkstationStopWords_Override_NeitherMatch(t *testing.T) {
	support.SkipLongFunctional(t, "slow workstation stop-word override neither-match sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "workstation_stopwords_override_dir"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "neither match"}`))

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
