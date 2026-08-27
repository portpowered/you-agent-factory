package definitions

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const sharedDefinitionsProcessShutdownTimeout = 15 * time.Second

// sharedDefinitionsProcessFixture owns the immutable root composition used by
// sequential CLI/static definition scenarios. Every Process.Execute call still
// supplies its own environment, working directory, paths, streams, and public
// command arguments; only process wiring is shared.
type sharedDefinitionsProcessFixture struct {
	process      support.ApplicationProcess
	providerEdge *support.RecordingCommandRunner
}

var (
	sharedDefinitionsFixtureOnce sync.Once
	sharedDefinitionsFixture     *sharedDefinitionsProcessFixture
	sharedDefinitionsFixtureErr  error
)

// TestMain closes the one package-scoped process after all sequential child
// invocations finish. Per-test t.Cleanup cannot own this process because it
// would close the shared wiring before the next top-level test runs.
func TestMain(m *testing.M) {
	code := m.Run()
	if sharedDefinitionsFixture != nil {
		ctx, cancel := context.WithTimeout(context.Background(), sharedDefinitionsProcessShutdownTimeout)
		err := sharedDefinitionsFixture.process.Close(ctx)
		cancel()
		if err != nil {
			fmt.Fprintf(os.Stderr, "close shared Factory Definitions process: %v\n", err)
			if code == 0 {
				code = 1
			}
		}
	}
	os.Exit(code)
}

func sharedDefinitionsProcess(t testing.TB) support.ApplicationProcess {
	t.Helper()
	sharedDefinitionsFixtureOnce.Do(func() {
		sharedDefinitionsFixture, sharedDefinitionsFixtureErr = startSharedDefinitionsProcess()
	})
	if sharedDefinitionsFixtureErr != nil {
		t.Fatalf("start shared Factory Definitions process: %v", sharedDefinitionsFixtureErr)
	}
	if sharedDefinitionsFixture == nil || sharedDefinitionsFixture.process == nil {
		t.Fatal("shared Factory Definitions process is unavailable")
	}
	return sharedDefinitionsFixture.process
}

func startSharedDefinitionsProcess() (*sharedDefinitionsProcessFixture, error) {
	providerEdge := support.NewRecordingCommandRunner("unexpected provider invocation in static definition group")
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		ProviderCommandRunner: providerEdge,
	})
	if err != nil {
		return nil, err
	}
	return &sharedDefinitionsProcessFixture{
		process:      process,
		providerEdge: providerEdge,
	}, nil
}

func sharedDefinitionsProviderCallCount(t testing.TB) int {
	t.Helper()
	sharedDefinitionsProcess(t)
	return sharedDefinitionsFixture.providerEdge.CallCount()
}

func sharedDefinitionsProviderRunner(t testing.TB) *support.RecordingCommandRunner {
	t.Helper()
	sharedDefinitionsProcess(t)
	return sharedDefinitionsFixture.providerEdge
}

// TestFactoryDefinitionsSharedProcessKeepsHomesAndCurrentFactoriesIsolated
// proves that valid persistence in two customer homes and a rejected public
// validation operation can run sequentially on one root process without
// crossing Current Factory or named-catalog state.
func TestFactoryDefinitionsSharedProcessKeepsHomesAndCurrentFactoriesIsolated(t *testing.T) {
	process := sharedDefinitionsProcess(t)
	providerCallsBefore := sharedDefinitionsProviderCallCount(t)

	homeA, workingA, envA := sharedDefinitionsHome(t)
	homeB, workingB, envB := sharedDefinitionsHome(t)
	namedRootA := sharedDefinitionsNamedFactoryRoot(t, homeA)
	namedRootB := sharedDefinitionsNamedFactoryRoot(t, homeB)

	sourceA := support.ScaffoldFactory(t, sharedDefinitionsFactoryConfig("factory-a", "task-a"))
	sourceB := support.ScaffoldFactory(t, sharedDefinitionsFactoryConfig("factory-b", "task-b"))
	factoryDirA := support.CreateAndActivateNamedFactoryAtRootWithProcess(
		t, process, envA, workingA, namedRootA, "factory-a", filepath.Join(sourceA, factorydefinitions.FactoryConfigFile),
	)
	factoryDirB := support.CreateAndActivateNamedFactoryAtRootWithProcess(
		t, process, envB, workingB, namedRootB, "factory-b", filepath.Join(sourceB, factorydefinitions.FactoryConfigFile),
	)

	baselineA := sharedDefinitionsReadback(t, process, envA, factoryDirA, "factory-a", "task-a")
	baselineB := sharedDefinitionsReadback(t, process, envB, factoryDirB, "factory-b", "task-b")
	sharedDefinitionsAssertFactoryList(t, process, envA, workingA, "factory-a", factoryDirA, "factory-b")
	sharedDefinitionsAssertFactoryList(t, process, envB, workingB, "factory-b", factoryDirB, "factory-a")

	invalidDir := support.ScaffoldFactory(t, compilationInvalidFactoryConfig())
	invalidInputs := support.FakeInputs(t.Context(), []string{
		"you", "factory", "config", "validate", filepath.Join(invalidDir, factorydefinitions.FactoryConfigFile),
	})
	invalidInputs.Input.Env = append([]string(nil), envB...)
	invalidInputs.Input.WorkingDirectory = workingB
	if err := process.Execute(invalidInputs.Input); err == nil {
		t.Fatalf(
			"Process.Execute(invalid validation) error = nil, want rejection; stdout=%q stderr=%q",
			invalidInputs.Stdout(), invalidInputs.Stderr(),
		)
	} else {
		diagnostic := err.Error() + "\n" + invalidInputs.Stdout() + "\n" + invalidInputs.Stderr()
		for _, want := range []string{"Factory validation failed.", validationCodeDanglingWorkerReference, compilationMissingWorker} {
			if !strings.Contains(diagnostic, want) {
				t.Fatalf("invalid validation diagnostic = %q, want %q", diagnostic, want)
			}
		}
	}

	afterA := sharedDefinitionsReadback(t, process, envA, factoryDirA, "factory-a", "task-a")
	afterB := sharedDefinitionsReadback(t, process, envB, factoryDirB, "factory-b", "task-b")
	if !reflect.DeepEqual(afterA, baselineA) {
		t.Fatalf("home A Factory changed after invalid home B validation:\nbefore=%#v\nafter=%#v", baselineA, afterA)
	}
	if !reflect.DeepEqual(afterB, baselineB) {
		t.Fatalf("home B Factory changed after its invalid validation:\nbefore=%#v\nafter=%#v", baselineB, afterB)
	}
	sharedDefinitionsAssertFactoryList(t, process, envA, workingA, "factory-a", factoryDirA, "factory-b")
	sharedDefinitionsAssertFactoryList(t, process, envB, workingB, "factory-b", factoryDirB, "factory-a")
	if got := sharedDefinitionsProviderCallCount(t); got != providerCallsBefore {
		t.Fatalf("provider edge calls during static isolation canary = %d, want unchanged %d", got, providerCallsBefore)
	}
}

