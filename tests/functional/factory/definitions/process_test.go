package definitions

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	definitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func buildDefinitionsProcess(t testing.TB) support.Process {
	t.Helper()
	return sharedDefinitionsProcessForTest(t)
}

// Static commands own independent homes and files; execution behavior is proved
// in named Sessions by TestDefinitionsHTTPValidationIsolation on the shared host.
func TestDefinitionsFixedRolesInstallReadbackAndFailures(t *testing.T) {
	t.Parallel()
	process := buildDefinitionsProcess(t)
	_, working, env := sharedDefinitionsHome(t)
	root := t.TempDir()
	run := func(ctx context.Context, args ...string) (string, error) {
		inputs := support.FakeInputs(ctx, append([]string{"you", "--json"}, args...))
		inputs.Input.Env, inputs.Input.WorkingDirectory = env, working
		err := process.Execute(inputs.Input)
		return inputs.Stdout(), err
	}
	installed := fixedRolesInstall(t, run, root, "created")
	original, err := os.ReadFile(filepath.Join(installed.FactoryDir, "factory.json"))
	if err != nil {
		t.Fatal(err)
	}
	marker := []byte(`{"name":"customer-edited"}`)
	if err := os.WriteFile(filepath.Join(installed.FactoryDir, "factory.json"), marker, 0o600); err != nil {
		t.Fatal(err)
	}
	fixedRolesInstall(t, run, root, "skipped")
	retained, err := os.ReadFile(filepath.Join(installed.FactoryDir, "factory.json"))
	if err != nil || !reflect.DeepEqual(retained, marker) {
		t.Fatalf("skip replaced customer content: %s (%v)", retained, err)
	}
	fixedRolesInstall(t, run, root, "replaced", "--replace", "true")
	restored, err := os.ReadFile(filepath.Join(installed.FactoryDir, "factory.json"))
	if err != nil || !reflect.DeepEqual(restored, original) {
		t.Fatalf("replacement did not restore packaged content: %v", err)
	}
	factory, err := support.LoadedFactoryWithProcessAndEnv(t, process, env, filepath.Join(installed.FactoryDir, "factory.json"))
	if err != nil || factory.Name == "" || factory.Workstations == nil || len(*factory.Workstations) == 0 {
		t.Fatalf("installed Factory is not useful: %#v (%v)", factory, err)
	}
	unknownRoot := t.TempDir()
	output, err := run(t.Context(), "init", "--package", "@you/unknown-fixed-role", "--dir", unknownRoot)
	if !errors.Is(err, definitions.ErrUnknownPackagedFactoryIdentity) || strings.Contains(output, `"outcome":"created"`) {
		t.Fatalf("unknown package result=%s error=%v", output, err)
	}
	assertFixedRolesEmpty(t, unknownRoot)
	deniedRoot := filepath.Join(t.TempDir(), "fixed-role-install-denied")
	if err := os.MkdirAll(deniedRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	output, err = run(t.Context(), "init", "--package", "@you/goal", "--dir", deniedRoot)
	if !errors.Is(err, os.ErrPermission) || strings.Contains(output, `"outcome":"created"`) {
		t.Fatalf("install write cause lost: %s (%v)", output, err)
	}
	if _, err := os.Stat(filepath.Join(deniedRoot, "@you", "goal")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed install published target: %v", err)
	}
	if _, err := support.LoadedFactoryWithProcessAndEnv(t, process, env, filepath.Join(installed.FactoryDir, "factory.json")); err != nil {
		t.Fatalf("write failure damaged peer: %v", err)
	}
	fixedRolesContextFailures(t, run)
}

func fixedRolesInstall(t *testing.T, run func(context.Context, ...string) (string, error), root, outcome string, extra ...string) struct{ Name, FactoryDir, Outcome, Format string } {
	t.Helper()
	args := append([]string{"init", "--package", "@you/goal", "--dir", root, "--format", "json"}, extra...)
	output, err := run(t.Context(), args...)
	if err != nil {
		t.Fatalf("install %s: %v output=%s", outcome, err, output)
	}
	var result struct{ Name, FactoryDir, Outcome, Format string }
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("install JSON=%s: %v", output, err)
	}
	if result.Name != "@you/goal" || result.Outcome != outcome || result.Format != string(definitions.PackagedFactoryFormatJSON) || result.FactoryDir == "" {
		t.Fatalf("install facts=%#v, want %s", result, outcome)
	}
	return result
}

