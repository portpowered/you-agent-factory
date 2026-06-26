package goal

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory/scheduler"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/config/builtingoal"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
)

var packagedGoalLifecycleStates = []string{
	"init",
	"plan",
	"execute",
	"check",
	"review",
	"structured-review",
	"complete",
	"blocked",
	"needs-human",
	"interrupted",
	"failed",
}

var packagedGoalProgressionWorkstations = []struct {
	name        string
	inputState  string
	outputState string
	publicType  string
	workerName  string
}{
	{name: PackagedPlanWorkstationName, inputState: "init", outputState: "plan", publicType: interfaces.WorkstationTypeAgent, workerName: "goal-planner"},
	{name: PackagedExecuteWorkstationName, inputState: "plan", outputState: "execute", publicType: interfaces.WorkstationTypeAgent, workerName: "goal-executor"},
	{name: PackagedCheckWorkstationName, inputState: "execute", outputState: "", publicType: interfaces.WorkstationTypeClassify, workerName: "goal-checker"},
}

var packagedGoalWorkerPublicTypes = map[string]string{
	"goal-planner":  interfaces.WorkerTypeAgent,
	"goal-executor": interfaces.WorkerTypeAgent,
	"goal-checker":  interfaces.WorkerTypeScript,
	"goal-reviewer": interfaces.WorkerTypeAgent,
}

var packagedGoalWorkstationPublicTypes = map[string]string{
	PackagedPlanWorkstationName:                  interfaces.WorkstationTypeAgent,
	PackagedExecuteWorkstationName:               interfaces.WorkstationTypeAgent,
	PackagedCheckWorkstationName:                 interfaces.WorkstationTypeClassify,
	PackagedReviewWorkstationName:                interfaces.WorkstationTypeClassify,
	PackagedStructuredReviewWorkstationName:      interfaces.WorkstationTypeAgent,
	PackagedLoopBreakerWorkstationName:           interfaces.WorkstationTypeLogical,
	PackagedStructuredLoopBreakerWorkstationName: interfaces.WorkstationTypeLogical,
}

func TestBuiltInFactoryJSON_LoadsRunnablePackagedGoalFactory(t *testing.T) {
	cfg, err := factoryconfig.FactoryConfigFromOpenAPIJSON(factoryconfig.BuiltInGoalFactoryJSON)
	if err != nil {
		t.Fatalf("ParseFactoryConfig: %v", err)
	}
	if cfg.Name != PackagedFactoryName {
		t.Fatalf("factory name = %q, want %s", cfg.Name, PackagedFactoryName)
	}
	if cfg.Project != PackagedFactoryProject {
		t.Fatalf("factory project = %q, want %s", cfg.Project, PackagedFactoryProject)
	}
	if len(cfg.WorkTypes) != 1 {
		t.Fatalf("workTypes = %#v, want one goal work type", cfg.WorkTypes)
	}
	workType := cfg.WorkTypes[0]
	if workType.Name != PackagedGoalWorkTypeName {
		t.Fatalf("work type name = %q, want %s", workType.Name, PackagedGoalWorkTypeName)
	}
	if len(workType.HandlingBehavior) != 1 || workType.HandlingBehavior[0] != interfaces.WorkTypeHandlingBehaviorDefault {
		t.Fatalf("handlingBehavior = %#v, want [DEFAULT]", workType.HandlingBehavior)
	}
	assertGoalLifecycleStates(t, workType.States)
	assertGoalProgressionTopology(t, cfg.Workstations, cfg.Workers)
	assertGoalCheckerContract(t, cfg.Workers)
	assertGoalCheckReviewModeRoutes(t, cfg.Workstations)
	assertGoalReviewRoutes(t, cfg.Workstations)
	assertGoalStructuredReviewRoutes(t, cfg.Workstations)
	assertGoalLoopBreaker(t, cfg.Workstations)
	assertGoalPublicPrimitiveVocabulary(t, cfg.Workers, cfg.Workstations)
}

func TestBuiltInGoalFactory_ExecuteStateSchedulesCheckReviewModeClassifier(t *testing.T) {
	cfg, err := factoryconfig.FactoryConfigFromOpenAPIJSON(factoryconfig.BuiltInGoalFactoryJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}

	mapper := &factoryconfig.ConfigMapper{}
	net, err := mapper.Map(context.Background(), cfg)
	if err != nil {
		t.Fatalf("ConfigMapper.Map: %v", err)
	}

	now := time.Date(2026, time.June, 20, 16, 0, 0, 0, time.UTC)
	executeToken := &interfaces.Token{
		ID:        "tok-execute",
		PlaceID:   "goal:execute",
		CreatedAt: now.Add(-time.Hour),
		EnteredAt: now.Add(-time.Minute),
		Color: interfaces.TokenColor{
			WorkID:     "work-goal-1",
			WorkTypeID: PackagedGoalWorkTypeName,
		},
		History: interfaces.TokenHistory{
			TotalVisits:         map[string]int{},
			ConsecutiveFailures: map[string]int{},
			PlaceVisits:         map[string]int{},
		},
	}
	marking := petri.MarkingSnapshot{
		Tokens:      map[string]*interfaces.Token{"tok-execute": executeToken},
		PlaceTokens: map[string][]string{"goal:execute": {"tok-execute"}},
	}
	snapshot := &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Marking:  marking,
		Topology: net,
	}

	runtimeLookup := goalRuntimeLookup{
		workstations: indexWorkstationsByName(cfg.Workstations),
		workers:      indexWorkerConfigsByName(cfg.Workers),
	}
	evaluator := scheduler.NewEnablementEvaluator(
		nil,
		scheduler.WithEnablementRuntimeConfig(runtimeLookup),
	)
	enabled := evaluator.FindEnabledTransitionsWithSnapshot(context.Background(), net, snapshot)

	var checkTransitionIDs []string
	for _, et := range enabled {
		transition := net.Transitions[et.TransitionID]
		if transition == nil {
			continue
		}
		if transition.Name == PackagedCheckWorkstationName {
			checkTransitionIDs = append(checkTransitionIDs, transition.Name)
		}
	}
	if len(checkTransitionIDs) != 1 {
		t.Fatalf("enabled check classifiers = %#v, want only %q", checkTransitionIDs, PackagedCheckWorkstationName)
	}

	sched := scheduler.NewWorkInQueueScheduler(1)
	decisions := sched.Select(enabled, snapshot)
	if len(decisions) != 1 {
		t.Fatalf("scheduler decisions = %#v, want one check classifier from goal:execute", decisions)
	}
	selected := net.Transitions[decisions[0].TransitionID]
	if selected == nil || selected.Name != PackagedCheckWorkstationName {
		t.Fatalf("selected transition = %#v, want %q", selected, PackagedCheckWorkstationName)
	}
}

