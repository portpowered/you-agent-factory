//go:build functionallong

package guards_batch

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestMatchesFieldsGuard_FixtureBoundaryMapsToRuntimeConfig(t *testing.T) {
	support.SkipLongFunctional(t, "slow matches-fields fixture-boundary sweep")
	t.Parallel()

	tests := []struct {
		name            string
		fixtureName     string
		workstationName string
		workerName      string
		inputCount      int
		inputKey        string
	}{
		{name: "single input name selector", fixtureName: "matches_fields_single_input_dir", workstationName: "match-asset", workerName: "matcher", inputCount: 1, inputKey: ".Name"},
		{name: "pair tag selector", fixtureName: "matches_fields_pair_guard_dir", workstationName: "match-pair", workerName: "matcher", inputCount: 2, inputKey: `.Tags["flavor"]`},
		{name: "triple nested tag selector", fixtureName: "matches_fields_triple_guard_dir", workstationName: "match-triple", workerName: "matcher", inputCount: 3, inputKey: `.Tags["_last_output"]`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, tt.fixtureName))
			factoryJSON, err := os.ReadFile(filepath.Join(dir, interfaces.FactoryConfigFile))
			if err != nil {
				t.Fatalf("read factory.json: %v", err)
			}

			generated, err := config.GeneratedFactoryFromOpenAPIJSON(factoryJSON)
			if err != nil {
				t.Fatalf("GeneratedFactoryFromOpenAPIJSON: %v", err)
			}
			if generated.Workstations == nil || len(*generated.Workstations) != 1 {
				t.Fatalf("generated workstations = %#v, want one guarded workstation", generated.Workstations)
			}

			workstation := (*generated.Workstations)[0]
			if workstation.Name != tt.workstationName {
				t.Fatalf("generated workstation name = %q, want %s", workstation.Name, tt.workstationName)
			}
			if len(workstation.Inputs) != tt.inputCount {
				t.Fatalf("generated inputs = %#v, want %d input(s)", workstation.Inputs, tt.inputCount)
			}
			if workstation.Guards == nil || len(*workstation.Guards) != 1 {
				t.Fatalf("generated workstation guards = %#v, want one matches-fields guard", workstation.Guards)
			}

			guard := (*workstation.Guards)[0]
			if guard.Type != factoryapi.WorkstationGuardTypeMatchesFields {
				t.Fatalf("generated guard type = %q, want MATCHES_FIELDS", guard.Type)
			}
			if guard.MatchConfig == nil || guard.MatchConfig.InputKey != tt.inputKey {
				t.Fatalf("generated matchConfig = %#v, want inputKey %s", guard.MatchConfig, tt.inputKey)
			}

			loaded, err := config.LoadRuntimeConfig(dir, nil)
			if err != nil {
				t.Fatalf("LoadRuntimeConfig: %v", err)
			}

			matcher, ok := loaded.Worker(tt.workerName)
			if !ok {
				t.Fatalf("expected %s worker definition", tt.workerName)
			}
			if matcher.Type != interfaces.WorkerTypeModel || matcher.StopToken != "COMPLETE" {
				t.Fatalf("worker runtime config = %#v", matcher)
			}

			runtimeWorkstation, ok := loaded.Workstation(tt.workstationName)
			if !ok {
				t.Fatalf("expected %s workstation definition", tt.workstationName)
			}
			if len(runtimeWorkstation.Inputs) != tt.inputCount || len(runtimeWorkstation.Guards) != 1 {
				t.Fatalf("runtime workstation = %#v, want %d input(s) and one workstation guard", runtimeWorkstation, tt.inputCount)
			}

			runtimeGuard := runtimeWorkstation.Guards[0]
			if runtimeGuard.Type != interfaces.GuardTypeMatchesFields {
				t.Fatalf("runtime guard type = %q, want matches_fields", runtimeGuard.Type)
			}
			if runtimeGuard.MatchConfig == nil || runtimeGuard.MatchConfig.InputKey != tt.inputKey {
				t.Fatalf("runtime matchConfig = %#v, want inputKey %s", runtimeGuard.MatchConfig, tt.inputKey)
			}
		})
	}
}