func sharedDefinitionsHome(t *testing.T) (string, string, []string) {
	t.Helper()
	home := t.TempDir()
	working := t.TempDir()
	return home, working, append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
}

func sharedDefinitionsNamedFactoryRoot(t *testing.T, home string) string {
	t.Helper()
	root := filepath.Join(home, ".you-agent-factory", "factories")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("create named Factory root %s: %v", root, err)
	}
	return root
}

func sharedDefinitionsFactoryConfig(name, workType string) map[string]any {
	worker := name + "-worker"
	workstation := name + "-process"
	return map[string]any{
		"name": name,
		"workTypes": []map[string]any{{
			"name": workType,
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{"name": worker}},
		"workstations": []map[string]any{{
			"name":      workstation,
			"worker":    worker,
			"inputs":    []map[string]string{{"workType": workType, "state": "init"}},
			"outputs":   []map[string]string{{"workType": workType, "state": "complete"}},
			"onFailure": []map[string]string{{"workType": workType, "state": "failed"}},
		}},
	}
}

func sharedDefinitionsReadback(
	t *testing.T,
	process support.Process,
	env []string,
	factoryDir string,
	wantName string,
	wantWorkType string,
) factoryapi.Factory {
	t.Helper()
	factory, err := support.LoadedFactoryWithProcessAndEnv(
		t, process, env, filepath.Join(factoryDir, factorydefinitions.FactoryConfigFile),
	)
	if err != nil {
		t.Fatalf("read Factory %s: %v", factoryDir, err)
	}
	if factory.Name != factoryapi.FactoryName(wantName) {
		t.Fatalf("read Factory %s name = %q, want %q", factoryDir, factory.Name, wantName)
	}
	if factory.WorkTypes == nil || len(*factory.WorkTypes) != 1 || (*factory.WorkTypes)[0].Name != wantWorkType {
		t.Fatalf("read Factory %s work types = %#v, want only %q", factoryDir, factory.WorkTypes, wantWorkType)
	}
	return factory
}

type sharedDefinitionsFactoryListEntry struct {
	Name             string `json:"name"`
	FactoryDirectory string `json:"factoryDirectory"`
	Current          bool   `json:"current"`
}

func sharedDefinitionsAssertFactoryList(
	t *testing.T,
	process support.Process,
	env []string,
	workingDir string,
	wantCurrent string,
	wantFactoryDir string,
	unwanted string,
) {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), []string{"you", "--json", "factory", "list"})
	inputs.Input.Env = append([]string(nil), env...)
	inputs.Input.WorkingDirectory = workingDir
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(factory list) error = %v\nstdout=%s\nstderr=%s", err, inputs.Stdout(), inputs.Stderr())
	}
	var entries []sharedDefinitionsFactoryListEntry
	if err := json.Unmarshal([]byte(inputs.Stdout()), &entries); err != nil {
		t.Fatalf("decode factory list: %v\nstdout=%s", err, inputs.Stdout())
	}
	var foundCurrent bool
	for _, entry := range entries {
		if entry.Name == unwanted {
			t.Fatalf("factory list for %q contains foreign Factory %q: %#v", wantCurrent, unwanted, entries)
		}
		if entry.Name == wantCurrent {
			foundCurrent = true
			if entry.FactoryDirectory != wantFactoryDir {
				t.Fatalf(
					"factory list entry %q directory = %q, want %q",
					wantCurrent,
					entry.FactoryDirectory,
					wantFactoryDir,
				)
			}
			if !entry.Current {
				t.Fatalf("factory list entry %q has current=false: %#v", wantCurrent, entry)
			}
		}
	}
	if !foundCurrent {
		t.Fatalf("factory list missing current Factory %q: %#v", wantCurrent, entries)
	}
}