func TestBuiltInGoalFactoryJSON_ExposesCurrentPublicPrimitiveVocabulary(t *testing.T) {
	cfg, err := factoryconfig.FactoryConfigFromOpenAPIJSON(factoryconfig.BuiltInGoalFactoryJSON)
	if err != nil {
		t.Fatalf("ParseFactoryConfig: %v", err)
	}
	assertGoalPublicPrimitiveVocabulary(t, cfg.Workers, cfg.Workstations)
}

func TestMaterializedPackagedGoalFactory_ExposesCanonicalWorkTypeAndLifecycleStates(t *testing.T) {
	factoryDir, err := factoryconfig.PersistNamedFactory(t.TempDir(), PackagedFactoryName, factoryconfig.BuiltInGoalFactoryJSON)
	if err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}

	loaded, err := factoryconfig.LoadRuntimeConfigFromFactoryDir(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir: %v", err)
	}

	cfg := loaded.FactoryConfig()
	if cfg.Project != PackagedFactoryProject {
		t.Fatalf("materialized project = %q, want %s", cfg.Project, PackagedFactoryProject)
	}
	if len(cfg.WorkTypes) != 1 {
		t.Fatalf("materialized workTypes = %#v, want one goal work type", cfg.WorkTypes)
	}
	workType := cfg.WorkTypes[0]
	if workType.Name != PackagedGoalWorkTypeName {
		t.Fatalf("materialized work type name = %q, want %s", workType.Name, PackagedGoalWorkTypeName)
	}
	if len(workType.HandlingBehavior) != 1 || workType.HandlingBehavior[0] != interfaces.WorkTypeHandlingBehaviorDefault {
		t.Fatalf("materialized handlingBehavior = %#v, want [DEFAULT]", workType.HandlingBehavior)
	}
	assertGoalLifecycleStates(t, workType.States)
	assertGoalProgressionTopology(t, cfg.Workstations, cfg.Workers)
	assertGoalCheckerContract(t, cfg.Workers)
	assertGoalCheckReviewModeRoutes(t, cfg.Workstations)
	assertGoalReviewRoutes(t, cfg.Workstations)
	assertGoalStructuredReviewRoutes(t, cfg.Workstations)
	assertGoalLoopBreaker(t, cfg.Workstations)
	assertGoalPublicPrimitiveVocabulary(t, cfg.Workers, cfg.Workstations)
}

func TestMaterializedPackagedGoalFactory_ExposesCurrentPublicPrimitiveVocabulary(t *testing.T) {
	factoryDir, err := factoryconfig.PersistNamedFactory(t.TempDir(), PackagedFactoryName, factoryconfig.BuiltInGoalFactoryJSON)
	if err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}

	loaded, err := factoryconfig.LoadRuntimeConfigFromFactoryDir(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir: %v", err)
	}

	assertGoalPublicPrimitiveVocabulary(t, loaded.FactoryConfig().Workers, loaded.FactoryConfig().Workstations)
}

var packagedGoalLegacyPromptAliases = []string{"MODEL_WORKER", "MODEL_RUN"}

var packagedGoalRolePromptRegressionExpectations = map[string]struct {
	mustContain    []string
	mustNotContain []string
}{
	"planner": {
		mustContain: []string{
			"AGENT_RUN",
			"AGENT_WORKER",
			"bounded plan",
			"## Goal",
			"## Plan",
			"## Acceptance checks",
			"open-ended discussion",
		},
		mustNotContain: packagedGoalLegacyPromptAliases,
	},
	"executor": {
		mustContain: []string{
			"AGENT_RUN",
			"AGENT_WORKER",
			"bounded execution result",
			"## Completed work",
			"## Blockers",
			"## Follow-up for review",
			"open-ended discussion",
		},
		mustNotContain: packagedGoalLegacyPromptAliases,
	},
	"checker": {
		mustContain: []string{
			"CLASSIFIER_WORKSTATION",
			"SCRIPT_WORKER",
			"plain",
			"structured",
			"stdout",
			"open-ended discussion",
		},
		mustNotContain: packagedGoalLegacyPromptAliases,
	},
	"reviewer": {
		mustContain: []string{
			"AGENT_WORKER",
			"reviewable disposition",
			"## Disposition",
			"accepted",
			"needs_changes",
			"## Findings",
			"## Outcome",
			"## Verification",
			"## Follow-up",
			"open-ended discussion",
		},
		mustNotContain: packagedGoalLegacyPromptAliases,
	},
	"summarizer": {
		mustContain: []string{
			"AGENT_RUN",
			"AGENT_WORKER",
			"SCRIPT_RUN",
			"bounded final summary",
			"## Disposition",
			"## Findings",
			"## Outcome",
			"## What was done",
			"## Verification",
			"## Follow-up",
			"open-ended discussion",
		},
		mustNotContain: packagedGoalLegacyPromptAliases,
	},
}