func TestMatchesFieldsGuard_SingleInputResolvedNameCompletes(t *testing.T) {
	support.SkipLongFunctional(t, "slow matches-fields single-input sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "matches_fields_single_input_dir"))
	provider := testutil.NewMockProvider(interfaces.InferenceResponse{Content: "single COMPLETE"})

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)
	h.SubmitFull(context.Background(), []interfaces.SubmitRequest{{
		Name:       "asset-alpha",
		WorkTypeID: "asset",
		TraceID:    "trace-match-single",
	}})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	errCh := h.RunInBackground(ctx)

	support.WaitForHarnessPlaceTokenCount(t, h, "asset:matched", 1, time.Second)
	support.WaitForHarnessPlaceTokenCount(t, h, "asset:ready", 0, time.Second)

	h.Assert().
		PlaceTokenCount("asset:matched", 1).
		HasNoTokenInPlace("asset:ready")

	if provider.CallCount() != 1 {
		t.Fatalf("expected matcher provider call once, got %d", provider.CallCount())
	}

	cancel()
	if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("factory run error: %v", err)
	}
}

func TestMatchesFieldsGuard_TwoInputMatchingTagsCompletesJoin(t *testing.T) {
	support.SkipLongFunctional(t, "slow matches-fields matching-tags sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "matches_fields_pair_guard_dir"))
	provider := testutil.NewMockProvider(interfaces.InferenceResponse{Content: "pair COMPLETE"})

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)
	h.SubmitFull(context.Background(), []interfaces.SubmitRequest{{
		Name:       "draft-alpha",
		WorkTypeID: "draft",
		TraceID:    "trace-match-pair-draft",
		Tags:       map[string]string{"flavor": "vanilla"},
	}})
	h.SubmitFull(context.Background(), []interfaces.SubmitRequest{{
		Name:       "review-beta",
		WorkTypeID: "review",
		TraceID:    "trace-match-pair-review",
		Tags:       map[string]string{"flavor": "vanilla"},
	}})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	errCh := h.RunInBackground(ctx)

	support.WaitForHarnessPlaceTokenCount(t, h, "pair:matched", 1, time.Second)
	support.WaitForHarnessPlaceTokenCount(t, h, "draft:ready", 0, time.Second)
	support.WaitForHarnessPlaceTokenCount(t, h, "review:ready", 0, time.Second)

	h.Assert().
		PlaceTokenCount("pair:matched", 1).
		HasNoTokenInPlace("draft:ready").
		HasNoTokenInPlace("review:ready")

	if provider.CallCount() != 1 {
		t.Fatalf("expected matcher provider call once, got %d", provider.CallCount())
	}

	cancel()
	if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("factory run error: %v", err)
	}
}

func TestMatchesFieldsGuard_TwoInputMismatchedTagsStayBlocked(t *testing.T) {
	support.SkipLongFunctional(t, "slow matches-fields mismatched-tags sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "matches_fields_pair_guard_dir"))
	provider := testutil.NewMockProvider(interfaces.InferenceResponse{Content: "pair COMPLETE"})

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)
	h.SubmitFull(context.Background(), []interfaces.SubmitRequest{{
		Name:       "draft-alpha",
		WorkTypeID: "draft",
		TraceID:    "trace-mismatch-pair-draft",
		Tags:       map[string]string{"flavor": "vanilla"},
	}})
	h.SubmitFull(context.Background(), []interfaces.SubmitRequest{{
		Name:       "review-beta",
		WorkTypeID: "review",
		TraceID:    "trace-mismatch-pair-review",
		Tags:       map[string]string{"flavor": "chocolate"},
	}})

	assertMatchesFieldsHarnessBlocked(t, h, provider, []string{"draft:ready", "review:ready"}, "pair:matched")
}

