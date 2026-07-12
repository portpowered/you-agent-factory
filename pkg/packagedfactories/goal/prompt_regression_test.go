package goal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/config/builtingoal"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

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
			if !strings.Contains(filepath.ToSlash(err.Error()), wantErrFragment) {
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
			if !strings.Contains(filepath.ToSlash(err.Error()), miswiredPromptFile) {
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

func packagedGoalRolePromptSourceByRole(role string) (PackagedGoalRolePromptSource, bool) {
	for _, source := range PackagedGoalRolePromptSources {
		if source.Role == role {
			return source, true
		}
	}
	return PackagedGoalRolePromptSource{}, false
}