var packagedGoalBoundedWorkerRolePromptExpectations = []struct {
	role           string
	mustContain    []string
	mustNotContain []string
}{
	{
		role: "planner",
		mustContain: []string{
			"AGENT_RUN",
			"AGENT_WORKER",
			"bounded plan",
			"## Goal",
			"## Plan",
			"## Acceptance checks",
			"open-ended discussion",
		},
		mustNotContain: []string{"MODEL_WORKER", "MODEL_RUN"},
	},
	{
		role: "executor",
		mustContain: []string{
			"AGENT_RUN",
			"AGENT_WORKER",
			"bounded execution result",
			"## Completed work",
			"## Blockers",
			"## Follow-up for review",
			"open-ended discussion",
		},
		mustNotContain: []string{"MODEL_WORKER", "MODEL_RUN"},
	},
	{
		role: "checker",
		mustContain: []string{
			"CLASSIFIER_WORKSTATION",
			"SCRIPT_WORKER",
			"plain",
			"structured",
			"stdout",
			"open-ended discussion",
		},
		mustNotContain: []string{"MODEL_WORKER", "MODEL_RUN"},
	},
	{
		role: "reviewer",
		mustContain: []string{
			"AGENT_WORKER",
			"reviewable disposition",
			"## Disposition",
			"accepted",
			"needs_changes",
			"## Findings",
			"## Outcome",
			"## Verification",
			"## Follow-up",
			"open-ended discussion",
		},
		mustNotContain: []string{"MODEL_WORKER", "MODEL_RUN"},
	},
}

func TestMaterializedPackagedGoalFactory_AuthorBoundedSummarizerPrompt(t *testing.T) {
	factoryDir, err := factoryconfig.PersistNamedFactory(t.TempDir(), PackagedFactoryName, factoryconfig.BuiltInGoalFactoryJSON)
	if err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}

	source, ok := packagedGoalRolePromptSourceByRole("summarizer")
	if !ok {
		t.Fatal("missing packaged role prompt source for summarizer")
	}

	promptPath := packagedGoalMaterializedPromptPath(factoryDir, source)
	promptBytes, err := os.ReadFile(promptPath)
	if err != nil {
		t.Fatalf("summarizer prompt file %s: %v", promptPath, err)
	}
	prompt := strings.TrimSpace(string(promptBytes))
	if prompt == "" {
		t.Fatal("summarizer prompt is empty")
	}

	mustContain := []string{
		"AGENT_RUN",
		"AGENT_WORKER",
		"SCRIPT_RUN",
		"bounded final summary",
		"## Disposition",
		"## Findings",
		"## Outcome",
		"## What was done",
		"## Verification",
		"## Follow-up",
		"open-ended discussion",
	}
	for _, marker := range mustContain {
		if !strings.Contains(prompt, marker) {
			t.Fatalf("summarizer prompt missing %q:\n%s", marker, prompt)
		}
	}
	for _, marker := range []string{"MODEL_WORKER", "MODEL_RUN"} {
		if strings.Contains(prompt, marker) {
			t.Fatalf("summarizer prompt must not contain legacy marker %q:\n%s", marker, prompt)
		}
	}
}

func TestMaterializedPackagedGoalFactory_AuthorBoundedWorkerRolePrompts(t *testing.T) {
	factoryDir, err := factoryconfig.PersistNamedFactory(t.TempDir(), PackagedFactoryName, factoryconfig.BuiltInGoalFactoryJSON)
	if err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}

	loaded, err := factoryconfig.LoadRuntimeConfigFromFactoryDir(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir: %v", err)
	}

	for _, expectation := range packagedGoalBoundedWorkerRolePromptExpectations {
		source, ok := packagedGoalRolePromptSourceByRole(expectation.role)
		if !ok {
			t.Fatalf("missing packaged role prompt source for role %q", expectation.role)
		}
		prompt, err := loadedPackagedGoalRolePrompt(loaded, source)
		if err != nil {
			t.Fatalf("load role %q prompt: %v", expectation.role, err)
		}
		if prompt == "" {
			t.Fatalf("role %q prompt is empty", expectation.role)
		}
		for _, marker := range expectation.mustContain {
			if !strings.Contains(prompt, marker) {
				t.Fatalf("role %q prompt missing %q:\n%s", expectation.role, marker, prompt)
			}
		}
		for _, marker := range expectation.mustNotContain {
			if strings.Contains(prompt, marker) {
				t.Fatalf("role %q prompt must not contain legacy marker %q:\n%s", expectation.role, marker, prompt)
			}
		}
	}
}

