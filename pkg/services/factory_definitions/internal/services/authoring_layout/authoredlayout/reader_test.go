package authoredlayout

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestReaderOwnsSplitLayoutFilesystemReads(t *testing.T) {
	dir := t.TempDir()
	agentsPath := filepath.Join(dir, factorydefinitions.FactoryAgentsFileName)
	if err := os.WriteFile(agentsPath, []byte("worker bytes"), 0o644); err != nil {
		t.Fatalf("WriteFile(AGENTS.md): %v", err)
	}
	reader := NewReader(
		func(data []byte, sourcePath string) (*factorydefinitions.FactoryWorkerConfig, error) {
			return &factorydefinitions.FactoryWorkerConfig{
				Name: sourcePath,
				Body: string(data),
			}, nil
		},
		func(data []byte, sourcePath string) (*factorydefinitions.FactoryWorkstationConfig, error) {
			return &factorydefinitions.FactoryWorkstationConfig{
				Name: sourcePath,
				Body: string(data),
			}, nil
		},
		func(data []byte, _ string) (string, error) {
			return strings.ToUpper(string(data)), nil
		},
		localTestFileSystem{},
	)

	worker, err := reader.LoadWorkerConfig(dir)
	if err != nil {
		t.Fatalf("LoadWorkerConfig() error = %v", err)
	}
	if worker.Name != agentsPath || worker.Body != "worker bytes" {
		t.Fatalf("LoadWorkerConfig() = %#v", worker)
	}
	workstation, err := reader.LoadWorkstationConfig(dir)
	if err != nil {
		t.Fatalf("LoadWorkstationConfig() error = %v", err)
	}
	if workstation.Name != agentsPath || workstation.Body != "worker bytes" {
		t.Fatalf("LoadWorkstationConfig() = %#v", workstation)
	}
	body, found, err := reader.LoadWorkerBody(dir)
	if err != nil || !found || body != "WORKER BYTES" {
		t.Fatalf("LoadWorkerBody() = (%q, %t, %v)", body, found, err)
	}
	if !reader.SplitRuntimeEntityDirExists(dir) {
		t.Fatal("SplitRuntimeEntityDirExists() = false, want true")
	}
}

func TestReaderLoadsWorkstationPromptAndDistinguishesBodyOnlyContent(t *testing.T) {
	dir := t.TempDir()
	agentsPath := filepath.Join(dir, factorydefinitions.FactoryAgentsFileName)
	if err := os.WriteFile(agentsPath, []byte("body-only prompt"), 0o644); err != nil {
		t.Fatalf("WriteFile(AGENTS.md): %v", err)
	}
	reader := NewReader(nil, func([]byte, string) (*factorydefinitions.FactoryWorkstationConfig, error) {
		return &factorydefinitions.FactoryWorkstationConfig{PromptFile: "prompt.md"}, nil
	}, nil, localTestFileSystem{})
	if err := os.WriteFile(filepath.Join(dir, "prompt.md"), []byte("referenced prompt"), 0o644); err != nil {
		t.Fatalf("WriteFile(prompt.md): %v", err)
	}

	body, found, err := reader.LoadWorkstationBody(dir)
	if err != nil || !found || body != "body-only prompt" {
		t.Fatalf("LoadWorkstationBody() = (%q, %t, %v)", body, found, err)
	}
	workstation, err := reader.LoadWorkstationConfig(dir)
	if err != nil {
		t.Fatalf("LoadWorkstationConfig() error = %v", err)
	}
	if workstation.PromptTemplate != "referenced prompt" {
		t.Fatalf("PromptTemplate = %q, want referenced prompt", workstation.PromptTemplate)
	}
}

func TestReaderTreatsMissingOptionalBodiesAsAbsent(t *testing.T) {
	reader := NewReader(nil, nil, func([]byte, string) (string, error) {
		return "", errors.New("unexpected parse")
	}, localTestFileSystem{})
	dir := t.TempDir()

	if body, found, err := reader.LoadWorkerBody(dir); err != nil || found || body != "" {
		t.Fatalf("LoadWorkerBody() = (%q, %t, %v)", body, found, err)
	}
	if body, found, err := reader.LoadWorkstationBody(dir); err != nil || found || body != "" {
		t.Fatalf("LoadWorkstationBody() = (%q, %t, %v)", body, found, err)
	}
}

func TestReaderFailsClosedWithoutFileSystem(t *testing.T) {
	reader := NewReader(
		func([]byte, string) (*factorydefinitions.FactoryWorkerConfig, error) {
			return &factorydefinitions.FactoryWorkerConfig{}, nil
		},
		nil,
		nil,
		nil,
	)
	if _, err := reader.LoadWorkerConfig(t.TempDir()); err == nil ||
		!strings.Contains(err.Error(), "reader filesystem is required") {
		t.Fatalf("LoadWorkerConfig() error = %v", err)
	}
}
