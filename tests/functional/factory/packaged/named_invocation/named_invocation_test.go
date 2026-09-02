package named_invocation

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/root"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	packagedGoalFactoryName             = "@you/goal"
	packagedGoalExecuteWorkstationName  = "execute-goal"
	packagedSubagentFactoryName         = "@you/subagent"
	packagedSubagentWorkerName          = "subagent-worker"
	packagedSubagentRunWorkstationName  = "run-subagent"
	packagedFactoryBuilderName          = "@you/factory-builder"
	packagedTournamentFactoryName       = "@you/tournament"
	customizedNamedGoalFactoryName      = "@test/goal"
	wantHermeticInvocationPrimaryResult = "mock worker accepted"
)

func addEffectiveSignatureFixture(t *testing.T, factoryPath string) {
	t.Helper()
	payload, err := os.ReadFile(factoryPath)
	if err != nil {
		t.Fatalf("read installed Factory: %v", err)
	}
	var factory map[string]any
	if err := json.Unmarshal(payload, &factory); err != nil {
		t.Fatalf("decode installed Factory: %v", err)
	}
	factory["invocationSignature"] = map[string]any{
		"unknownNamedArgumentPolicy": "REJECT",
		"parameters": []any{
			map[string]any{
				"name":     "input",
				"required": true,
				"bindings": []any{map[string]any{"kind": "POSITIONAL", "position": 1}},
			},
			map[string]any{
				"name":      "files",
				"valueMode": "VARIADIC",
				"bindings":  []any{map[string]any{"kind": "POSITIONAL", "position": 2}},
			},
			map[string]any{
				"name":         "tags",
				"externalName": "tag",
				"aliases":      []any{"t"},
				"valueMode":    "REPEATED",
				"sensitive":    true,
				"bindings":     []any{map[string]any{"kind": "NAMED"}},
			},
			map[string]any{
				"name":         "format",
				"choices":      []any{"json", "text"},
				"defaultValue": "json",
				"bindings":     []any{map[string]any{"kind": "NAMED"}},
			},
			map[string]any{
				"name":     "count",
				"typeHint": "NUMBER_STRING",
				"bindings": []any{map[string]any{"kind": "NAMED"}},
			},
			map[string]any{
				"name":         "document",
				"externalName": "document",
				"aliases":      []any{"file"},
				"typeHint":     "FILE_PATH",
				"valueMode":    "FILE_CONTENTS",
				"bindings":     []any{map[string]any{"kind": "NAMED"}},
			},
			map[string]any{
				"name":     "body",
				"bindings": []any{map[string]any{"kind": "STDIN"}},
			},
		},
	}
	workersConfig, ok := factory["workers"].([]any)
	if !ok || len(workersConfig) == 0 {
		t.Fatalf("installed Factory workers = %#v", factory["workers"])
	}
	workerConfig, ok := workersConfig[0].(map[string]any)
	if !ok {
		t.Fatalf("installed Factory worker = %#v", workersConfig[0])
	}
	delete(workerConfig, "promptFile")
	workerConfig["body"] = "input=${input}|format=${format}|count=${count}|document=${document}|stdin=${body}"
	updated, err := json.MarshalIndent(factory, "", "  ")
	if err != nil {
		t.Fatalf("encode signature Factory fixture: %v", err)
	}
	if err := os.WriteFile(factoryPath, updated, 0o600); err != nil {
		t.Fatalf("write signature Factory fixture: %v", err)
	}
	support.ReplaceGoalWorkerInstructions(
		t,
		factoryPath,
		"input=${input}|format=${format}|count=${count}|document=${document}|stdin=${body}",
	)
}

func replaceInvocationSignatureFixture(t *testing.T, factoryPath string, signature map[string]any) {
	t.Helper()
	payload, err := os.ReadFile(factoryPath)
	if err != nil {
		t.Fatalf("read installed Factory: %v", err)
	}
	var factory map[string]any
	if err := json.Unmarshal(payload, &factory); err != nil {
		t.Fatalf("decode installed Factory: %v", err)
	}
	factory["invocationSignature"] = signature
	updated, err := json.MarshalIndent(factory, "", "  ")
	if err != nil {
		t.Fatalf("encode signature Factory fixture: %v", err)
	}
	if err := os.WriteFile(factoryPath, updated, 0o600); err != nil {
		t.Fatalf("write signature Factory fixture: %v", err)
	}
}

type customerProcess interface {
	Execute(root.Input) error
}

func executeCustomerCommand(
	t *testing.T,
	process customerProcess,
	environment []string,
	workingDirectory string,
	args []string,
) (string, string) {
	t.Helper()
	return executeCustomerCommandWithStdin(t, process, environment, workingDirectory, args, "")
}

func executeCustomerCommandWithStdin(
	t *testing.T,
	process customerProcess,
	environment []string,
	workingDirectory string,
	args []string,
	stdin string,
) (string, string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	stdinIsTTY := true
	stdoutIsTTY := false
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := process.Execute(root.Input{
		Args:             args,
		Env:              environment,
		Stdin:            strings.NewReader(stdin),
		Stdout:           &stdout,
		Stderr:           &stderr,
		Context:          ctx,
		WorkingDirectory: workingDirectory,
		StdinIsTTY:       &stdinIsTTY,
		StdoutIsTTY:      &stdoutIsTTY,
	})
	if err != nil {
		t.Fatalf(
			"Process.Execute(%v) error = %v; stdout=%q stderr=%q",
			args,
			err,
			stdout.String(),
			stderr.String(),
		)
	}
	return stdout.String(), stderr.String()
}

func initializePackagedFactory(
	t *testing.T,
	process customerProcess,
	environment []string,
	workingDirectory string,
	homeDir string,
	name string,
) string {
	t.Helper()
	factoriesRoot := filepath.Join(homeDir, ".you-agent-factory", "factories")
	missingFactory := filepath.Join(workingDirectory, "missing-initialization-factory.json")
	err := process.Execute(root.Input{
		Args:             []string{"you", "run", "--factory", missingFactory},
		Env:              environment,
		Context:          t.Context(),
		WorkingDirectory: workingDirectory,
	})
	if err == nil || !strings.Contains(err.Error(), filepath.Base(missingFactory)) {
		t.Fatalf("Process.Execute(run missing Factory) error = %v", err)
	}
	factoryDir := filepath.Join(
		append([]string{factoriesRoot}, strings.Split(name, "/")...)...,
	)
	if _, err := os.Stat(filepath.Join(factoryDir, "factory.json")); err != nil {
		t.Fatalf("installed packaged Factory %q: %v", name, err)
	}
	return factoryDir
}