func TestMatchesFieldsGuard_ThreeInputNestedTagMismatchRejectsCandidateSet(t *testing.T) {
	support.SkipLongFunctional(t, "slow matches-fields nested-tag mismatch sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "matches_fields_triple_guard_dir"))
	provider := testutil.NewMockProvider(interfaces.InferenceResponse{Content: "triple COMPLETE"})

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)
	h.SubmitFull(context.Background(), []interfaces.SubmitRequest{{
		Name:       "first-alpha",
		WorkTypeID: "first",
		TraceID:    "trace-match-triple-first",
		Tags:       map[string]string{"_last_output": "model-a"},
	}})
	h.SubmitFull(context.Background(), []interfaces.SubmitRequest{{
		Name:       "second-beta",
		WorkTypeID: "second",
		TraceID:    "trace-match-triple-second",
		Tags:       map[string]string{"_last_output": "model-a"},
	}})
	h.SubmitFull(context.Background(), []interfaces.SubmitRequest{{
		Name:       "third-gamma",
		WorkTypeID: "third",
		TraceID:    "trace-match-triple-third",
		Tags:       map[string]string{"_last_output": "model-b"},
	}})

	assertMatchesFieldsHarnessBlocked(t, h, provider, []string{"first:ready", "second:ready", "third:ready"}, "triple:matched")
}

func TestMatchesFieldsGuard_IntegrationSmoke_GroupedExecution(t *testing.T) {
	support.SkipLongFunctional(t, "slow matches-fields grouped-execution sweep")
	t.Run("matching pair dispatches through normal path", func(t *testing.T) {
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "matches_fields_pair_guard_dir"))
		assertMatchesFieldsPairFixtureContract(t, dir)

		provider := testutil.NewMockProvider(interfaces.InferenceResponse{Content: "pair COMPLETE"})
		h := testutil.NewServiceTestHarness(t, dir,
			testutil.WithProvider(provider),
			testutil.WithFullWorkerPoolAndScriptWrap(),
		)

		h.SubmitFull(context.Background(), []interfaces.SubmitRequest{{
			Name:       "draft-alpha",
			WorkTypeID: "draft",
			TraceID:    "trace-integration-match-draft",
			Tags:       map[string]string{"flavor": "vanilla"},
		}})
		h.SubmitFull(context.Background(), []interfaces.SubmitRequest{{
			Name:       "review-beta",
			WorkTypeID: "review",
			TraceID:    "trace-integration-match-review",
			Tags:       map[string]string{"flavor": "vanilla"},
		}})

		h.RunUntilComplete(t, 10*time.Second)

		h.Assert().
			PlaceTokenCount("pair:matched", 1).
			HasNoTokenInPlace("draft:ready").
			HasNoTokenInPlace("review:ready")

		if provider.CallCount() != 1 {
			t.Fatalf("expected one matcher dispatch, got %d", provider.CallCount())
		}

		events, err := h.GetFactoryEvents(context.Background())
		if err != nil {
			t.Fatalf("GetFactoryEvents: %v", err)
		}
		if got := support.CountFactoryEvents(events, factoryapi.FactoryEventTypeDispatchRequest); got != 1 {
			t.Fatalf("DISPATCH_REQUEST events = %d, want 1", got)
		}
		if got := support.CountFactoryEvents(events, factoryapi.FactoryEventTypeDispatchResponse); got != 1 {
			t.Fatalf("DISPATCH_RESPONSE events = %d, want 1", got)
		}
	})

	t.Run("mismatched pair is rejected before dispatch", func(t *testing.T) {
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "matches_fields_pair_guard_dir"))
		assertMatchesFieldsPairFixtureContract(t, dir)

		provider := testutil.NewMockProvider(interfaces.InferenceResponse{Content: "pair COMPLETE"})
		h := testutil.NewServiceTestHarness(t, dir,
			testutil.WithProvider(provider),
			testutil.WithFullWorkerPoolAndScriptWrap(),
		)

		h.SubmitFull(context.Background(), []interfaces.SubmitRequest{{
			Name:       "draft-alpha",
			WorkTypeID: "draft",
			TraceID:    "trace-integration-mismatch-draft",
			Tags:       map[string]string{"flavor": "vanilla"},
		}})
		h.SubmitFull(context.Background(), []interfaces.SubmitRequest{{
			Name:       "review-beta",
			WorkTypeID: "review",
			TraceID:    "trace-integration-mismatch-review",
			Tags:       map[string]string{"flavor": "chocolate"},
		}})

		assertMatchesFieldsHarnessBlocked(t, h, provider, []string{"draft:ready", "review:ready"}, "pair:matched")

		events, err := h.GetFactoryEvents(context.Background())
		if err != nil {
			t.Fatalf("GetFactoryEvents: %v", err)
		}
		if got := support.CountFactoryEvents(events, factoryapi.FactoryEventTypeDispatchRequest); got != 0 {
			t.Fatalf("DISPATCH_REQUEST events = %d, want 0", got)
		}
		if got := support.CountFactoryEvents(events, factoryapi.FactoryEventTypeDispatchResponse); got != 0 {
			t.Fatalf("DISPATCH_RESPONSE events = %d, want 0", got)
		}
	})
}