func TestMaterializedPackagedGoalFactory_ResolvesSplitRolePromptFiles(t *testing.T) {
	factoryDir, loaded := materializeAndLoadPackagedGoalFactory(t)
	assertPackagedGoalSplitRolePromptRegression(t, factoryDir, loaded)
}

func TestMaterializedPackagedGoalFactory_GuardsSplitRolePromptResolutionAndVocabulary(t *testing.T) {
	factoryDir, loaded := materializeAndLoadPackagedGoalFactory(t)
	assertPackagedGoalSplitRolePromptRegression(t, factoryDir, loaded)
}

func TestResolveNamedFactoryAcrossRoots_BuiltInGoalGuardsSplitRolePromptRegression(t *testing.T) {
	projectRoot := t.TempDir()
	globalRoot := t.TempDir()

	resolution, err := factoryconfig.ResolveNamedFactoryAcrossRoots(projectRoot, globalRoot, PackagedFactoryName)
	if err != nil {
		t.Fatalf("ResolveNamedFactoryAcrossRoots(builtin goal): %v", err)
	}
	if resolution.Source != factoryconfig.NamedFactoryResolutionSourceBuiltin {
		t.Fatalf("resolution source = %q, want builtin materialization", resolution.Source)
	}

	loaded, err := factoryconfig.LoadRuntimeConfigFromFactoryDir(resolution.FactoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir(materialized builtin goal): %v", err)
	}
	assertPackagedGoalSplitRolePromptRegression(t, resolution.FactoryDir, loaded)
}

func TestMaterializedPackagedGoalFactory_AuthoredPromptSourcesMatchMaterializedFiles(t *testing.T) {
	factoryDir, loaded := materializeAndLoadPackagedGoalFactory(t)

	for _, source := range PackagedGoalRolePromptSources {
		authoredPrompt, ok := builtingoal.AuthoredRolePrompt(source.Role)
		if !ok {
			t.Fatalf("missing authored prompt source for role %q", source.Role)
		}

		materializedPrompt, err := loadPackagedGoalRolePrompt(factoryDir, source)
		if err != nil {
			t.Fatalf("role %q materialized prompt load: %v", source.Role, err)
		}
		if materializedPrompt != authoredPrompt {
			t.Fatalf("role %q materialized prompt does not match authored source file", source.Role)
		}

		loadedPrompt, err := loadedPackagedGoalRolePrompt(loaded, source)
		if err != nil {
			t.Fatalf("role %q loaded prompt: %v", source.Role, err)
		}
		if loadedPrompt != materializedPrompt {
			t.Fatalf("loaded role %q prompt does not match authored source", source.Role)
		}
	}
}

func TestMaterializedPackagedGoalFactory_SplitRolePromptRegressionFailsWhenPromptMissing(t *testing.T) {
	for _, source := range PackagedGoalRolePromptSources {
		source := source
		t.Run(source.Role, func(t *testing.T) {
			factoryDir, err := factoryconfig.PersistNamedFactory(t.TempDir(), PackagedFactoryName, factoryconfig.BuiltInGoalFactoryJSON)
			if err != nil {
				t.Fatalf("PersistNamedFactory: %v", err)
			}
			promptPath := packagedGoalMaterializedPromptPath(factoryDir, source)
			if err := os.Remove(promptPath); err != nil {
				t.Fatalf("remove role %q prompt file %s: %v", source.Role, promptPath, err)
			}

			_, err = factoryconfig.LoadRuntimeConfigFromFactoryDir(factoryDir, nil)
			if err == nil {
				t.Fatalf("expected packaged goal load to fail when role %q prompt source is missing", source.Role)
			}
			wantErrFragment := source.PromptFile
			if source.SourceKind == PackagedGoalRolePromptSourceKindWorkerBody {
				wantErrFragment = interfaces.FactoryAgentsFileName
			}
			if !strings.Contains(err.Error(), wantErrFragment) {
				t.Fatalf("missing prompt error = %v, want reference to %q", err, wantErrFragment)
			}
		})
	}
}

func TestMaterializedPackagedGoalFactory_SplitRolePromptRegressionFailsWhenPromptMiswired(t *testing.T) {
	for _, source := range PackagedGoalRolePromptSources {
		if source.SourceKind != PackagedGoalRolePromptSourceKindWorkstationPromptFile {
			continue
		}
		source := source
		t.Run(source.Role, func(t *testing.T) {
			factoryDir, err := factoryconfig.PersistNamedFactory(t.TempDir(), PackagedFactoryName, factoryconfig.BuiltInGoalFactoryJSON)
			if err != nil {
				t.Fatalf("PersistNamedFactory: %v", err)
			}

			configPath := filepath.Join(factoryDir, interfaces.FactoryConfigFile)
			configBytes, err := os.ReadFile(configPath)
			if err != nil {
				t.Fatalf("ReadFile(factory config): %v", err)
			}
			miswiredPromptFile := "prompts/missing-" + source.Role + ".md"
			tamperedConfig := strings.Replace(
				string(configBytes),
				`"promptFile": "`+source.PromptFile+`"`,
				`"promptFile": "`+miswiredPromptFile+`"`,
				1,
			)
			if tamperedConfig == string(configBytes) {
				t.Fatalf("failed to tamper promptFile for role %q in factory config", source.Role)
			}
			if err := os.WriteFile(configPath, []byte(tamperedConfig), 0o644); err != nil {
				t.Fatalf("WriteFile(factory config): %v", err)
			}

			_, err = factoryconfig.LoadRuntimeConfigFromFactoryDir(factoryDir, nil)
			if err == nil {
				t.Fatalf("expected packaged goal load to fail when role %q promptFile is miswired", source.Role)
			}
			if !strings.Contains(err.Error(), miswiredPromptFile) {
				t.Fatalf("miswired prompt error = %v, want reference to %q", err, miswiredPromptFile)
			}
		})
	}
}

