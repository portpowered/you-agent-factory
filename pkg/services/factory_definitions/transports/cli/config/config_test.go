package config

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

type scriptedFactoryDefinitionPersistence struct {
	factorydefinitions.Persistence
	flatten func(string) ([]byte, error)
	expand  func(string) (string, factorydefinitions.LayoutExpansionReport, error)
}

func (s scriptedFactoryDefinitionPersistence) FlattenFactoryLayout(
	path string,
) ([]byte, error) {
	if s.flatten == nil {
		panic("unexpected FlattenFactoryLayout call for " + path)
	}
	return s.flatten(path)
}

func (s scriptedFactoryDefinitionPersistence) ExpandFactoryLayout(
	path string,
) (string, factorydefinitions.LayoutExpansionReport, error) {
	if s.expand == nil {
		panic("unexpected ExpandFactoryLayout call for " + path)
	}
	return s.expand(path)
}

func TestExpandFactoryConfig_VerboseLogsWrittenPathCountsWithoutChangingStdout(t *testing.T) {
	inputPath := filepath.Join(t.TempDir(), "factory.json")
	targetDir := t.TempDir()
	persistence := scriptedFactoryDefinitionPersistence{expand: func(path string) (
		string,
		factorydefinitions.LayoutExpansionReport,
		error,
	) {
		if path != inputPath {
			t.Fatalf("ExpandFactoryLayout path = %q, want %q", path, inputPath)
		}
		return targetDir, factorydefinitions.LayoutExpansionReport{
			FactoryConfigPaths:    1,
			WorkerAgentPaths:      1,
			WorkstationAgentPaths: 1,
			PromptPaths:           1,
		}, nil
	}}

	var out bytes.Buffer
	var diagnostics bytes.Buffer
	err := NewExpandFactoryConfig(persistence)(FactoryConfigExpandConfig{
		Path: inputPath, Output: &out, Verbose: true, Diagnostics: &diagnostics,
	})
	if err != nil {
		t.Fatalf("ExpandFactoryConfig: %v", err)
	}

	if got := out.String(); !strings.HasPrefix(got, "Expanded factory config into "+targetDir) {
		t.Fatalf("stdout = %q, want normal expand output", got)
	}
	got := diagnostics.String()
	for _, want := range []string{
		"config expand request",
		"inputPath=" + inputPath,
		"outputMode=filesystem",
		"config expand complete",
		"outputDir=" + targetDir,
		"writtenFactoryConfigs=1",
		"writtenWorkerAgents=1",
		"writtenWorkstationAgents=1",
		"writtenPromptFiles=1",
		"replacedBundledFiles=0",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("diagnostics missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Complete {{ .WorkID }} deterministically.") {
		t.Fatalf("diagnostics should not include prompt body:\n%s", got)
	}
}

func TestFlattenFactoryConfig_VerboseLogsInputAndOutputMetadata(t *testing.T) {
	inputPath := filepath.Join(t.TempDir(), "factory.json")
	if err := os.WriteFile(inputPath, []byte(`{"name":"fixture"}`), 0o600); err != nil {
		t.Fatalf("write input fixture: %v", err)
	}
	payload := []byte(`{"name":"canonical"}`)
	persistence := scriptedFactoryDefinitionPersistence{flatten: func(path string) ([]byte, error) {
		if path != inputPath {
			t.Fatalf("FlattenFactoryLayout path = %q, want %q", path, inputPath)
		}
		return payload, nil
	}}

	var out bytes.Buffer
	var diagnostics bytes.Buffer
	err := NewFlattenFactoryConfig(persistence)(FactoryConfigFlattenConfig{
		Path: inputPath, Output: &out, Verbose: true, Diagnostics: &diagnostics,
	})
	if err != nil {
		t.Fatalf("FlattenFactoryConfig: %v", err)
	}
	if !bytes.Equal(out.Bytes(), payload) {
		t.Fatalf("stdout = %q, want %q", out.Bytes(), payload)
	}
	got := diagnostics.String()
	for _, want := range []string{
		"config flatten request",
		"inputPath=" + inputPath,
		"outputMode=stdout",
		"config flatten complete",
		"outputBytes=",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("diagnostics missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "You are the expanded executor.") {
		t.Fatalf("diagnostics should not include worker body:\n%s", got)
	}
}

func TestFlattenFactoryConfig_VerboseLogsParseFailurePhase(t *testing.T) {
	inputPath := filepath.Join(t.TempDir(), "factory.json")
	persistence := scriptedFactoryDefinitionPersistence{flatten: func(path string) ([]byte, error) {
		if path != inputPath {
			t.Fatalf("FlattenFactoryLayout path = %q, want %q", path, inputPath)
		}
		return nil, errors.New("parse factory config: malformed JSON")
	}}

	var diagnostics bytes.Buffer
	err := NewFlattenFactoryConfig(persistence)(FactoryConfigFlattenConfig{
		Path: inputPath, Output: io.Discard, Verbose: true, Diagnostics: &diagnostics,
	})
	if err == nil {
		t.Fatal("expected invalid config to fail")
	}
	got := diagnostics.String()
	for _, want := range []string{"config flatten failed", "inputPath=" + inputPath, "phase=parse"} {
		if !strings.Contains(got, want) {
			t.Fatalf("diagnostics missing %q:\n%s", want, got)
		}
	}
}

func TestExpandFactoryConfig_ReportsReplacedPortableBundledFiles(t *testing.T) {
	inputPath := filepath.Join(t.TempDir(), "factory.json")
	persistence := scriptedFactoryDefinitionPersistence{expand: func(path string) (
		string,
		factorydefinitions.LayoutExpansionReport,
		error,
	) {
		if path != inputPath {
			t.Fatalf("ExpandFactoryLayout path = %q, want %q", path, inputPath)
		}
		return filepath.Dir(inputPath), factorydefinitions.LayoutExpansionReport{
			BundledReplacements: []factorydefinitions.PortableBundledFileReplacement{{
				TargetPath: "factory/scripts/execute-story.ps1",
			}},
		}, nil
	}}

	var out bytes.Buffer
	if err := NewExpandFactoryConfig(persistence)(FactoryConfigExpandConfig{
		Path: inputPath, Output: &out,
	}); err != nil {
		t.Fatalf("ExpandFactoryConfig: %v", err)
	}
	if !strings.Contains(out.String(), "Replaced existing portable bundled file at factory/scripts/execute-story.ps1") {
		t.Fatalf("expected portable bundled replacement report, got %q", out.String())
	}
}

func TestExpandFactoryConfig_InvalidPathReturnsContext(t *testing.T) {
	inputPath := filepath.Join(t.TempDir(), "missing-factory.json")
	persistence := scriptedFactoryDefinitionPersistence{expand: func(path string) (
		string,
		factorydefinitions.LayoutExpansionReport,
		error,
	) {
		if path != inputPath {
			t.Fatalf("ExpandFactoryLayout path = %q, want %q", path, inputPath)
		}
		return "", factorydefinitions.LayoutExpansionReport{},
			errors.New("find factory config source: file does not exist")
	}}
	err := NewExpandFactoryConfig(persistence)(FactoryConfigExpandConfig{
		Path: inputPath, Output: io.Discard,
	})
	if err == nil {
		t.Fatal("expected missing factory config path to fail")
	}
	if !strings.Contains(err.Error(), "find factory config source") {
		t.Fatalf("error = %q, want source path context", err.Error())
	}
}

func TestConfigLayoutOperationsRequireCobraOutput(t *testing.T) {
	persistence := scriptedFactoryDefinitionPersistence{}
	if err := NewFlattenFactoryConfig(persistence)(FactoryConfigFlattenConfig{}); err == nil ||
		!strings.Contains(err.Error(), "config flatten output is required") {
		t.Fatalf("flatten error = %v, want missing output", err)
	}
	if err := NewExpandFactoryConfig(persistence)(FactoryConfigExpandConfig{}); err == nil ||
		!strings.Contains(err.Error(), "config expand output is required") {
		t.Fatalf("expand error = %v, want missing output", err)
	}
}
