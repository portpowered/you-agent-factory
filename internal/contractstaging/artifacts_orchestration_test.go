package contractstaging

import (
	"bytes"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/contractjoiner"
	"github.com/portpowered/infinite-you/internal/contractvalidator"
)

func TestArtifactsWithDependenciesOrchestratesPipelineInExpectedOrder(t *testing.T) {
	root := t.TempDir()
	inputRoot, _ := filepath.Abs(filepath.Join(root, "..", "repo"))
	var callLog []string
	deps := ArtifactsDependencies{
		Join: func(input contractjoiner.Input) ([]contractjoiner.Document, []contractvalidator.Diagnostic) {
			callLog = append(callLog, "join")
			if input.RepositoryRoot != inputRoot {
				t.Fatalf("join input root = %q, want %q", input.RepositoryRoot, inputRoot)
			}
			return []contractjoiner.Document{
				{Path: "contracts/common/documentation.schema.json", Value: map[string]any{"title": "doc"}},
			}, nil
		},
		ReadRawArtifact: func(path string) ([]byte, error) {
			base := filepath.Base(path)
			callLog = append(callLog, "read:"+base)
			if base == filepath.Base(CanonicalOpenAPIPath) {
				return []byte("openapi"), nil
			}
			return []byte(base), nil
		},
		ProjectOpenAPI: func(canonical []byte, policy OpenAPIBytePolicy) ([]byte, error) {
			callLog = append(callLog, "projectOpenAPI")
			if !policy.AllowByteIdenticalCopy {
				return nil, errors.New("policy must allow copy")
			}
			return bytes.Clone(canonical), nil
		},
		GenerateSchema: func(_ string) ([]byte, error) {
			callLog = append(callLog, "generateSchema")
			return []byte("schema"), nil
		},
		GenerateStandaloneSchemas: func(_ string) (map[string][]byte, error) {
			callLog = append(callLog, "generateStandaloneSchemas")
			return map[string][]byte{
				factoryEventSchemaTarget:     []byte("event-schema"),
				factoryRecordingSchemaTarget: []byte("recording-schema"),
			}, nil
		},
		GenerateManifest: func(_ string, artifacts map[string][]byte) ([]byte, error) {
			callLog = append(callLog, "generateManifest")
			if _, ok := artifacts[factorySchemaTarget]; !ok {
				return nil, errors.New("missing schema")
			}
			return []byte("manifest"), nil
		},
	}

	artifacts, err := ArtifactsWithDependencies(inputRoot, deps)
	if err != nil {
		t.Fatalf("ArtifactsWithDependencies() error = %v", err)
	}
	if _, ok := artifacts[manifestTarget]; !ok {
		t.Fatal("manifest artifact missing from result")
	}
	expected := []string{
		"join",
		"read:openapi.yaml",
		"projectOpenAPI",
		"read:cli-commands.json",
		"read:mcp-tools.json",
		"read:you-config.schema.json",
		"read:mock-workers.schema.json",
		"read:runtime-api.json",
		"generateSchema",
		"generateStandaloneSchemas",
		"generateManifest",
	}
	if !reflect.DeepEqual(callLog, expected) {
		t.Fatalf("call log = %#v, want %#v", callLog, expected)
	}
}

func TestArtifactsWithDependencies_PropagatesJoinError(t *testing.T) {
	root := t.TempDir()
	_, err := ArtifactsWithDependencies(root, ArtifactsDependencies{
		Join: func(contractjoiner.Input) ([]contractjoiner.Document, []contractvalidator.Diagnostic) {
			return nil, []contractvalidator.Diagnostic{{Path: "/", Message: "join failed", Code: "join.fail"}}
		},
	})
	if err == nil || !strings.Contains(err.Error(), "join canonical contracts") {
		t.Fatalf("expected join failure, got %v", err)
	}
}

func TestArtifactsWithDependencies_PropagatesRawArtifactReadFailure(t *testing.T) {
	root := t.TempDir()
	_, err := ArtifactsWithDependencies(root, ArtifactsDependencies{
		Join: func(contractjoiner.Input) ([]contractjoiner.Document, []contractvalidator.Diagnostic) {
			return []contractjoiner.Document{}, nil
		},
		ReadRawArtifact: func(path string) ([]byte, error) {
			if filepath.Base(path) == filepath.Base(CanonicalOpenAPIPath) {
				return nil, errors.New("read failed")
			}
			return []byte("ok"), nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "read canonical raw artifact") {
		t.Fatalf("expected read failure, got %v", err)
	}
}

func TestArtifactsWithDependencies_PropagatesFactorySchemaFailure(t *testing.T) {
	root := t.TempDir()
	_, err := ArtifactsWithDependencies(root, ArtifactsDependencies{
		Join: func(contractjoiner.Input) ([]contractjoiner.Document, []contractvalidator.Diagnostic) {
			return []contractjoiner.Document{}, nil
		},
		ReadRawArtifact: func(string) ([]byte, error) { return []byte("raw"), nil },
		GenerateSchema:  func(string) ([]byte, error) { return nil, errors.New("schema failed") },
	})
	if err == nil || !strings.Contains(err.Error(), "schema failed") {
		t.Fatalf("expected schema failure, got %v", err)
	}
}

func TestArtifactsWithDependencies_PropagatesManifestFailure(t *testing.T) {
	root := t.TempDir()
	_, err := ArtifactsWithDependencies(root, ArtifactsDependencies{
		Join: func(contractjoiner.Input) ([]contractjoiner.Document, []contractvalidator.Diagnostic) {
			return []contractjoiner.Document{}, nil
		},
		ReadRawArtifact: func(string) ([]byte, error) { return []byte("raw"), nil },
		GenerateSchema:  func(string) ([]byte, error) { return []byte("schema"), nil },
		GenerateStandaloneSchemas: func(string) (map[string][]byte, error) {
			return map[string][]byte{}, nil
		},
		GenerateManifest: func(string, map[string][]byte) ([]byte, error) { return nil, errors.New("manifest failed") },
	})
	if err == nil || !strings.Contains(err.Error(), "manifest failed") {
		t.Fatalf("expected manifest failure, got %v", err)
	}
}