func materializeAndLoadPackagedGoalFactory(t *testing.T) (string, *factoryconfig.LoadedFactoryConfig) {
	t.Helper()

	factoryDir, err := factoryconfig.PersistNamedFactory(t.TempDir(), PackagedFactoryName, factoryconfig.BuiltInGoalFactoryJSON)
	if err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}

	loaded, err := factoryconfig.LoadRuntimeConfigFromFactoryDir(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir: %v", err)
	}
	return factoryDir, loaded
}

func assertPackagedGoalSplitRolePromptRegression(t *testing.T, factoryDir string, loaded *factoryconfig.LoadedFactoryConfig) {
	t.Helper()

	if len(PackagedGoalRolePromptSources) != len(packagedGoalRolePromptRegressionExpectations) {
		t.Fatalf("role prompt sources = %d, regression expectations = %d, want matching coverage for every goal role",
			len(PackagedGoalRolePromptSources), len(packagedGoalRolePromptRegressionExpectations))
	}

	for _, source := range PackagedGoalRolePromptSources {
		expectation, ok := packagedGoalRolePromptRegressionExpectations[source.Role]
		if !ok {
			t.Fatalf("missing regression expectations for role %q", source.Role)
		}

		promptOnDisk := assertPackagedGoalMaterializedPromptMatchesAuthoredSource(t, factoryDir, source)
		assertPackagedGoalLoadedPromptSurface(t, loaded, source, promptOnDisk)

		loadedPrompt, err := loadPackagedGoalRolePrompt(factoryDir, source)
		if err != nil {
			t.Fatalf("role %q prompt resolution: %v", source.Role, err)
		}
		if loadedPrompt != promptOnDisk {
			t.Fatalf("role %q resolved prompt = %q, want %q", source.Role, loadedPrompt, promptOnDisk)
		}

		assertPackagedGoalRolePromptVocabulary(t, source.Role, loadedPrompt, expectation.mustContain, expectation.mustNotContain)
	}
}

func assertPackagedGoalMaterializedPromptMatchesAuthoredSource(t *testing.T, factoryDir string, source PackagedGoalRolePromptSource) string {
	t.Helper()

	promptPath := packagedGoalMaterializedPromptPath(factoryDir, source)
	promptBytes, err := os.ReadFile(promptPath)
	if err != nil {
		t.Fatalf("role %q prompt file %s: %v", source.Role, promptPath, err)
	}
	promptOnDisk := strings.TrimSpace(string(promptBytes))
	if promptOnDisk == "" {
		t.Fatalf("role %q prompt file %s is empty", source.Role, promptPath)
	}

	authoredPrompt, ok := builtingoal.AuthoredRolePrompt(source.Role)
	if !ok {
		t.Fatalf("missing authored prompt source for role %q", source.Role)
	}
	if promptOnDisk != authoredPrompt {
		t.Fatalf("role %q materialized prompt does not match authored source file", source.Role)
	}

	return promptOnDisk
}

func assertPackagedGoalLoadedPromptSurface(t *testing.T, loaded *factoryconfig.LoadedFactoryConfig, source PackagedGoalRolePromptSource, wantPrompt string) {
	t.Helper()

	switch source.SourceKind {
	case PackagedGoalRolePromptSourceKindWorkstationPromptFile:
		assertPackagedGoalLoadedWorkstationPrompt(t, loaded, source, wantPrompt)
	case PackagedGoalRolePromptSourceKindWorkerBody:
		assertPackagedGoalLoadedWorkerPrompt(t, loaded, source, wantPrompt)
	default:
		t.Fatalf("unsupported prompt source kind %q for role %q", source.SourceKind, source.Role)
	}
}

func assertPackagedGoalLoadedWorkstationPrompt(t *testing.T, loaded *factoryconfig.LoadedFactoryConfig, source PackagedGoalRolePromptSource, wantPrompt string) {
	t.Helper()

	workstation, ok := loaded.Workstation(source.WorkstationName)
	if !ok {
		t.Fatalf("missing workstation %q for role %q", source.WorkstationName, source.Role)
	}
	if workstation.PromptFile != source.PromptFile {
		t.Fatalf("workstation %q promptFile = %q, want %q", source.WorkstationName, workstation.PromptFile, source.PromptFile)
	}
	if strings.TrimSpace(workstation.PromptTemplate) != wantPrompt {
		t.Fatalf("workstation %q prompt template = %q, want loaded split file %q", source.WorkstationName, workstation.PromptTemplate, wantPrompt)
	}
}

func assertPackagedGoalLoadedWorkerPrompt(t *testing.T, loaded *factoryconfig.LoadedFactoryConfig, source PackagedGoalRolePromptSource, wantPrompt string) {
	t.Helper()

	workerBody, ok := loadedPackagedGoalWorkerBody(loaded, source.WorkerName)
	if !ok {
		t.Fatalf("missing worker %q for role %q", source.WorkerName, source.Role)
	}
	if workerBody != wantPrompt {
		t.Fatalf("worker %q body = %q, want authored role prompt %q", source.WorkerName, workerBody, wantPrompt)
	}
}

