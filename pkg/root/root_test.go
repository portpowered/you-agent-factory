package root

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/config/defaultpaths"
)

func TestNormalizeSnapshotsArgumentsAndEnvironment(t *testing.T) {
	args := []string{"custom-you", "docs", "--", "", "--topic", "--topic"}
	environment := []string{"PRESENT=first", "EMPTY=", "PRESENT=last"}

	input, err := Normalize(Input{Args: args, Env: environment})
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	args[1] = "changed"
	environment[0] = "PRESENT=changed"

	if input.Executable() != "custom-you" {
		t.Fatalf("Executable() = %q, want custom-you", input.Executable())
	}
	wantArguments := []string{"docs", "--", "", "--topic", "--topic"}
	if got := strings.Join(input.Arguments(), "\x00"); got != strings.Join(wantArguments, "\x00") {
		t.Fatalf("Arguments() = %q, want %q", input.Arguments(), wantArguments)
	}
	returned := input.Arguments()
	returned[0] = "mutated"
	if input.Arguments()[0] != "docs" {
		t.Fatal("Arguments() exposed mutable normalized state")
	}
	if value, ok := input.LookupEnv("PRESENT"); !ok || value != "last" {
		t.Fatalf("LookupEnv(PRESENT) = %q, %t; want last, true", value, ok)
	}
	if value, ok := input.LookupEnv("EMPTY"); !ok || value != "" {
		t.Fatalf("LookupEnv(EMPTY) = %q, %t; want empty, true", value, ok)
	}
	if _, ok := input.LookupEnv("ABSENT"); ok {
		t.Fatal("LookupEnv(ABSENT) reported an absent value as present")
	}
}

func TestExecuteRoutesHelpAndExplicitCommandsToSuppliedStreams(t *testing.T) {
	t.Parallel()

	var help bytes.Buffer
	err := Execute(Input{
		Args:    []string{"renamed-binary", "--help"},
		Env:     homeEnvironment(t.TempDir()),
		Stdout:  &help,
		Context: context.Background(),
	})
	if err != nil {
		t.Fatalf("Execute(help) error = %v", err)
	}
	if !strings.HasPrefix(help.String(), "Run and manage CPN-based workflow factories") {
		t.Fatalf("help output = %q", help.String())
	}

	var docs bytes.Buffer
	err = Execute(Input{
		Args:    []string{"you", "docs", "agents"},
		Env:     homeEnvironment(t.TempDir()),
		Stdout:  &docs,
		Context: context.Background(),
	})
	if err != nil {
		t.Fatalf("Execute(docs agents) error = %v", err)
	}
	if !strings.Contains(docs.String(), "# Agents") {
		t.Fatalf("docs output does not contain agents topic: %q", docs.String())
	}
	if strings.Contains(help.String(), "# Agents") {
		t.Fatal("sequential execution leaked the second command's output into the first stream")
	}
}

func TestExecuteInvalidArgumentsReturnsDiagnostic(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer
	err := Execute(Input{
		Args:    []string{"you", "definitely-not-a-command"},
		Env:     homeEnvironment(t.TempDir()),
		Stderr:  &stderr,
		Context: context.Background(),
	})
	if err == nil {
		t.Fatal("Execute(invalid command) error = nil")
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("Execute(invalid command) error = %q", err)
	}
}