func fixedRolesContextFailures(t *testing.T, run func(context.Context, ...string) (string, error)) {
	t.Helper()
	source := support.ScaffoldFactory(t, validAPIValidationFactoryConfig())
	for _, expired := range []bool{false, true} {
		ctx, cancel := context.WithCancel(t.Context())
		want := context.Canceled
		if expired {
			cancel()
			ctx, cancel = context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
			want = context.DeadlineExceeded
		} else {
			cancel()
		}
		for _, operation := range []string{"create", "install"} {
			root := t.TempDir()
			args := []string{"init", "--package", "@you/goal", "--dir", root}
			if operation == "create" {
				args = []string{"factory", "create", "canceled", "--from", filepath.Join(source, "factory.json"), "--dir", root}
			}
			output, err := run(ctx, args...)
			matches := errors.Is(err, want)
			// Editable pre-persist validation retains its existing domain error
			// with the expired-context diagnostic rather than unwrapping that cause.
			if expired && operation == "create" {
				matches = errors.Is(err, definitions.ErrInvalidNamedFactory) && strings.Contains(err.Error(), want.Error())
			}
			if !matches || strings.Contains(output, `"outcome":"created"`) {
				t.Fatalf("%s canceled=%t output=%s error=%v, want %v", operation, !expired, output, err, want)
			}
			assertFixedRolesEmpty(t, root)
		}
		cancel()
	}
}

func assertFixedRolesEmpty(t *testing.T, root string) {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatalf("failure published target in %s: %v (%v)", root, entries, err)
	}
}

func TestDefinitionsFixedRolesCreateFailureLeavesPeerReadable(t *testing.T) {
	t.Parallel()
	process := buildDefinitionsProcess(t)
	_, working, env := sharedDefinitionsHome(t)
	root := t.TempDir()
	source := support.ScaffoldFactory(t, sharedDefinitionsFactoryConfig("peer", "task"))
	peer := support.CreateAndActivateNamedFactoryAtRootWithProcess(t, process, env, working, root, "peer", filepath.Join(source, "factory.json"))
	before := sharedDefinitionsReadback(t, process, env, peer, "peer", "task")
	for _, name := range []string{"write-blocked", "invalid"} {
		selected := source
		if name == "invalid" {
			selected = support.ScaffoldFactory(t, compilationInvalidFactoryConfig())
		}
		inputs := support.FakeInputs(t.Context(), []string{"you", "--json", "factory", "create", name, "--from", filepath.Join(selected, "factory.json"), "--dir", root})
		inputs.Input.Env, inputs.Input.WorkingDirectory = env, working
		err := process.Execute(inputs.Input)
		if err == nil {
			t.Fatalf("%s falsely succeeded: %s", name, inputs.Stdout())
		}
		if name == "write-blocked" && !errors.Is(err, os.ErrPermission) {
			t.Fatalf("write cause lost: %v", err)
		}
		if name == "invalid" && !strings.Contains(err.Error()+inputs.Stdout()+inputs.Stderr(), validationCodeDanglingWorkerReference) {
			t.Fatalf("validation code lost: %v", err)
		}
		if _, err := os.Stat(filepath.Join(root, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("failed target published: %v", err)
		}
	}
	after := sharedDefinitionsReadback(t, process, env, peer, "peer", "task")
	if !reflect.DeepEqual(before, after) {
		t.Fatal("failed create changed peer")
	}
}
