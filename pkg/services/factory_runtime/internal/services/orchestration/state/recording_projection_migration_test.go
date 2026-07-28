package state_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	runtimestate "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

func TestProjectInitialStructure_NilNet_ReturnsEmptyPayload(t *testing.T) {
	got := runtimestate.ProjectInitialStructure(nil)

	if !reflect.DeepEqual(got, interfaces.InitialStructurePayload{}) {
		t.Fatalf("factoryruntime.ProjectInitialStructure(nil) = %#v, want empty payload", got)
	}
}

func TestProjectInitialStructure_NetOnlyTopology_ProjectsCanonicalPayload(t *testing.T) {
	net := representativeProjectionNet()

	got := runtimestate.ProjectInitialStructure(net)

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
				InputPlaceIDs:     []string{"story:init", "cpu:available"},
				OutputPlaceIDs:    []string{"story:review", "cpu:available"},
				RejectionPlaceIDs: []string{"story:init"},
				FailurePlaceIDs:   []string{"story:failed"},
			},
			{
				ID:             "review",
				Name:           "Review",
				WorkerID:       "reviewer",
				InputPlaceIDs:  []string{"story:review", "gpu:available"},
				OutputPlaceIDs: []string{"story:done", "gpu:available"},
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
		Relations: []work.FactoryRelation{
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
		t.Fatalf("factoryruntime.ProjectInitialStructure() mismatch\n got: %#v\nwant: %#v", got, want)
	}
}

func TestProjectInitialStructure_NetOnlyTopology_PreservesNonSuccessRouteArrayOrder(t *testing.T) {
	net := projectionNetWithOrderedNonSuccessRoutes()

	got := runtimestate.ProjectInitialStructure(net)
	if len(got.Workstations) != 1 {
		t.Fatalf("workstations = %#v, want one projected workstation", got.Workstations)
	}

	workstation := got.Workstations[0]
	if !reflect.DeepEqual(workstation.ContinuePlaceIDs, []string{"story:retry", "story:init"}) {
		t.Fatalf("continue routes = %#v, want authored order", workstation.ContinuePlaceIDs)
	}
	if !reflect.DeepEqual(workstation.RejectionPlaceIDs, []string{"story:triage", "story:init"}) {
		t.Fatalf("rejection routes = %#v, want authored order", workstation.RejectionPlaceIDs)
	}
	if !reflect.DeepEqual(workstation.FailurePlaceIDs, []string{"story:failed", "story:abandoned"}) {
		t.Fatalf("failure routes = %#v, want authored order", workstation.FailurePlaceIDs)
	}
}

func TestProjectInitialStructure_ConfigMappedCronImplicitFailureRoutesAppearInTopology(t *testing.T) {
	net := &factoryruntime.Net{
		Places: map[string]*factoryruntime.PetriPlace{
			"task:queued": {ID: "task:queued", TypeID: "task", State: "queued"},
			"task:done":   {ID: "task:done", TypeID: "task", State: "done"},
			"task:failed": {ID: "task:failed", TypeID: "task", State: "failed"},
		},
		Transitions: map[string]*factoryruntime.PetriTransition{
			"poll-for-work": {
				ID:          "poll-for-work",
				Name:        "poll-for-work",
				WorkerType:  "cron-worker",
				OutputArcs:  []factoryruntime.PetriArc{{PlaceID: "task:done"}},
				FailureArcs: []factoryruntime.PetriArc{{PlaceID: "task:failed"}},
			},
		},
		WorkTypes: map[string]*factoryruntime.WorkType{
			"task": {
				ID: "task",
				States: []factoryruntime.StateDefinition{
					{Value: "queued", Category: factoryruntime.StateCategoryInitial},
					{Value: "done", Category: factoryruntime.StateCategoryTerminal},
					{Value: "failed", Category: factoryruntime.StateCategoryFailed},
				},
			},
		},
	}

	got := runtimestate.ProjectInitialStructure(net)
	for _, workstation := range got.Workstations {
		if workstation.ID != "poll-for-work" {
			continue
		}
		if !reflect.DeepEqual(workstation.FailurePlaceIDs, []string{"task:failed"}) {
			t.Fatalf("failure routes = %#v, want implicit failed-state route", workstation.FailurePlaceIDs)
		}
		return
	}
	t.Fatalf("workstations = %#v, want projected cron workstation", got.Workstations)
}