func packagedGoalMaterializedPromptPath(factoryDir string, source PackagedGoalRolePromptSource) string {
	if source.SourceKind == PackagedGoalRolePromptSourceKindWorkerBody {
		return filepath.Join(factoryDir, interfaces.WorkersDir, source.WorkerName, interfaces.FactoryAgentsFileName)
	}
	return filepath.Join(factoryDir, interfaces.WorkstationsDir, source.WorkstationName, source.PromptFile)
}

func loadPackagedGoalRolePrompt(factoryDir string, source PackagedGoalRolePromptSource) (string, error) {
	promptPath := packagedGoalMaterializedPromptPath(factoryDir, source)
	data, err := os.ReadFile(promptPath)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func loadedPackagedGoalRolePrompt(loaded *factoryconfig.LoadedFactoryConfig, source PackagedGoalRolePromptSource) (string, error) {
	switch source.SourceKind {
	case PackagedGoalRolePromptSourceKindWorkerBody:
		prompt, ok := loadedPackagedGoalWorkerBody(loaded, source.WorkerName)
		if !ok {
			return "", os.ErrNotExist
		}
		return prompt, nil
	default:
		workstation, ok := loaded.Workstation(source.WorkstationName)
		if !ok {
			return "", os.ErrNotExist
		}
		return strings.TrimSpace(workstation.PromptTemplate), nil
	}
}

func loadedPackagedGoalWorkerBody(loaded *factoryconfig.LoadedFactoryConfig, workerName string) (string, bool) {
	for _, worker := range loaded.FactoryConfig().Workers {
		if worker.Name == workerName {
			return strings.TrimSpace(worker.Body), true
		}
	}
	return "", false
}

func assertPackagedGoalRolePromptVocabulary(t *testing.T, role, prompt string, mustContain, mustNotContain []string) {
	t.Helper()

	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		t.Fatalf("role %q prompt is empty", role)
	}
	for _, marker := range mustContain {
		if !strings.Contains(prompt, marker) {
			t.Fatalf("role %q prompt missing %q:\n%s", role, marker, prompt)
		}
	}
	for _, marker := range mustNotContain {
		if strings.Contains(prompt, marker) {
			t.Fatalf("role %q prompt must not contain legacy marker %q:\n%s", role, marker, prompt)
		}
	}
}

func assertGoalPublicPrimitiveVocabulary(t *testing.T, workers []interfaces.WorkerConfig, workstations []interfaces.FactoryWorkstationConfig) {
	t.Helper()

	workerTypes := indexWorkerTypesByName(workers)
	for name, wantPublic := range packagedGoalWorkerPublicTypes {
		rawType, ok := workerTypes[name]
		if !ok {
			t.Fatalf("missing worker %q", name)
		}
		if rawType == interfaces.WorkerTypeModel {
			t.Fatalf("worker %q uses legacy alias %q", name, interfaces.WorkerTypeModel)
		}
		publicType := interfaces.PublicWorkerTypeFromInternalRuntime(rawType)
		if publicType != wantPublic {
			t.Fatalf("worker %q public type = %q, want %q", name, publicType, wantPublic)
		}
	}

	byName := indexWorkstationsByName(workstations)
	for name, wantPublic := range packagedGoalWorkstationPublicTypes {
		workstation, ok := byName[name]
		if !ok {
			t.Fatalf("missing workstation %q", name)
		}
		workerType := workerTypes[workstation.WorkerTypeName]
		publicType := interfaces.PublicWorkstationTypeFromInternalRuntime(workstation.Type, workerType, workstation.Kind)
		if publicType == interfaces.WorkstationTypeModel {
			t.Fatalf("workstation %q projects to legacy alias %q", name, interfaces.WorkstationTypeModel)
		}
		if publicType != wantPublic {
			t.Fatalf("workstation %q public type = %q, want %q", name, publicType, wantPublic)
		}
	}
}

func assertGoalLifecycleStates(t *testing.T, states []interfaces.StateConfig) {
	t.Helper()

	got := make([]string, 0, len(states))
	for _, state := range states {
		got = append(got, state.Name)
	}
	slices.Sort(got)
	want := append([]string(nil), packagedGoalLifecycleStates...)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("goal lifecycle states = %#v, want %#v", got, want)
	}
}

func assertGoalProgressionTopology(t *testing.T, workstations []interfaces.FactoryWorkstationConfig, workers []interfaces.WorkerConfig) {
	t.Helper()

	byName := indexWorkstationsByName(workstations)
	workerTypes := indexWorkerTypesByName(workers)
	for _, want := range packagedGoalProgressionWorkstations {
		workstation, ok := byName[want.name]
		if !ok {
			t.Fatalf("missing progression workstation %q", want.name)
		}
		workerType := workerTypes[workstation.WorkerTypeName]
		publicType := interfaces.PublicWorkstationTypeFromInternalRuntime(workstation.Type, workerType, workstation.Kind)
		if publicType != want.publicType {
			t.Fatalf("workstation %q public type = %q, want %q", want.name, publicType, want.publicType)
		}
		assertSingleGoalRoute(t, want.name, workstation.Inputs, want.inputState)
		if want.outputState != "" {
			assertSingleGoalRoute(t, want.name, workstation.Outputs, want.outputState)
		}
	}
}

