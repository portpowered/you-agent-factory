package projections

import (
	"reflect"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil/runtimefixtures"

	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/petri"
)

func TestProjectInitialStructure_NilNet_ReturnsEmptyPayload(t *testing.T) {
	got := ProjectInitialStructure(nil)

	if !reflect.DeepEqual(got, interfaces.InitialStructurePayload{}) {
		t.Fatalf("ProjectInitialStructure(nil) = %#v, want empty payload", got)
	}
}

func TestProjectInitialStructure_NetOnlyTopology_ProjectsCanonicalPayload(t *testing.T) {
	net := representativeProjectionNet()

	got := ProjectInitialStructure(net)

	want := interfaces.InitialStructurePayload{
		Resources: []interfaces.FactoryResource{
			{ID: "cpu", Name: "CPU slots", Capacity: 4},
			{ID: "gpu", Name: "GPU slots", Capacity: 2},
		},
		WorkTypes: []interfaces.FactoryWorkType{
			{
				ID:   "bug",
				Name: "Bug",
				States: []interfaces.FactoryStateDefinition{
					{Value: "init", Category: "INITIAL"},
					{Value: "closed", Category: "TERMINAL"},
				},
			},
			{
				ID:   "story",
				Name: "Story",
				States: []interfaces.FactoryStateDefinition{
					{Value: "init", Category: "INITIAL"},
					{Value: "review", Category: "PROCESSING"},
					{Value: "done", Category: "TERMINAL"},
					{Value: "failed", Category: "FAILED"},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstation{
			{
				ID:                "build",
				Name:              "Build",
				WorkerID:          "builder",
				InputPlaceIDs:     []string{"cpu:available", "story:init"},
				OutputPlaceIDs:    []string{"cpu:available", "story:review"},
				RejectionPlaceIDs: []string{"story:init"},
				FailurePlaceIDs:   []string{"story:failed"},
			},
			{
				ID:             "review",
				Name:           "Review",
				WorkerID:       "reviewer",
				InputPlaceIDs:  []string{"gpu:available", "story:review"},
				OutputPlaceIDs: []string{"gpu:available", "story:done"},
			},
		},
		Places: []interfaces.FactoryPlace{
			{ID: "bug:closed", TypeID: "bug", State: "closed", Category: "TERMINAL"},
			{ID: "bug:init", TypeID: "bug", State: "init", Category: "INITIAL"},
			{ID: "cpu:available", TypeID: "cpu", State: "available", Category: "PROCESSING"},
			{ID: "gpu:available", TypeID: "gpu", State: "available", Category: "PROCESSING"},
			{ID: "story:done", TypeID: "story", State: "done", Category: "TERMINAL"},
			{ID: "story:failed", TypeID: "story", State: "failed", Category: "FAILED"},
			{ID: "story:init", TypeID: "story", State: "init", Category: "INITIAL"},
			{ID: "story:review", TypeID: "story", State: "review", Category: "PROCESSING"},
		},
		Relations: []interfaces.FactoryRelation{
			{Type: "INPUT", TargetWorkID: "story:init", RequiredState: "work"},
			{Type: "INPUT", TargetWorkID: "cpu:available", RequiredState: "cpu"},
			{Type: "OUTPUT", SourceWorkID: "build", TargetWorkID: "story:review"},
			{Type: "OUTPUT", SourceWorkID: "build", TargetWorkID: "cpu:available"},
			{Type: "INPUT", TargetWorkID: "story:review", RequiredState: "work"},
			{Type: "INPUT", TargetWorkID: "gpu:available", RequiredState: "gpu"},
			{Type: "OUTPUT", SourceWorkID: "review", TargetWorkID: "story:done"},
			{Type: "OUTPUT", SourceWorkID: "review", TargetWorkID: "gpu:available"},
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ProjectInitialStructure() mismatch\n got: %#v\nwant: %#v", got, want)
	}
}

func TestProjectInitialStructure_NetOnlyTopology_OrdersMapDerivedOutputDeterministically(t *testing.T) {
	net := representativeProjectionNet()

	first := ProjectInitialStructure(net)
	for range 20 {
		got := ProjectInitialStructure(net)
		if !reflect.DeepEqual(got, first) {
			t.Fatalf("ProjectInitialStructure() changed across runs\nfirst: %#v\n got: %#v", first, got)
		}
	}
}

func TestProjectInitialStructure_RuntimeConfig_ProjectsLoadedWorkerMetadata(t *testing.T) {
	net := representativeProjectionNet()
	runtimeConfig := projectionRuntimeConfig{
		Workers: map[string]*interfaces.WorkerConfig{
			"builder": {
				Type:             interfaces.WorkerTypeModel,
				ExecutorProvider: "codex-cli",
				ModelProvider:    "openai",
				Model:            "gpt-5.4",
				SessionID:        "sess-builder",
			},
			"reviewer": {
				Type:             interfaces.WorkerTypeModel,
				ExecutorProvider: "claude-cli",
				ModelProvider:    "anthropic",
				Model:            "claude-sonnet-4-5",
			},
		},
	}

	got := ProjectInitialStructure(net, runtimeConfig)

	want := []interfaces.FactoryWorker{
		{
			ID:            "builder",
			Name:          "builder",
			Provider:      "script_wrap",
			ModelProvider: "codex",
			Model:         "gpt-5.4",
			Config: map[string]string{
				"type": interfaces.WorkerTypeModel,
			},
		},
		{
			ID:            "reviewer",
			Name:          "reviewer",
			Provider:      "script_wrap",
			ModelProvider: "claude",
			Model:         "claude-sonnet-4-5",
			Config: map[string]string{
				"type": interfaces.WorkerTypeModel,
			},
		},
	}
	if !reflect.DeepEqual(got.Workers, want) {
		t.Fatalf("ProjectInitialStructure(...).Workers = %#v, want %#v", got.Workers, want)
	}
	if got.Workstations[0].WorkerID != "builder" || got.Workstations[1].WorkerID != "reviewer" {
		t.Fatalf("workstations changed worker references: %#v", got.Workstations)
	}
}

func TestProjectInitialStructure_RuntimeConfig_MissingWorkerKeepsWorkstationTopology(t *testing.T) {
	net := representativeProjectionNet()
	runtimeConfig := projectionRuntimeConfig{
		Workers: map[string]*interfaces.WorkerConfig{
			"reviewer": {
				Type:             interfaces.WorkerTypeModel,
				ExecutorProvider: "claude-cli",
				ModelProvider:    "anthropic",
				Model:            "claude-sonnet-4-5",
			},
		},
	}

	got := ProjectInitialStructure(net, runtimeConfig)

	if len(got.Workers) != 1 || got.Workers[0].ID != "reviewer" {
		t.Fatalf("Workers = %#v, want only reviewer metadata", got.Workers)
	}
	if !reflect.DeepEqual(got.Workstations, ProjectInitialStructure(net).Workstations) {
		t.Fatalf("Workstations = %#v, want net-derived topology", got.Workstations)
	}
}

// portos:func-length-exception owner=agent-factory reason=topology-projection-contract-fixture review=2026-07-18 removal=split-runtime-config-projection-assertions-before-next-topology-expansion
func TestProjectInitialStructure_RuntimeConfig_ProjectsConstraintsAndWorkstationMetadata(t *testing.T) {
	net := representativeProjectionNet()
	net.Limits = state.GlobalLimits{
		MaxTokenAge:    2 * time.Hour,
		MaxTotalVisits: 7,
	}
	net.Transitions["build"].InputArcs[0].Guard = &petri.VisitCountGuard{
		TransitionID: "build",
		MaxVisits:    3,
	}
	runtimeConfig := projectionRuntimeConfig{
		Workers: map[string]*interfaces.WorkerConfig{
			"builder": {
				Type:        interfaces.WorkerTypeModel,
				Concurrency: 2,
				Timeout:     "30m",
			},
		},
		Workstations: map[string]*interfaces.FactoryWorkstationConfig{
			"build": {
				Name:           "Build",
				Kind:           interfaces.WorkstationKindCron,
				Type:           interfaces.WorkstationTypeModel,
				WorkerTypeName: "builder",
				Cron: &interfaces.CronConfig{
					Schedule:       "*/5 * * * *",
					TriggerAtStart: true,
					Jitter:         "30s",
					ExpiryWindow:   "2m",
				},
				Resources: []interfaces.ResourceConfig{{Name: "cpu", Capacity: 1}},
				Guards: []interfaces.GuardConfig{
					{Type: interfaces.GuardTypeVisitCount, Workstation: "Build", MaxVisits: 3},
				},
				PromptFile:       "prompt.md",
				OutputSchema:     "schema.json",
				Timeout:          "10m",
				Worktree:         "{{.worktree}}",
				WorkingDirectory: "{{.working_directory}}",
				Limits: interfaces.WorkstationLimits{
					MaxRetries:       2,
					MaxExecutionTime: "10m",
				},
				StopWords: []string{"DONE", "STOP"},
			},
		},
	}

	got := ProjectInitialStructure(net, runtimeConfig)

	wantConstraints := []interfaces.FactoryConstraint{
		{
			ID:    "global/limits",
			Type:  "global_limit",
			Scope: "global",
			Values: map[string]string{
				"max_token_age":    "2h0m0s",
				"max_total_visits": "7",
			},
		},
		{
			ID:    "worker/builder/concurrency",
			Type:  "worker_concurrency",
			Scope: "worker:builder",
			Values: map[string]string{
				"max_concurrency": "2",
			},
		},
		{
			ID:    "worker/builder/timeout",
			Type:  "worker_timeout",
			Scope: "worker:builder",
			Values: map[string]string{
				"timeout": "30m",
			},
		},
		{
			ID:    "workstation/build/config-guard/0",
			Type:  "configured_guard",
			Scope: "workstation:build",
			Values: map[string]string{
				"type":        string(interfaces.GuardTypeVisitCount),
				"workstation": "Build",
				"max_visits":  "3",
			},
		},
		{
			ID:    "workstation/build/cron",
			Type:  "cron_trigger",
			Scope: "workstation:build",
			Values: map[string]string{
				"expiry_window":    "2m",
				"jitter":           "30s",
				"schedule":         "*/5 * * * *",
				"trigger_at_start": "true",
			},
		},
		{
			ID:    "workstation/build/input/0/guard",
			Type:  "visit_count_guard",
			Scope: "workstation:build",
			Values: map[string]string{
				"arc_set":               "input",
				"binding":               "work",
				"cardinality":           "ONE",
				"max_visits":            "3",
				"mode":                  "CONSUME",
				"place_id":              "story:init",
				"watched_transition_id": "build",
			},
		},
		{
			ID:    "workstation/build/limits",
			Type:  "workstation_limit",
			Scope: "workstation:build",
			Values: map[string]string{
				"max_execution_time": "10m",
				"max_retries":        "2",
			},
		},
		{
			ID:    "workstation/build/resource/cpu/0",
			Type:  "resource_usage",
			Scope: "workstation:build",
			Values: map[string]string{
				"capacity":    "1",
				"resource_id": "cpu",
			},
		},
		{
			ID:    "workstation/build/stop-words",
			Type:  "stop_words",
			Scope: "workstation:build",
			Values: map[string]string{
				"words": "DONE,STOP",
			},
		},
	}
	if !reflect.DeepEqual(got.Constraints, wantConstraints) {
		t.Fatalf("Constraints = %#v, want %#v", got.Constraints, wantConstraints)
	}
	assertSingleConstraint(t, got.Constraints, "workstation/build/stop-words")
	assertSingleConstraint(t, got.Constraints, "workstation/build/limits")

	wantConfig := map[string]string{
		"configured_worker": "builder",
		"kind":              string(interfaces.WorkstationKindCron),
		"output_schema":     "schema.json",
		"prompt_file":       "prompt.md",
		"type":              interfaces.WorkstationTypeModel,
		"worker":            "builder",
		"worktree":          "{{.worktree}}",
		"working_directory": "{{.working_directory}}",
	}
	if !reflect.DeepEqual(got.Workstations[0].Config, wantConfig) {
		t.Fatalf("Workstations[0].Config = %#v, want %#v", got.Workstations[0].Config, wantConfig)
	}
	if got.Workstations[0].Kind != "CRON" {
		t.Fatalf("Workstations[0].Kind = %q, want CRON", got.Workstations[0].Kind)
	}
}

func TestProjectInitialStructure_RuntimeConfig_LimitsConstraintUsesRuntimeConfig(t *testing.T) {
	net := representativeProjectionNet()
	runtimeConfig := projectionRuntimeConfig{
		Workstations: map[string]*interfaces.FactoryWorkstationConfig{
			"build": {
				Name:   "Build",
				Limits: interfaces.WorkstationLimits{MaxRetries: 2, MaxExecutionTime: "10m"},
			},
		},
	}

	got := ProjectInitialStructure(net, runtimeConfig)

	assertSingleConstraint(t, got.Constraints, "workstation/build/limits")
	for _, constraint := range got.Constraints {
		if constraint.ID != "workstation/build/limits" {
			continue
		}
		if constraint.Values["max_retries"] != "2" {
			t.Fatalf("limits max_retries = %q, want 2 from runtime config", constraint.Values["max_retries"])
		}
		if constraint.Values["max_execution_time"] != "10m" {
			t.Fatalf("limits max_execution_time = %q, want 10m from runtime config", constraint.Values["max_execution_time"])
		}
		return
	}

	t.Fatalf("missing workstation/build/limits constraint in %#v", got.Constraints)
}

func assertSingleConstraint(t *testing.T, constraints []interfaces.FactoryConstraint, id string) {
	t.Helper()
	count := 0
	for _, constraint := range constraints {
		if constraint.ID == id {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("constraint %q count = %d, want 1 in %#v", id, count, constraints)
	}
}

type projectionRuntimeConfig = runtimefixtures.RuntimeDefinitionLookupFixture

func representativeProjectionNet() *state.Net {
	story := &state.WorkType{
		ID:   "story",
		Name: "Story",
		States: []state.StateDefinition{
			{Value: "init", Category: state.StateCategoryInitial},
			{Value: "review", Category: state.StateCategoryProcessing},
			{Value: "done", Category: state.StateCategoryTerminal},
			{Value: "failed", Category: state.StateCategoryFailed},
		},
	}
	bug := &state.WorkType{
		ID:   "bug",
		Name: "Bug",
		States: []state.StateDefinition{
			{Value: "init", Category: state.StateCategoryInitial},
			{Value: "closed", Category: state.StateCategoryTerminal},
		},
	}

	return &state.Net{
		ID: "projection-net",
		Places: map[string]*petri.Place{
			"story:review":  {ID: "story:review", TypeID: "story", State: "review"},
			"story:init":    {ID: "story:init", TypeID: "story", State: "init"},
			"story:failed":  {ID: "story:failed", TypeID: "story", State: "failed"},
			"story:done":    {ID: "story:done", TypeID: "story", State: "done"},
			"cpu:available": {ID: "cpu:available", TypeID: "cpu", State: "available"},
			"gpu:available": {ID: "gpu:available", TypeID: "gpu", State: "available"},
			"bug:init":      {ID: "bug:init", TypeID: "bug", State: "init"},
			"bug:closed":    {ID: "bug:closed", TypeID: "bug", State: "closed"},
		},
		Transitions: map[string]*petri.Transition{
			"review": {
				ID:         "review",
				Name:       "Review",
				WorkerType: "reviewer",
				InputArcs: []petri.Arc{
					{Name: "work", PlaceID: "story:review"},
					{Name: "gpu", PlaceID: "gpu:available"},
				},
				OutputArcs: []petri.Arc{
					{PlaceID: "story:done"},
					{PlaceID: "gpu:available"},
				},
			},
			"build": {
				ID:         "build",
				Name:       "Build",
				WorkerType: "builder",
				InputArcs: []petri.Arc{
					{Name: "work", PlaceID: "story:init"},
					{Name: "cpu", PlaceID: "cpu:available"},
				},
				OutputArcs: []petri.Arc{
					{PlaceID: "story:review"},
					{PlaceID: "cpu:available"},
				},
				RejectionArcs: []petri.Arc{
					{PlaceID: "story:init"},
				},
				FailureArcs: []petri.Arc{
					{PlaceID: "story:failed"},
				},
			},
		},
		WorkTypes: map[string]*state.WorkType{
			"story": story,
			"bug":   bug,
		},
		Resources: map[string]*state.ResourceDef{
			"gpu": {ID: "gpu", Name: "GPU slots", Capacity: 2},
			"cpu": {ID: "cpu", Name: "CPU slots", Capacity: 4},
		},
	}
}