func TestExecuteSequentialHomesControlConfigAndRunPaths(t *testing.T) {
	ambientHome := t.TempDir()
	t.Setenv("HOME", ambientHome)
	t.Setenv("USERPROFILE", ambientHome)

	homes := []string{t.TempDir(), t.TempDir()}
	for _, home := range homes {
		if err := Execute(Input{
			Args: []string{"you", "config", "init"}, Env: homeEnvironment(home), Context: context.Background(),
		}); err != nil {
			t.Fatalf("Execute(config init, home %q) error = %v", home, err)
		}
		if _, err := os.Stat(defaultpaths.OperatorConfigPath(home)); err != nil {
			t.Fatalf("Stat(config for supplied home %q) error = %v", home, err)
		}

		builder := &recordingGraphBuilder{graph: &ApplicationGraph{}}
		initializer := &recordingInitializer{}
		err := ExecuteWithDependencies(Input{
			Args: []string{"you", "run", "--named", "@you/goal", "--no-record", "--quiet", "Plan the sprint"},
			Env:  homeEnvironment(home), Context: context.Background(),
		}, Dependencies{GraphBuilder: builder, Initializer: initializer})
		if err != nil {
			t.Fatalf("ExecuteWithDependencies(run, home %q) error = %v", home, err)
		}
		cfg := builder.request.Startup.RunConfig
		if cfg == nil || cfg.HomeDir != home {
			t.Fatalf("run home = %v, want %q", cfg, home)
		}
		wantGlobalRoot := defaultpaths.NamedFactoriesRoot(home)
		if cfg.NamedFactoryResolution == nil || cfg.NamedFactoryResolution.GlobalRoot != wantGlobalRoot {
			t.Fatalf("named-factory resolution = %+v, want global root %q", cfg.NamedFactoryResolution, wantGlobalRoot)
		}
		if !strings.HasPrefix(filepath.Clean(cfg.Dir), filepath.Clean(wantGlobalRoot)+string(os.PathSeparator)) {
			t.Fatalf("named-factory dir = %q, want below supplied home root %q", cfg.Dir, wantGlobalRoot)
		}
	}
	if _, err := os.Stat(defaultpaths.OperatorConfigPath(ambientHome)); !os.IsNotExist(err) {
		t.Fatalf("ambient config Stat error = %v, want not-exist", err)
	}
}

func TestExecuteSetupAndFactoryAuthoringCommandsThroughProductionComposition(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	factoryDir := filepath.Join(t.TempDir(), "authored-factory")
	var initOutput bytes.Buffer
	if err := Execute(Input{
		Args:    []string{"you", "init", "--dir", factoryDir},
		Env:     homeEnvironment(home),
		Stdout:  &initOutput,
		Context: context.Background(),
	}); err != nil {
		t.Fatalf("Execute(init) error = %v", err)
	}
	if !strings.Contains(initOutput.String(), "Initialized default factory directory structure") {
		t.Fatalf("init output = %q", initOutput.String())
	}

	var validateOutput bytes.Buffer
	if err := Execute(Input{
		Args:    []string{"you", "factory", "config", "validate", factoryDir},
		Env:     homeEnvironment(home),
		Stdout:  &validateOutput,
		Context: context.Background(),
	}); err != nil {
		t.Fatalf("Execute(factory config validate) error = %v", err)
	}
	if !strings.Contains(validateOutput.String(), "Factory validation passed") {
		t.Fatalf("factory validation output = %q", validateOutput.String())
	}

	missingPath := filepath.Join(t.TempDir(), "missing-factory")
	if err := Execute(Input{
		Args:    []string{"you", "factory", "config", "validate", missingPath},
		Env:     homeEnvironment(home),
		Context: context.Background(),
	}); err == nil || !strings.Contains(err.Error(), "find factory config") {
		t.Fatalf("Execute(factory config validate missing path) error = %v", err)
	}
	if _, err := os.Stat(missingPath); !os.IsNotExist(err) {
		t.Fatalf("missing factory path Stat error = %v, want not-exist", err)
	}
}

func homeEnvironment(home string) []string {
	if runtime.GOOS == "windows" {
		return []string{"USERPROFILE=" + home}
	}
	if runtime.GOOS == "plan9" {
		return []string{"home=" + home}
	}
	return []string{"HOME=" + home}
}

func TestNormalizeRejectsMissingExecutableAndMalformedEnvironment(t *testing.T) {
	t.Parallel()

	if _, err := Normalize(Input{}); err == nil || !strings.Contains(err.Error(), "executable") {
		t.Fatalf("Normalize(missing executable) error = %v", err)
	}
	if _, err := Normalize(Input{Args: []string{"you"}, Env: []string{"MALFORMED"}}); err == nil || !strings.Contains(err.Error(), "environment") {
		t.Fatalf("Normalize(malformed environment) error = %v", err)
	}
}