func TestProjectInitialStructure_NetOnlyTopology_OrdersMapDerivedOutputDeterministically(t *testing.T) {
	net := representativeProjectionNet()

	first := runtimestate.ProjectInitialStructure(net)
	for range 20 {
		got := runtimestate.ProjectInitialStructure(net)
		if !reflect.DeepEqual(got, first) {
			t.Fatalf("factoryruntime.ProjectInitialStructure() changed across runs\nfirst: %#v\n got: %#v", first, got)
		}
	}
}

func TestProjectInitialStructure_RuntimeConfig_ProjectsLoadedWorkerMetadata(t *testing.T) {
	net := representativeProjectionNet()
	runtimeConfig := projectionRuntimeConfig{
		Workers: map[string]*interfaces.FactoryWorkerConfig{
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

	got := runtimestate.ProjectInitialStructure(net, runtimeConfig)

	want := []interfaces.FactoryWorker{
		{
			ID:            "builder",
			Name:          "builder",
			Provider:      "codex-cli",
			ModelProvider: "openai",
			Model:         "gpt-5.4",
			Config: map[string]string{
				"type": interfaces.WorkerTypeModel,
			},
		},
		{
			ID:            "reviewer",
			Name:          "reviewer",
			Provider:      "claude-cli",
			ModelProvider: "anthropic",
			Model:         "claude-sonnet-4-5",
			Config: map[string]string{
				"type": interfaces.WorkerTypeModel,
			},
		},
	}
	if !reflect.DeepEqual(got.Workers, want) {
		t.Fatalf("factoryruntime.ProjectInitialStructure(...).Workers = %#v, want %#v", got.Workers, want)
	}
	if got.Workstations[0].WorkerID != "builder" || got.Workstations[1].WorkerID != "reviewer" {
		t.Fatalf("workstations changed worker references: %#v", got.Workstations)
	}
}

func TestProjectInitialStructure_RuntimeConfig_ProjectsFactoryNameFromSharedLookup(t *testing.T) {
	net := representativeProjectionNet()
	runtimeConfig := projectionRuntimeConfig{
		Factory: &interfaces.FactoryConfig{Name: "runtime-factory"},
	}

	got := runtimestate.ProjectInitialStructure(net, runtimeConfig)

	if got.Name != "runtime-factory" {
		t.Fatalf("Name = %q, want runtime factory config name", got.Name)
	}
}

func TestProjectInitialStructure_RuntimeConfig_MissingFactoryLookupLeavesNameEmpty(t *testing.T) {
	net := representativeProjectionNet()

	got := runtimestate.ProjectInitialStructure(net, runtimeDefinitionOnlyFixture{})

	if got.Name != "" {
		t.Fatalf("Name = %q, want empty when runtime config has no shared factory config lookup", got.Name)
	}
}

func TestProjectInitialStructure_RuntimeConfig_NilFactoryConfigLeavesNameEmpty(t *testing.T) {
	net := representativeProjectionNet()
	runtimeConfig := projectionRuntimeConfig{}

	got := runtimestate.ProjectInitialStructure(net, runtimeConfig)

	if got.Name != "" {
		t.Fatalf("Name = %q, want empty when runtime factory config is nil", got.Name)
	}
}

func TestProjectInitialStructure_RuntimeConfig_MissingWorkerKeepsWorkstationTopology(t *testing.T) {
	net := representativeProjectionNet()
	runtimeConfig := projectionRuntimeConfig{
		Workers: map[string]*interfaces.FactoryWorkerConfig{
			"reviewer": {
				Type:             interfaces.WorkerTypeModel,
				ExecutorProvider: "claude-cli",
				ModelProvider:    "anthropic",
				Model:            "claude-sonnet-4-5",
			},
		},
	}

	got := runtimestate.ProjectInitialStructure(net, runtimeConfig)

	if len(got.Workers) != 1 || got.Workers[0].ID != "reviewer" {
		t.Fatalf("Workers = %#v, want only reviewer metadata", got.Workers)
	}
	if !reflect.DeepEqual(got.Workstations, runtimestate.ProjectInitialStructure(net).Workstations) {
		t.Fatalf("Workstations = %#v, want net-derived topology", got.Workstations)
	}
}

func TestProjectInitialStructure_RuntimeConfig_ProjectsConstraintsAndWorkstationMetadata(t *testing.T) {
	net, runtimeConfig := projectionNetAndRuntimeConfigWithConstraints()
	got := runtimestate.ProjectInitialStructure(net, runtimeConfig)

	wantConstraints := projectionRuntimeConstraints()
	if !reflect.DeepEqual(got.Constraints, wantConstraints) {
		t.Fatalf("Constraints = %#v, want %#v", got.Constraints, wantConstraints)
	}
	assertSingleConstraint(t, got.Constraints, "workstation/build/stop-words")
	assertSingleConstraint(t, got.Constraints, "workstation/build/limits")

	wantConfig := projectionRuntimeWorkstationConfig()
	if !reflect.DeepEqual(got.Workstations[0].Config, wantConfig) {
		t.Fatalf("Workstations[0].Config = %#v, want %#v", got.Workstations[0].Config, wantConfig)
	}
	if got.Workstations[0].Kind != "CRON" {
		t.Fatalf("Workstations[0].Kind = %q, want CRON", got.Workstations[0].Kind)
	}
}

func projectionNetAndRuntimeConfigWithConstraints() (*factoryruntime.Net, projectionRuntimeConfig) {
	net := representativeProjectionNet()
	net.Limits = factoryruntime.GlobalLimits{
		MaxTokenAge:    2 * time.Hour,
		MaxTotalVisits: 7,
	}
	net.Transitions["build"].InputArcs[0].Guard = &factoryruntime.PetriVisitCountGuard{
		TransitionID: "build",
		MaxVisits:    3,
	}
	return net, projectionRuntimeConfig{
		Workers: map[string]*interfaces.FactoryWorkerConfig{
			"builder": {
				Type:        interfaces.WorkerTypeModel,
				Concurrency: 2,
				Timeout:     "30m",
			},
		},
		Workstations: map[string]*interfaces.FactoryWorkstationConfig{
			"Build": {
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
}

func projectionRuntimeConstraints() []interfaces.FactoryConstraint {
	return []interfaces.FactoryConstraint{
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
}

func projectionRuntimeWorkstationConfig() map[string]string {
	return map[string]string{
		"configured_worker": "builder",
		"behavior":          string(interfaces.WorkstationKindCron),
		"output_schema":     "schema.json",
		"prompt_file":       "prompt.md",
		"type":              interfaces.WorkstationTypeModel,
		"worker":            "builder",
		"worktree":          "{{.worktree}}",
		"working_directory": "{{.working_directory}}",
	}
}

func TestProjectInitialStructure_RuntimeConfig_LimitsConstraintUsesRuntimeConfig(t *testing.T) {
	net := representativeProjectionNet()
	runtimeConfig := projectionRuntimeConfig{
		Workstations: map[string]*interfaces.FactoryWorkstationConfig{
			"Build": {
				Name:   "Build",
				Limits: interfaces.WorkstationLimits{MaxRetries: 2, MaxExecutionTime: "10m"},
			},
		},
	}

	got := runtimestate.ProjectInitialStructure(net, runtimeConfig)

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

func TestProjectInitialStructure_RuntimeConfig_UsesTransitionNameForWorkstationMetadata(t *testing.T) {
	net := representativeProjectionNet()
	runtimeConfig := projectionRuntimeConfig{
		Workstations: map[string]*interfaces.FactoryWorkstationConfig{
			"Build": {
				Name: "Build",
				Kind: interfaces.WorkstationKindCron,
			},
		},
	}

	got := runtimestate.ProjectInitialStructure(net, runtimeConfig)

	if got.Workstations[0].Kind != "CRON" {
		t.Fatalf("Workstations[0].Kind = %q, want CRON from authored transition name lookup", got.Workstations[0].Kind)
	}
}

func TestProjectInitialStructure_LogicalMoveCronProjectsKindTypeAndNoWorker(t *testing.T) {
	net := &factoryruntime.Net{
		Places: map[string]*factoryruntime.PetriPlace{
			"task:init": {ID: "task:init", TypeID: "task", State: "init"},
		},
		Transitions: map[string]*factoryruntime.PetriTransition{
			"scheduled-route": {
				ID:         "scheduled-route",
				Name:       "scheduled-route",
				OutputArcs: []factoryruntime.PetriArc{{PlaceID: "task:init"}},
			},
		},
		WorkTypes: map[string]*factoryruntime.WorkType{
			"task": {
				ID: "task",
				States: []factoryruntime.StateDefinition{{
					Value: "init", Category: factoryruntime.StateCategoryInitial,
				}},
			},
		},
	}

	runtimeConfig := projectionRuntimeConfig{
		Workstations: map[string]*interfaces.FactoryWorkstationConfig{
			"scheduled-route": {
				Name: "scheduled-route",
				Type: interfaces.WorkstationTypeLogical,
				Kind: interfaces.WorkstationKindCron,
				Cron: &interfaces.CronConfig{Schedule: "0 * * * *"},
			},
		},
	}
	got := runtimestate.ProjectInitialStructure(net, runtimeConfig)

	var cronWS *interfaces.FactoryWorkstation
	for i := range got.Workstations {
		if got.Workstations[i].ID == "scheduled-route" {
			cronWS = &got.Workstations[i]
			break
		}
	}
	if cronWS == nil {
		t.Fatalf("Workstations = %#v, want scheduled-route cron workstation", got.Workstations)
	}
	if cronWS.Kind != "CRON" {
		t.Fatalf("Kind = %q, want CRON", cronWS.Kind)
	}
	if cronWS.WorkerID != "" {
		t.Fatalf("WorkerID = %q, want empty for workerless cron logical move", cronWS.WorkerID)
	}
	if cronWS.Config["type"] != interfaces.WorkstationTypeLogical {
		t.Fatalf("type config = %q, want %q", cronWS.Config["type"], interfaces.WorkstationTypeLogical)
	}
	if cronWS.Config["behavior"] != string(interfaces.WorkstationKindCron) {
		t.Fatalf("behavior config = %q, want %q", cronWS.Config["behavior"], interfaces.WorkstationKindCron)
	}
	if cronWS.Config["worker"] != "" {
		t.Fatalf("worker config = %q, want empty", cronWS.Config["worker"])
	}
}

func TestProjectInitialStructure_RuntimeConfig_DoesNotUseTransitionIDFallback(t *testing.T) {
	net := representativeProjectionNet()
	runtimeConfig := projectionRuntimeConfig{
		Workstations: map[string]*interfaces.FactoryWorkstationConfig{
			"build": {
				Name: "build",
				Kind: interfaces.WorkstationKindCron,
			},
		},
	}

	got := runtimestate.ProjectInitialStructure(net, runtimeConfig)

	if got.Workstations[0].Kind != "" {
		t.Fatalf("Workstations[0].Kind = %q, want empty kind when only transition ID matches runtime workstation lookup", got.Workstations[0].Kind)
	}
	if got.Workstations[0].Config != nil {
		t.Fatalf("Workstations[0].Config = %#v, want nil when only transition ID matches runtime workstation lookup", got.Workstations[0].Config)
	}
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

type runtimeDefinitionOnlyFixture struct {
	Workers      map[string]*interfaces.FactoryWorkerConfig
	Workstations map[string]*interfaces.FactoryWorkstationConfig
}

var _ interfaces.RuntimeDefinitionLookup = runtimeDefinitionOnlyFixture{}

func (f runtimeDefinitionOnlyFixture) Worker(name string) (*interfaces.FactoryWorkerConfig, bool) {
	worker, ok := f.Workers[name]
	return worker, ok
}

func (f runtimeDefinitionOnlyFixture) Workstation(name string) (*interfaces.FactoryWorkstationConfig, bool) {
	workstation, ok := f.Workstations[name]
	return workstation, ok
}

func projectionNetWithOrderedNonSuccessRoutes() *factoryruntime.Net {
	return &factoryruntime.Net{
		ID: "projection-net-non-success-routes",
		Places: map[string]*factoryruntime.PetriPlace{
			"story:init":      {ID: "story:init", TypeID: "story", State: "init"},
			"story:retry":     {ID: "story:retry", TypeID: "story", State: "retry"},
			"story:triage":    {ID: "story:triage", TypeID: "story", State: "triage"},
			"story:complete":  {ID: "story:complete", TypeID: "story", State: "complete"},
			"story:failed":    {ID: "story:failed", TypeID: "story", State: "failed"},
			"story:abandoned": {ID: "story:abandoned", TypeID: "story", State: "abandoned"},
		},
		Transitions: map[string]*factoryruntime.PetriTransition{
			"execute": {
				ID:         "execute",
				Name:       "Execute",
				WorkerType: "executor",
				InputArcs:  []factoryruntime.PetriArc{{Name: "work", PlaceID: "story:init"}},
				OutputArcs: []factoryruntime.PetriArc{{PlaceID: "story:complete"}},
				ContinueArcs: []factoryruntime.PetriArc{
					{PlaceID: "story:retry"},
					{PlaceID: "story:init"},
				},
				RejectionArcs: []factoryruntime.PetriArc{
					{PlaceID: "story:triage"},
					{PlaceID: "story:init"},
				},
				FailureArcs: []factoryruntime.PetriArc{
					{PlaceID: "story:failed"},
					{PlaceID: "story:abandoned"},
				},
			},
		},
		WorkTypes: map[string]*factoryruntime.WorkType{
			"story": {
				ID:   "story",
				Name: "Story",
				States: []factoryruntime.StateDefinition{
					{Value: "init", Category: factoryruntime.StateCategoryInitial},
					{Value: "retry", Category: factoryruntime.StateCategoryProcessing},
					{Value: "triage", Category: factoryruntime.StateCategoryProcessing},
					{Value: "complete", Category: factoryruntime.StateCategoryTerminal},
					{Value: "failed", Category: factoryruntime.StateCategoryFailed},
					{Value: "abandoned", Category: factoryruntime.StateCategoryFailed},
				},
			},
		},
	}
}

func representativeProjectionNet() *factoryruntime.Net {
	story := &factoryruntime.WorkType{
		ID:   "story",
		Name: "Story",
		States: []factoryruntime.StateDefinition{
			{Value: "init", Category: factoryruntime.StateCategoryInitial},
			{Value: "review", Category: factoryruntime.StateCategoryProcessing},
			{Value: "done", Category: factoryruntime.StateCategoryTerminal},
			{Value: "failed", Category: factoryruntime.StateCategoryFailed},
		},
	}
	bug := &factoryruntime.WorkType{
		ID:   "bug",
		Name: "Bug",
		States: []factoryruntime.StateDefinition{
			{Value: "init", Category: factoryruntime.StateCategoryInitial},
			{Value: "closed", Category: factoryruntime.StateCategoryTerminal},
		},
	}

	return &factoryruntime.Net{
		ID: "projection-net",
		Places: map[string]*factoryruntime.PetriPlace{
			"story:review":  {ID: "story:review", TypeID: "story", State: "review"},
			"story:init":    {ID: "story:init", TypeID: "story", State: "init"},
			"story:failed":  {ID: "story:failed", TypeID: "story", State: "failed"},
			"story:done":    {ID: "story:done", TypeID: "story", State: "done"},
			"cpu:available": {ID: "cpu:available", TypeID: "cpu", State: "available"},
			"gpu:available": {ID: "gpu:available", TypeID: "gpu", State: "available"},
			"bug:init":      {ID: "bug:init", TypeID: "bug", State: "init"},
			"bug:closed":    {ID: "bug:closed", TypeID: "bug", State: "closed"},
		},
		Transitions: map[string]*factoryruntime.PetriTransition{
			"review": {
				ID:         "review",
				Name:       "Review",
				WorkerType: "reviewer",
				InputArcs: []factoryruntime.PetriArc{
					{Name: "work", PlaceID: "story:review"},
					{Name: "gpu", PlaceID: "gpu:available"},
				},
				OutputArcs: []factoryruntime.PetriArc{
					{PlaceID: "story:done"},
					{PlaceID: "gpu:available"},
				},
			},
			"build": {
				ID:         "build",
				Name:       "Build",
				WorkerType: "builder",
				InputArcs: []factoryruntime.PetriArc{
					{Name: "work", PlaceID: "story:init"},
					{Name: "cpu", PlaceID: "cpu:available"},
				},
				OutputArcs: []factoryruntime.PetriArc{
					{PlaceID: "story:review"},
					{PlaceID: "cpu:available"},
				},
				RejectionArcs: []factoryruntime.PetriArc{
					{PlaceID: "story:init"},
				},
				FailureArcs: []factoryruntime.PetriArc{
					{PlaceID: "story:failed"},
				},
			},
		},
		WorkTypes: map[string]*factoryruntime.WorkType{
			"story": story,
			"bug":   bug,
		},
		Resources: map[string]*factoryruntime.ResourceDef{
			"gpu": {ID: "gpu", Name: "GPU slots", Capacity: 2},
			"cpu": {ID: "cpu", Name: "CPU slots", Capacity: 4},
		},
	}
}
