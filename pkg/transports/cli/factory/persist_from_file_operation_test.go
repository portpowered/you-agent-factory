package factory

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestCreateFromFileInvokesOneInjectedFactoryDefinitionsOperation(t *testing.T) {
	t.Parallel()

	type contextKey struct{}
	ctx := context.WithValue(t.Context(), contextKey{}, "create")
	source := filepath.Join(t.TempDir(), "factory.json")
	if err := os.WriteFile(source, []byte("factory payload"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	calls := 0
	var output bytes.Buffer
	err := CreateFromFileWithServices(
		CreateFromFileConfig{
			Context:    ctx,
			Name:       "alpha",
			From:       source,
			Dir:        "factory-root",
			SetCurrent: true,
			Output:     &output,
		},
		func(
			gotContext context.Context,
			request factorydefinitions.NamedFactoryPersistenceRequest,
		) (factorydefinitions.NamedFactoryPersistenceResult, error) {
			calls++
			if gotContext != ctx {
				t.Fatal("context was not propagated")
			}
			if request.Mode != factorydefinitions.NamedFactoryPersistenceModeCreate ||
				request.RootDir != "factory-root" ||
				request.Name != "alpha" ||
				string(request.Payload) != "factory payload" ||
				!request.SetCurrent {
				t.Fatalf("request = %#v", request)
			}
			return factorydefinitions.NamedFactoryPersistenceResult{
				Name:       "alpha",
				FactoryDir: "factory-root/alpha",
			}, nil
		},
		factorydefinitions.AuthoredFactorySourceLoader(os.ReadFile),
	)
	if err != nil {
		t.Fatalf("CreateFromFileWithServices: %v", err)
	}
	if calls != 1 {
		t.Fatalf("operation calls = %d, want 1", calls)
	}
	if got := output.String(); got != "Created factory alpha\nDirectory: factory-root/alpha\n" {
		t.Fatalf("output = %q", got)
	}
}

func TestUpdateFromFileInvokesReplaceOperationAndRendersJSON(t *testing.T) {
	t.Parallel()

	source := filepath.Join(t.TempDir(), "factory.json")
	if err := os.WriteFile(source, []byte("{}"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	var output bytes.Buffer
	err := UpdateFromFileWithServices(
		UpdateFromFileConfig{
			Context: t.Context(),
			Name:    "@you/goal",
			From:    source,
			Dir:     "factory-root",
			JSON:    true,
			Output:  &output,
		},
		func(
			_ context.Context,
			request factorydefinitions.NamedFactoryPersistenceRequest,
		) (factorydefinitions.NamedFactoryPersistenceResult, error) {
			if request.Mode != factorydefinitions.NamedFactoryPersistenceModeReplace ||
				request.SetCurrent {
				t.Fatalf("request = %#v", request)
			}
			return factorydefinitions.NamedFactoryPersistenceResult{
				Name:       request.Name,
				FactoryDir: "factory-root/@you/goal",
			}, nil
		},
		factorydefinitions.AuthoredFactorySourceLoader(os.ReadFile),
	)
	if err != nil {
		t.Fatalf("UpdateFromFileWithServices: %v", err)
	}
	var result UpdateFromFileResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if result.Name != "@you/goal" || result.FactoryDir != "factory-root/@you/goal" {
		t.Fatalf("result = %#v", result)
	}
}

func TestPersistFromFileRequiresExplicitEdges(t *testing.T) {
	t.Parallel()

	source := filepath.Join(t.TempDir(), "factory.json")
	if err := os.WriteFile(source, []byte("{}"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	operation := factorydefinitions.NamedFactoryPersistenceOperation(
		func(
			context.Context,
			factorydefinitions.NamedFactoryPersistenceRequest,
		) (factorydefinitions.NamedFactoryPersistenceResult, error) {
			return factorydefinitions.NamedFactoryPersistenceResult{}, errors.New("unexpected call")
		},
	)
	for _, test := range []struct {
		name    string
		config  CreateFromFileConfig
		persist factorydefinitions.NamedFactoryPersistenceOperation
	}{
		{
			name:    "context",
			config:  CreateFromFileConfig{From: source, Output: &bytes.Buffer{}},
			persist: operation,
		},
		{
			name:   "operation",
			config: CreateFromFileConfig{Context: t.Context(), From: source, Output: &bytes.Buffer{}},
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			if err := CreateFromFileWithServices(test.config, test.persist, factorydefinitions.AuthoredFactorySourceLoader(os.ReadFile)); err == nil {
				t.Fatal("error = nil, want missing-edge failure")
			}
		})
	}
}
