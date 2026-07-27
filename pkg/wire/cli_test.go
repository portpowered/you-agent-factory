package wire

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	initializerapplication "github.com/portpowered/infinite-you/pkg/initializer/application"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/transports/cli/completionprojection"
	factorycli "github.com/portpowered/infinite-you/pkg/transports/cli/factory"
	cliobservation "github.com/portpowered/infinite-you/pkg/transports/cli/observation"
)

type processCommandRunner struct{}

func TestCLIRunDefaultsRetainWireSelectedRecordingTargetPlanner(t *testing.T) {
	t.Parallel()

	planner := recordings.LiveRecordingTargetPlannerFunc(func(recordings.LiveRecordingTargetRequest) (recordings.LiveRecordingTarget, error) {
		return recordings.LiveRecordingTarget{}, nil
	})
	recordingsCLI := provideRecordingsCLIAdapter()
	defaults := provideCLIRunDefaults(planner, recordingsCLI)
	if defaults.RecordingTargetPlanner == nil {
		t.Fatal("CLI run defaults dropped the Wire-selected recording target planner")
	}
	if defaults.RecordingsCLI == nil {
		t.Fatal("CLI run defaults dropped the Wire-selected Recordings CLI adapter")
	}
}

func TestProductionLiveRecordingTargetPlannerIsUsable(t *testing.T) {
	t.Parallel()

	target, err := provideLiveRecordingTargetPlanner().PlanLiveRecordingTarget(recordings.LiveRecordingTargetRequest{
		HomeDir:           t.TempDir(),
		ReportedSessionID: "~default",
	})
	if err != nil {
		t.Fatalf("PlanLiveRecordingTarget: %v", err)
	}
	if target.ServicePath == "" || target.ReportedPath == "" || target.ServicePath == target.ReportedPath {
		t.Fatalf("target = %#v, want distinct runtime template and reported paths", target)
	}
}

func (*processCommandRunner) Run(
	context.Context,
	platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	return platformprocess.CommandResult{}, nil
}

func TestGeneratedBundleHasNoSecondaryRuntimeOrInvocationBuilder(t *testing.T) {
	t.Parallel()

	source, err := os.ReadFile("wire_gen.go")
	if err != nil {
		t.Fatalf("read wire_gen.go: %v", err)
	}
	for _, forbidden := range []string{
		"ProvideRuntimeBuilder(",
		"NewInvocationBootstrapBuilder(",
		"NewRuntimeFactory(runtimeBuilder",
		"NewRuntimeFactoryFromOpening(",
		"application.NewFactory(",
		"application2.NewFactory(",
		"stdio.NewFactory(",
		"provideScopeAttacher(",
		"OpenedRuntime",
	} {
		if strings.Contains(string(source), forbidden) {
			t.Errorf("wire_gen.go contains secondary composition %q", forbidden)
		}
	}
}

func TestRootBundleProducesFreshDetachedCLIObservation(t *testing.T) {
	t.Parallel()

	observations := make([]cliobservation.Result, 0, 2)
	rootBundle, err := InjectBundle(t.Context(), serviceedges.Edges{CLIObserver: cliobservation.CaptureAppend(&observations)})
	if err != nil {
		t.Fatalf("InjectBundle() error = %v", err)
	}
	executeCLIObservation(t, rootBundle)
	executeCLIObservation(t, rootBundle)
	if len(observations) != 2 {
		t.Fatalf("CLI observations = %d, want 2", len(observations))
	}
	first, second := observations[0], observations[1]
	if !reflect.DeepEqual(first.Snapshot, second.Snapshot) {
		t.Fatal("reusable process produced different detached CLI snapshots")
	}
	if first.Snapshot.Commands.RootPath != "you" {
		t.Fatalf("root bundle command root = %q, want you", first.Snapshot.Commands.RootPath)
	}
}

func TestInjectBundleReturnsLazyServiceComposition(t *testing.T) {
	t.Parallel()

	var observation cliobservation.Result
	rootBundle, err := InjectBundle(t.Context(), serviceedges.Edges{CLIObserver: cliobservation.Capture(&observation)})
	if err != nil {
		t.Fatalf("InjectBundle() error = %v", err)
	}
	if rootBundle == nil {
		t.Fatal("InjectBundle() returned incomplete composition")
	}
	executeCLIObservation(t, rootBundle)
	if len(observation.Snapshot.Commands.Commands) == 0 {
		t.Fatal("InjectBundle() omitted the canonical CLI observation")
	}
}

func TestInjectBundlePreservesOverridesInCanonicalLazyComposition(t *testing.T) {
	t.Parallel()

	runner := &processCommandRunner{}
	var observation cliobservation.Result
	overrideEdges := serviceedges.Edges{ProviderCommandRunner: runner, CLIObserver: cliobservation.Capture(&observation)}
	custom, err := InjectBundle(t.Context(), overrideEdges)
	if err != nil {
		t.Fatalf("InjectBundle() error = %v", err)
	}

	if custom == nil {
		t.Fatal("InjectBundle() returned nil process")
	}
	executeCLIObservation(t, custom)
	if len(observation.Snapshot.Commands.Commands) == 0 {
		t.Fatal("InjectBundle() omitted the canonical lazy composition")
	}
}