func assertGoalCheckerContract(t *testing.T, workers []interfaces.WorkerConfig) {
	t.Helper()

	checker, ok := indexWorkerConfigsByName(workers)["goal-checker"]
	if !ok {
		t.Fatal("missing goal-checker worker")
	}
	if checker.Command != "sh" {
		t.Fatalf("goal-checker command = %q, want %q", checker.Command, "sh")
	}
	if len(checker.Args) != 2 {
		t.Fatalf("goal-checker args = %#v, want shell wrapper", checker.Args)
	}
	if checker.Args[0] != "-c" {
		t.Fatalf("goal-checker args[0] = %q, want %q", checker.Args[0], "-c")
	}
	wantScript := "make test >/dev/null && printf '%s' \"${" + PackagedCheckReviewModeEnvVar + ":-plain}\""
	if checker.Args[1] != wantScript {
		t.Fatalf("goal-checker shell wrapper = %q, want %q", checker.Args[1], wantScript)
	}
}

func assertGoalReviewRoutes(t *testing.T, workstations []interfaces.FactoryWorkstationConfig) {
	t.Helper()

	review, ok := indexWorkstationsByName(workstations)[PackagedReviewWorkstationName]
	if !ok {
		t.Fatal("missing review workstation")
	}
	if review.Type != interfaces.WorkstationTypeClassify {
		t.Fatalf("review workstation type = %q, want %q", review.Type, interfaces.WorkstationTypeClassify)
	}

	wantRoutes := map[string]string{
		"accepted":      "complete",
		"needs_changes": "plan",
		"tests_failed":  "plan",
		"needs_human":   "needs-human",
		"blocked":       "blocked",
		"interrupted":   "interrupted",
		"failed":        "failed",
	}
	gotRoutes := make(map[string]string, len(review.ClassificationRoutes))
	for _, route := range review.ClassificationRoutes {
		if len(route.Outputs) != 1 {
			t.Fatalf("review route %q outputs = %#v, want one goal output", route.Label, route.Outputs)
		}
		gotRoutes[route.Label] = route.Outputs[0].StateName
	}
	for label, wantState := range wantRoutes {
		gotState, ok := gotRoutes[label]
		if !ok {
			t.Fatalf("review route missing label %q", label)
		}
		if gotState != wantState {
			t.Fatalf("review route %q state = %q, want %q", label, gotState, wantState)
		}
	}

	execute, ok := indexWorkstationsByName(workstations)[PackagedExecuteWorkstationName]
	if !ok {
		t.Fatal("missing execute workstation for retry path")
	}
	assertSingleGoalRoute(t, execute.Name, execute.Inputs, "plan")
}

func assertGoalStructuredReviewRoutes(t *testing.T, workstations []interfaces.FactoryWorkstationConfig) {
	t.Helper()

	byName := indexWorkstationsByName(workstations)
	structuredReview, ok := byName[PackagedStructuredReviewWorkstationName]
	if !ok {
		t.Fatal("missing structured review workstation")
	}
	if structuredReview.Type != interfaces.WorkstationTypeAgent && structuredReview.Type != interfaces.WorkstationTypeModel {
		t.Fatalf("structured review workstation type = %q, want agent runtime type", structuredReview.Type)
	}
	if structuredReview.OutcomeFormat != DecisionEnvelopeOutcomeFormat {
		t.Fatalf("structured review outcomeFormat = %q, want %q", structuredReview.OutcomeFormat, DecisionEnvelopeOutcomeFormat)
	}
	if !UsesGoalRoutingDecisionEnvelope(&structuredReview) {
		t.Fatal("structured review workstation should use goal routing decision envelope")
	}
	assertSingleGoalRoute(t, structuredReview.Name, structuredReview.Inputs, PackagedStructuredReviewStateName)

	wantRoutes := map[string]string{
		"accepted":      "complete",
		"needs_changes": "plan",
		"tests_failed":  "plan",
		"needs_human":   "needs-human",
		"blocked":       "blocked",
		"interrupted":   "interrupted",
		"failed":        "failed",
	}
	gotRoutes := make(map[string]string, len(structuredReview.ClassificationRoutes))
	for _, route := range structuredReview.ClassificationRoutes {
		if len(route.Outputs) != 1 {
			t.Fatalf("structured review route %q outputs = %#v, want one goal output", route.Label, route.Outputs)
		}
		gotRoutes[route.Label] = route.Outputs[0].StateName
	}
	for label, wantState := range wantRoutes {
		gotState, ok := gotRoutes[label]
		if !ok {
			t.Fatalf("structured review route missing label %q", label)
		}
		if gotState != wantState {
			t.Fatalf("structured review route %q state = %q, want %q", label, gotState, wantState)
		}
	}

	if _, ok := byName[PackagedAdvanceStructuredReviewWorkstationName]; ok {
		t.Fatalf("built-in factory should not declare %q; check-goal routes review modes directly", PackagedAdvanceStructuredReviewWorkstationName)
	}
}

