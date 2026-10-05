package named_lifecycle

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	packagedfactories "github.com/portpowered/infinite-you/packages/packaged-factories"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// Published Factory materialization crosses validation, rendering, storage,
// and loading boundaries. Prove that composition through customer commands.
func TestPackagedFactoryInstallFormatsValidateAndPreserveCustomerFiles(t *testing.T) {
	t.Parallel()
	for _, format := range []string{"json", "yaml", "yml"} {
		t.Run(format, func(t *testing.T) {
			t.Parallel()
			home, working := t.TempDir(), t.TempDir()
			installRoot := filepath.Join(working, "installed")
			execute := func(args ...string) *support.CapturedInputs {
				t.Helper()
				inputs := support.FakeInputs(t.Context(), args)
				inputs.Input.Env = []string{"HOME=" + home, "USERPROFILE=" + home}
				inputs.Input.WorkingDirectory = working
				if err := namedLifecycleProcess.Execute(inputs.Input); err != nil {
					t.Fatalf("Execute(%v): %v; stderr=%s", args, err, inputs.Stderr())
				}
				return inputs
			}
			args := []string{"you", "init", "--package", "@you/goal", "--dir", installRoot, "--format", format}
			installed := execute(args...)
			if !strings.Contains(installed.Stdout(), "Installed packaged factory @you/goal") {
				t.Fatalf("install output = %q", installed.Stdout())
			}
			factoryDir := filepath.Join(installRoot, "@you", "goal")
			root := filepath.Join(factoryDir, "factory."+format)
			assertInstalledGoalPrompts(t, factoryDir)
			execute("you", "factory", "config", "validate", root)
			originalName := installedFactoryPublicName(t, execute("you", "factory", "config", "flatten", root).Stdout())
			editInstalledFactoryName(t, root, format)
			if name := installedFactoryPublicName(t, execute("you", "factory", "config", "flatten", root).Stdout()); name != "customer-edited" {
				t.Fatalf("public name after customer edit = %q", name)
			}
			marker := filepath.Join(factoryDir, "customer-owned.txt")
			if err := os.WriteFile(marker, []byte("keep"), 0600); err != nil {
				t.Fatal(err)
			}
			repeated := execute(args...)
			if !strings.Contains(repeated.Stdout(), "already installed") {
				t.Fatalf("repeat output = %q", repeated.Stdout())
			}
			if body, err := os.ReadFile(marker); err != nil || string(body) != "keep" {
				t.Fatalf("repeat changed customer file: %q, %v", body, err)
			}
			if name := installedFactoryPublicName(t, execute("you", "factory", "config", "flatten", root).Stdout()); name != "customer-edited" {
				t.Fatalf("repeat overwrote customer configuration: %q", name)
			}
			execute(append(args, "--replace=true")...)
			assertInstalledGoalPrompts(t, factoryDir)
			if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("explicit replacement retained customer file: %v", err)
			}
			execute("you", "factory", "config", "validate", root)
			if name := installedFactoryPublicName(t, execute("you", "factory", "config", "flatten", root).Stdout()); name != originalName {
				t.Fatalf("public name after replacement = %q, want %q", name, originalName)
			}
		})
	}
}

func assertInstalledGoalPrompts(t *testing.T, factoryDir string) {
	t.Helper()
	for _, relative := range []string{"workers/goal-executor/AGENTS.md", "workstations/execute-goal/AGENTS.md", "workstations/execute-goal/prompts/executor.md"} {
		body, err := os.ReadFile(filepath.Join(factoryDir, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatalf("editable prompt %s: %v", relative, err)
		}
		if strings.HasSuffix(relative, "prompts/executor.md") {
			authored, err := fs.ReadFile(packagedfactories.Source(), "factories/goal/prompts/executor.md")
			if err != nil || string(body) != string(authored) {
				t.Fatalf("materialized executor prompt differs from authored source: %v", err)
			}
		}
	}
}

func installedFactoryPublicName(t *testing.T, output string) string {
	t.Helper()
	var factory struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(output), &factory); err != nil {
		t.Fatalf("decode public flatten result: %v", err)
	}
	return factory.Name
}

func editInstalledFactoryName(t *testing.T, path, format string) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var edited []byte
	if format == "json" {
		var document map[string]any
		if err := json.Unmarshal(body, &document); err != nil {
			t.Fatal(err)
		}
		document["name"] = "customer-edited"
		edited, err = json.MarshalIndent(document, "", "  ")
	} else {
		edited = regexp.MustCompile(`(?m)^name:.*$`).ReplaceAll(body, []byte("name: customer-edited"))
	}
	if err != nil || string(edited) == string(body) {
		t.Fatalf("edit customer configuration: %v", err)
	}
	if err := os.WriteFile(path, edited, 0600); err != nil {
		t.Fatal(err)
	}
}
