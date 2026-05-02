package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestWriteExpandedFactoryLayout_CopiesReferencedScriptForOptedInScriptWorkstation(t *testing.T) {
	sourceDir := t.TempDir()
	targetDir := t.TempDir()

	cfg := portableScriptFactoryConfig(true, "python", []string{"scripts/setup-workspace.py", "--mode", "portable"})
	canonical := flattenLayoutTestFactory(t, cfg)
	scriptPath := filepath.Join(sourceDir, "scripts", "setup-workspace.py")
	writeLayoutScriptTestFile(t, scriptPath, "#!/usr/bin/env python3\nprint('portable setup')\n")

	if err := writeExpandedFactoryLayout(sourceDir, targetDir, cfg, canonical, filepath.Join(sourceDir, interfaces.FactoryConfigFile)); err != nil {
		t.Fatalf("writeExpandedFactoryLayout: %v", err)
	}

	copiedPath := filepath.Join(targetDir, "scripts", "setup-workspace.py")
	copied, err := os.ReadFile(copiedPath)
	if err != nil {
		t.Fatalf("read copied script: %v", err)
	}
	if string(copied) != "#!/usr/bin/env python3\nprint('portable setup')\n" {
		t.Fatalf("copied script content = %q", string(copied))
	}

	loaded, err := LoadRuntimeConfig(targetDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig(expanded layout): %v", err)
	}
	worker, ok := loaded.Worker("workspace-setup")
	if !ok {
		t.Fatal("expected copied-script worker definition to load")
	}
	if worker.Type != interfaces.WorkerTypeScript || worker.Command != "python" {
		t.Fatalf("loaded worker = %#v", worker)
	}
	if len(worker.Args) < 1 || worker.Args[0] != "scripts/setup-workspace.py" {
		t.Fatalf("loaded worker args = %#v", worker.Args)
	}
}

func TestWriteExpandedFactoryLayout_DoesNotCopyReferencedScriptWhenOptOut(t *testing.T) {
	sourceDir := t.TempDir()
	targetDir := t.TempDir()

	cfg := portableScriptFactoryConfig(false, "powershell", []string{"-File", "scripts/execute-story.ps1"})
	canonical := flattenLayoutTestFactory(t, cfg)
	writeLayoutScriptTestFile(t, filepath.Join(sourceDir, "scripts", "execute-story.ps1"), "Write-Output 'portable'\n")

	if err := writeExpandedFactoryLayout(sourceDir, targetDir, cfg, canonical, filepath.Join(sourceDir, interfaces.FactoryConfigFile)); err != nil {
		t.Fatalf("writeExpandedFactoryLayout: %v", err)
	}

	if _, err := os.Stat(filepath.Join(targetDir, "scripts", "execute-story.ps1")); !os.IsNotExist(err) {
		t.Fatalf("expected referenced script not to be copied, stat err = %v", err)
	}
}

func TestWriteExpandedFactoryLayout_SkipsInterpreterFlagValuesBeforeScriptPath(t *testing.T) {
	sourceDir := t.TempDir()
	targetDir := t.TempDir()

	cfg := portableScriptFactoryConfig(true, "node", []string{"--loader", "ts-node/esm", "scripts/run.ts"})
	canonical := flattenLayoutTestFactory(t, cfg)
	writeLayoutScriptTestFile(t, filepath.Join(sourceDir, "scripts", "run.ts"), "console.log('portable');\n")

	if err := writeExpandedFactoryLayout(sourceDir, targetDir, cfg, canonical, filepath.Join(sourceDir, interfaces.FactoryConfigFile)); err != nil {
		t.Fatalf("writeExpandedFactoryLayout: %v", err)
	}

	if _, err := os.Stat(filepath.Join(targetDir, "ts-node", "esm")); !os.IsNotExist(err) {
		t.Fatalf("expected loader value not to be copied, stat err = %v", err)
	}

	copiedPath := filepath.Join(targetDir, "scripts", "run.ts")
	copied, err := os.ReadFile(copiedPath)
	if err != nil {
		t.Fatalf("read copied script: %v", err)
	}
	if string(copied) != "console.log('portable');\n" {
		t.Fatalf("copied script content = %q", string(copied))
	}
}

func TestWriteExpandedFactoryLayout_RejectsUnsafeReferencedScriptPaths(t *testing.T) {
	tests := []struct {
		name    string
		command string
		args    []string
		want    string
	}{
		{
			name:    "absolute command path",
			command: "__ABSOLUTE__",
			want:    "must be relative to the factory directory",
		},
		{
			name:    "escaping script arg path",
			command: "python",
			args:    []string{"../scripts/setup.py"},
			want:    "cannot escape the factory directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sourceDir := t.TempDir()
			targetDir := t.TempDir()
			command := tt.command
			if command == "__ABSOLUTE__" {
				command = filepath.Join(t.TempDir(), "setup.py")
			}
			cfg := portableScriptFactoryConfig(true, command, tt.args)
			canonical := flattenLayoutTestFactory(t, cfg)

			err := writeExpandedFactoryLayout(sourceDir, targetDir, cfg, canonical, filepath.Join(sourceDir, interfaces.FactoryConfigFile))
			if err == nil {
				t.Fatal("expected unsafe referenced script path to fail")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tt.want)
			}
		})
	}
}

func portableScriptFactoryConfig(copyReferencedScripts bool, command string, args []string) *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Resources: []interfaces.ResourceConfig{},
		Workers: []interfaces.WorkerConfig{
			{
				Name:    "workspace-setup",
				Type:    interfaces.WorkerTypeScript,
				Command: command,
				Args:    append([]string(nil), args...),
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name:                  "setup-workspace",
				Type:                  interfaces.WorkstationTypeModel,
				WorkerTypeName:        "workspace-setup",
				CopyReferencedScripts: copyReferencedScripts,
				Inputs: []interfaces.IOConfig{
					{WorkTypeName: "task", StateName: "init"},
				},
				Outputs: []interfaces.IOConfig{
					{WorkTypeName: "task", StateName: "complete"},
				},
			},
		},
	}
}

func flattenLayoutTestFactory(t *testing.T, cfg *interfaces.FactoryConfig) []byte {
	t.Helper()

	canonical, err := NewFactoryConfigMapper().Flatten(cfg)
	if err != nil {
		t.Fatalf("flatten test factory: %v", err)
	}
	return canonical
}

func writeLayoutScriptTestFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