func assertMatchesFieldsPairFixtureContract(t *testing.T, dir string) {
	t.Helper()

	factoryJSON, err := os.ReadFile(filepath.Join(dir, interfaces.FactoryConfigFile))
	if err != nil {
		t.Fatalf("read factory.json: %v", err)
	}
	generated, err := config.GeneratedFactoryFromOpenAPIJSON(factoryJSON)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromOpenAPIJSON: %v", err)
	}
	if generated.Workstations == nil || len(*generated.Workstations) != 1 {
		t.Fatalf("generated workstations = %#v, want one guarded workstation", generated.Workstations)
	}

	guard := (*generated.Workstations)[0].Guards
	if guard == nil || len(*guard) != 1 {
		t.Fatalf("generated workstation guards = %#v, want one guard", guard)
	}
	if (*guard)[0].Type != factoryapi.WorkstationGuardTypeMatchesFields {
		t.Fatalf("generated guard type = %q, want MATCHES_FIELDS", (*guard)[0].Type)
	}
	if (*guard)[0].MatchConfig == nil || (*guard)[0].MatchConfig.InputKey != `.Tags["flavor"]` {
		t.Fatalf("generated match config = %#v, want flavor selector", (*guard)[0].MatchConfig)
	}

	loaded, err := config.LoadRuntimeConfig(dir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}
	workstation, ok := loaded.Workstation("match-pair")
	if !ok {
		t.Fatal("expected match-pair workstation definition")
	}
	if len(workstation.Guards) != 1 || workstation.Guards[0].MatchConfig == nil {
		t.Fatalf("runtime workstation guards = %#v, want one match-config guard", workstation.Guards)
	}
	if workstation.Guards[0].Type != interfaces.GuardTypeMatchesFields || workstation.Guards[0].MatchConfig.InputKey != `.Tags["flavor"]` {
		t.Fatalf("runtime guard = %#v, want matches_fields flavor selector", workstation.Guards[0])
	}
}

func assertMatchesFieldsHarnessBlocked(
	t *testing.T,
	h *testutil.ServiceTestHarness,
	provider *testutil.MockProvider,
	readyPlaces []string,
	outputPlace string,
) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	errCh := h.RunInBackground(ctx)

	for _, placeID := range readyPlaces {
		support.WaitForHarnessPlaceTokenCount(t, h, placeID, 1, time.Second)
	}

	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if provider.CallCount() != 0 {
			t.Fatalf("expected matcher to remain blocked, got %d provider calls", provider.CallCount())
		}
		snapshot, err := h.GetEngineStateSnapshot()
		if err != nil {
			t.Fatalf("GetEngineStateSnapshot: %v", err)
		}
		if support.PlaceTokenCount(snapshot.Marking, outputPlace) != 0 {
			t.Fatalf("expected no matched output in %s, got marking %#v", outputPlace, snapshot.Marking.PlaceTokens)
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancel()
	if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("factory run error: %v", err)
	}
}