func assertGoalCheckReviewModeRoutes(t *testing.T, workstations []interfaces.FactoryWorkstationConfig) {
	t.Helper()

	check, ok := indexWorkstationsByName(workstations)[PackagedCheckWorkstationName]
	if !ok {
		t.Fatal("missing check workstation")
	}
	if check.Type != interfaces.WorkstationTypeClassify {
		t.Fatalf("check workstation type = %q, want %q", check.Type, interfaces.WorkstationTypeClassify)
	}
	assertSingleGoalRoute(t, check.Name, check.Inputs, "execute")

	wantRoutes := map[string]string{
		PackagedReviewModePlainLabel:      "review",
		PackagedReviewModeStructuredLabel: PackagedStructuredReviewStateName,
	}
	gotRoutes := make(map[string]string, len(check.ClassificationRoutes))
	for _, route := range check.ClassificationRoutes {
		if len(route.Outputs) != 1 {
			t.Fatalf("check route %q outputs = %#v, want one goal output", route.Label, route.Outputs)
		}
		gotRoutes[route.Label] = route.Outputs[0].StateName
	}
	for label, wantState := range wantRoutes {
		gotState, ok := gotRoutes[label]
		if !ok {
			t.Fatalf("check route missing label %q", label)
		}
		if gotState != wantState {
			t.Fatalf("check route %q state = %q, want %q", label, gotState, wantState)
		}
	}

	workersByName := indexWorkstationsByName(workstations)
	if got := workersByName[PackagedCheckWorkstationName].Env[PackagedCheckReviewModeEnvVar]; got != "" {
		t.Fatalf("check workstation env %q = %q, want empty default so checker falls back to plain", PackagedCheckReviewModeEnvVar, got)
	}
}

func assertGoalLoopBreaker(t *testing.T, workstations []interfaces.FactoryWorkstationConfig) {
	t.Helper()

	byName := indexWorkstationsByName(workstations)
	assertGoalLoopBreakerWorkstation(t, byName, PackagedLoopBreakerWorkstationName, PackagedReviewWorkstationName)
	assertGoalLoopBreakerWorkstation(t, byName, PackagedStructuredLoopBreakerWorkstationName, PackagedStructuredReviewWorkstationName)
}

func assertGoalLoopBreakerWorkstation(
	t *testing.T,
	workstations map[string]interfaces.FactoryWorkstationConfig,
	workstationName string,
	watchedWorkstation string,
) {
	t.Helper()

	loopBreaker, ok := workstations[workstationName]
	if !ok {
		t.Fatalf("missing goal loop breaker workstation %q", workstationName)
	}
	if loopBreaker.Type != interfaces.WorkstationTypeLogical {
		t.Fatalf("loop breaker %q type = %q, want %q", workstationName, loopBreaker.Type, interfaces.WorkstationTypeLogical)
	}
	if len(loopBreaker.Guards) != 1 {
		t.Fatalf("loop breaker %q guards = %#v, want one visit_count guard", workstationName, loopBreaker.Guards)
	}
	guard := loopBreaker.Guards[0]
	if guard.Type != interfaces.GuardTypeVisitCount {
		t.Fatalf("loop breaker %q guard type = %q, want %q", workstationName, guard.Type, interfaces.GuardTypeVisitCount)
	}
	if guard.Workstation != watchedWorkstation {
		t.Fatalf("loop breaker %q guard workstation = %q, want %q", workstationName, guard.Workstation, watchedWorkstation)
	}
	if guard.MaxVisits != 5 {
		t.Fatalf("loop breaker %q guard maxVisits = %d, want 5", workstationName, guard.MaxVisits)
	}
	assertSingleGoalRoute(t, loopBreaker.Name, loopBreaker.Inputs, "plan")
	assertSingleGoalRoute(t, loopBreaker.Name, loopBreaker.Outputs, "failed")
}

func assertSingleGoalRoute(t *testing.T, workstationName string, routes []interfaces.IOConfig, wantState string) {
	t.Helper()

	if len(routes) != 1 {
		t.Fatalf("workstation %q routes = %#v, want one goal route", workstationName, routes)
	}
	if routes[0].WorkTypeName != PackagedGoalWorkTypeName {
		t.Fatalf("workstation %q work type = %q, want %s", workstationName, routes[0].WorkTypeName, PackagedGoalWorkTypeName)
	}
	if routes[0].StateName != wantState {
		t.Fatalf("workstation %q state = %q, want %q", workstationName, routes[0].StateName, wantState)
	}
}

func indexWorkerConfigsByName(workers []interfaces.WorkerConfig) map[string]interfaces.WorkerConfig {
	byName := make(map[string]interfaces.WorkerConfig, len(workers))
	for _, worker := range workers {
		byName[worker.Name] = worker
	}
	return byName
}

func indexWorkerTypesByName(workers []interfaces.WorkerConfig) map[string]string {
	byName := make(map[string]string, len(workers))
	for _, worker := range workers {
		byName[worker.Name] = worker.Type
	}
	return byName
}

func indexWorkstationsByName(workstations []interfaces.FactoryWorkstationConfig) map[string]interfaces.FactoryWorkstationConfig {
	byName := make(map[string]interfaces.FactoryWorkstationConfig, len(workstations))
	for _, workstation := range workstations {
		byName[workstation.Name] = workstation
	}
	return byName
}

type goalRuntimeLookup struct {
	workstations map[string]interfaces.FactoryWorkstationConfig
	workers      map[string]interfaces.WorkerConfig
}

func (l goalRuntimeLookup) Workstation(name string) (*interfaces.FactoryWorkstationConfig, bool) {
	workstation, ok := l.workstations[name]
	if !ok {
		return nil, false
	}
	return &workstation, true
}

func (l goalRuntimeLookup) Worker(name string) (*interfaces.WorkerConfig, bool) {
	worker, ok := l.workers[name]
	if !ok {
		return nil, false
	}
	return &worker, true
}

func packagedGoalRolePromptSourceByRole(role string) (PackagedGoalRolePromptSource, bool) {
	for _, source := range PackagedGoalRolePromptSources {
		if source.Role == role {
			return source, true
		}
	}
	return PackagedGoalRolePromptSource{}, false
}