func executeCLIObservation(t *testing.T, process *initializerapplication.Process) {
	t.Helper()
	home := t.TempDir()
	err := process.Execute(initializerapplication.Input{
		Args: []string{"you"}, Env: []string{"HOME=" + home, "USERPROFILE=" + home},
		WorkingDirectory: home, Stdout: io.Discard, Stderr: io.Discard,
	})
	if err != nil {
		t.Fatalf("Process.Execute(observe CLI) error = %v", err)
	}
}

func TestEffectiveFactoryCatalogServiceFeedsListAndNameProjectionsOnce(t *testing.T) {
	t.Parallel()

	calls := 0
	expected := factorydefinitions.ListEffectiveFactoriesResult{
		Entries: []factorydefinitions.EffectiveFactoryCatalogEntry{
			{
				Name: "alpha",
				Definition: &factorydefinitions.FactoryConfig{
					Description: &factorydefinitions.NameValueConfig{Value: "Alpha Factory"},
				},
			},
			{
				Name: "beta",
				Definition: &factorydefinitions.FactoryConfig{
					Description: &factorydefinitions.NameValueConfig{Value: "Beta Factory"},
				},
			},
		},
	}
	operation := factorydefinitions.EffectiveFactoryCatalogOperation(func(
		context.Context,
		factorydefinitions.ListEffectiveFactoriesRequest,
	) (factorydefinitions.ListEffectiveFactoriesResult, error) {
		calls++
		return expected, nil
	})
	definitions, err := provideEffectiveFactoryDefinitionsService(operation)
	if err != nil {
		t.Fatalf("provide effective Factory Definitions service: %v", err)
	}

	catalog, err := definitions.ListEffectiveFactories(
		context.Background(),
		factorydefinitions.ListEffectiveFactoriesRequest{
			ProjectRoot: "project",
			GlobalRoot:  "global",
		},
	)
	if err != nil {
		t.Fatalf("ListEffectiveFactories() error = %v", err)
	}
	listEntries, err := factorycli.ProjectEffectiveFactoryList(
		context.Background(),
		catalog,
		"beta",
	)
	if err != nil {
		t.Fatalf("ProjectEffectiveFactoryList() error = %v", err)
	}
	nameProjection, err := completionprojection.ProjectFactoryNames(
		context.Background(),
		catalog,
	)
	if err != nil {
		t.Fatalf("ProjectFactoryNames() error = %v", err)
	}

	if calls != 1 {
		t.Fatalf("catalog discovery calls = %d, want one shared service result", calls)
	}
	if len(listEntries) != 2 || len(nameProjection.Candidates) != 2 {
		t.Fatalf(
			"list entries = %#v, candidates = %#v, want two of each",
			listEntries,
			nameProjection.Candidates,
		)
	}
	gotListNames := []string{listEntries[0].Name, listEntries[1].Name}
	gotCandidateNames := []string{
		nameProjection.Candidates[0].Value,
		nameProjection.Candidates[1].Value,
	}
	if !reflect.DeepEqual(gotListNames, gotCandidateNames) {
		t.Fatalf("list names = %v, candidate names = %v", gotListNames, gotCandidateNames)
	}
	if !listEntries[1].Current {
		t.Fatalf("list entries = %#v, want beta marked current", listEntries)
	}
	if listEntries[0].Description != nameProjection.Candidates[0].Description ||
		listEntries[1].Description != nameProjection.Candidates[1].Description {
		t.Fatalf(
			"list descriptions = %#v, candidate descriptions = %#v",
			listEntries,
			nameProjection.Candidates,
		)
	}
}

func TestProvideListFactoriesOperationCallsFactoryDefinitionsOwner(t *testing.T) {
	t.Parallel()

	calls := 0
	definitions, err := provideEffectiveFactoryDefinitionsService(
		func(
			context.Context,
			factorydefinitions.ListEffectiveFactoriesRequest,
		) (factorydefinitions.ListEffectiveFactoriesResult, error) {
			calls++
			return factorydefinitions.ListEffectiveFactoriesResult{
				Entries: []factorydefinitions.EffectiveFactoryCatalogEntry{{
					Name:       "owned",
					Definition: &factorydefinitions.FactoryConfig{},
				}},
			}, nil
		},
	)
	if err != nil {
		t.Fatalf("provide effective Factory Definitions service: %v", err)
	}
	list := provideListFactoriesOperation(
		definitions,
		func(string) (string, error) { return "", nil },
	)
	var output bytes.Buffer
	if err := list(factorycli.ListConfig{
		Context:     context.Background(),
		ProjectRoot: "project",
		GlobalRoot:  "global",
		JSON:        true,
		Output:      &output,
	}); err != nil {
		t.Fatalf("list operation error = %v", err)
	}

	var entries []factorycli.ListEntry
	if err := json.Unmarshal(output.Bytes(), &entries); err != nil {
		t.Fatalf("decode list output: %v", err)
	}
	if calls != 1 || len(entries) != 1 || entries[0].Name != "owned" {
		t.Fatalf("service calls = %d, entries = %#v", calls, entries)
	}
}
