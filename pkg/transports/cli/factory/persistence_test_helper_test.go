package factory

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// scriptedNamedFactoryPersistence keeps transport tests at the public Factory
// Definitions boundary. Persistence, validation, path mapping, and pointer
// behavior are covered by the owning service; these tests only observe the
// request sent to that service and its typed result or error.
func scriptedNamedFactoryPersistence(
	t *testing.T,
	result factorydefinitions.NamedFactoryPersistenceResult,
	operationErr error,
	inspect func(factorydefinitions.NamedFactoryPersistenceRequest),
) factorydefinitions.NamedFactoryPersistenceOperation {
	t.Helper()
	calls := 0
	t.Cleanup(func() {
		if calls != 1 {
			t.Errorf("Factory Definitions persistence calls = %d, want 1", calls)
		}
	})
	return func(
		_ context.Context,
		request factorydefinitions.NamedFactoryPersistenceRequest,
	) (factorydefinitions.NamedFactoryPersistenceResult, error) {
		calls++
		if inspect != nil {
			inspect(request)
		}
		return result, operationErr
	}
}

func createFromFileWithScriptedPersistence(
	t *testing.T,
	config CreateFromFileConfig,
	result factorydefinitions.NamedFactoryPersistenceResult,
	operationErr error,
	inspect func(factorydefinitions.NamedFactoryPersistenceRequest),
) error {
	t.Helper()
	if config.Context == nil {
		config.Context = t.Context()
	}
	return CreateFromFileWithServices(
		config,
		scriptedNamedFactoryPersistence(t, result, operationErr, inspect),
		readAuthoredTestSource,
	)
}

func updateFromFileWithScriptedPersistence(
	t *testing.T,
	config UpdateFromFileConfig,
	result factorydefinitions.NamedFactoryPersistenceResult,
	operationErr error,
	inspect func(factorydefinitions.NamedFactoryPersistenceRequest),
) error {
	t.Helper()
	if config.Context == nil {
		config.Context = t.Context()
	}
	return UpdateFromFileWithServices(
		config,
		scriptedNamedFactoryPersistence(t, result, operationErr, inspect),
		readAuthoredTestSource,
	)
}

func readAuthoredTestSource(
	path string,
) (factorydefinitions.AuthoredFactorySource, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return factorydefinitions.AuthoredFactorySource{}, err
	}
	format := factorydefinitions.AuthoredFactoryFormatJSON
	if filepath.Ext(path) != ".json" {
		format = factorydefinitions.AuthoredFactoryFormatYAML
	}
	return factorydefinitions.AuthoredFactorySource{
		Path:   path,
		Format: format,
		Data:   data,
	}, nil
}
