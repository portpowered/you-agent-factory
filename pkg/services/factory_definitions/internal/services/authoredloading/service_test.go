package authoredloading

import (
	"context"
	"errors"
	"io/fs"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestLoadValidatedAuthoredFactoryDefinitionClassifiesFailures(t *testing.T) {
	t.Parallel()

	dependencyErr := errors.New("injected dependency failure")
	blockingResult := factorydefinitions.ValidationResult{Targets: []factorydefinitions.ValidationTarget{{
		Code:     "factory.test.blocking",
		Severity: factorydefinitions.ValidationSeverityError,
		Message:  "blocking test finding",
	}}}
	portableErr := factorydefinitions.ValidatePortableBundledFileType(
		factorydefinitions.BundledFileConfig{},
	)
	tests := []struct {
		name      string
		cause     error
		wantKind  factorydefinitions.AuthoredFactoryDefinitionLoadFailureKind
		wantMatch error
	}{
		{
			name:      "missing selected source",
			cause:     fs.ErrNotExist,
			wantKind:  factorydefinitions.AuthoredFactoryDefinitionLoadFailureMissing,
			wantMatch: factorydefinitions.ErrAuthoredFactoryDefinitionMissing,
		},
		{
			name: "missing supported directory root",
			cause: errors.Join(
				factorydefinitions.ErrAuthoredFactoryDefinitionMissing,
				errors.New("Factory Definition directory fixtures/alpha has no supported root"),
			),
			wantKind:  factorydefinitions.AuthoredFactoryDefinitionLoadFailureMissing,
			wantMatch: factorydefinitions.ErrAuthoredFactoryDefinitionMissing,
		},
		{
			name: "malformed authored configuration",
			cause: errors.Join(
				factorydefinitions.ErrAuthoredFactoryDefinitionMalformed,
				errors.New("decode Factory Definition fixtures/alpha/factory.yaml as YAML: malformed content"),
			),
			wantKind:  factorydefinitions.AuthoredFactoryDefinitionLoadFailureMalformed,
			wantMatch: factorydefinitions.ErrAuthoredFactoryDefinitionMalformed,
		},
		{
			name:      "unresolved portable content",
			cause:     portableErr,
			wantKind:  factorydefinitions.AuthoredFactoryDefinitionLoadFailureUnresolved,
			wantMatch: factorydefinitions.ErrAuthoredFactoryDefinitionUnresolved,
		},
		{
			name:      "blocking load validation",
			cause:     factorydefinitions.NewBlockingFactoryLoadError(blockingResult),
			wantKind:  factorydefinitions.AuthoredFactoryDefinitionLoadFailureValidation,
			wantMatch: factorydefinitions.ErrAuthoredFactoryDefinitionValidation,
		},
		{
			name:      "dependency failure",
			cause:     dependencyErr,
			wantKind:  factorydefinitions.AuthoredFactoryDefinitionLoadFailureDependency,
			wantMatch: factorydefinitions.ErrAuthoredFactoryDefinitionDependency,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			service := New(
				func(string, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
					return nil, test.cause
				},
				func(string, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
					return nil, test.cause
				},
				&blockingValidatorStub{},
			)
			result, err := service.LoadValidatedAuthoredFactoryDefinition(
				t.Context(),
				factorydefinitions.LoadValidatedAuthoredFactoryDefinitionRequest{
					SourcePath: "fixtures/alpha/factory.yaml",
				},
			)
			assertEmptyLoadResult(t, result)
			assertTypedLoadFailure(t, err, test.wantKind, test.wantMatch, test.cause)
			if test.wantKind == factorydefinitions.AuthoredFactoryDefinitionLoadFailureValidation &&
				len(loadFailureValidation(t, err).Targets) != 1 {
				t.Fatalf("validation targets = %#v, want the blocking finding", loadFailureValidation(t, err))
			}
		})
	}
}

func TestLoadValidatedAuthoredFactoryDefinitionPreservesCancellationAndNonBlockingFindings(t *testing.T) {
	t.Parallel()

	loaded := &loadedSourceStub{config: &factorydefinitions.FactoryConfig{Name: "alpha"}}
	warningAndHint := factorydefinitions.ValidationResult{Targets: []factorydefinitions.ValidationTarget{
		{Code: "factory.test.warning", Severity: factorydefinitions.ValidationSeverityWarning},
		{Code: "factory.test.hint", Severity: factorydefinitions.ValidationSeverityHint},
	}}
	service := New(
		func(string, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
			return loaded, nil
		},
		func(string, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
			return loaded, nil
		},
		&blockingValidatorStub{result: warningAndHint},
	)
	result, err := service.LoadValidatedAuthoredFactoryDefinition(
		t.Context(),
		factorydefinitions.LoadValidatedAuthoredFactoryDefinitionRequest{SourcePath: "fixtures/alpha/factory.yaml"},
	)
	if err != nil {
		t.Fatalf("LoadValidatedAuthoredFactoryDefinition warnings and hints: %v", err)
	}
	if result.Definition == nil || len(result.Validation.Targets) != 2 {
		t.Fatalf("non-blocking validation result = %#v", result)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	result, err = service.LoadValidatedAuthoredFactoryDefinition(
		ctx,
		factorydefinitions.LoadValidatedAuthoredFactoryDefinitionRequest{SourcePath: "fixtures/alpha/factory.yaml"},
	)
	assertEmptyLoadResult(t, result)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error = %v, want errors.Is context.Canceled", err)
	}
}

func assertTypedLoadFailure(
	t *testing.T,
	err error,
	wantKind factorydefinitions.AuthoredFactoryDefinitionLoadFailureKind,
	wantMatch error,
	wantCause error,
) {
	t.Helper()

	var failure *factorydefinitions.AuthoredFactoryDefinitionLoadFailure
	if !errors.As(err, &failure) {
		t.Fatalf("error = %T %v, want typed authored load failure", err, err)
	}
	if failure.Kind != wantKind || failure.Source.Path != "fixtures/alpha/factory.yaml" {
		t.Fatalf("typed failure = %#v, want kind=%q with selected source", failure, wantKind)
	}
	if !errors.Is(err, wantMatch) || !errors.Is(err, wantCause) {
		t.Fatalf("error = %v, want classification %v and cause %v", err, wantMatch, wantCause)
	}
	if causeText := wantCause.Error(); causeText != "" && !strings.Contains(err.Error(), causeText) {
		t.Fatalf("error = %q, want retained cause diagnostic %q", err, causeText)
	}
}

func loadFailureValidation(
	t *testing.T,
	err error,
) factorydefinitions.ValidationResult {
	t.Helper()
	var failure *factorydefinitions.AuthoredFactoryDefinitionLoadFailure
	if !errors.As(err, &failure) {
		t.Fatalf("error = %T %v, want typed authored load failure", err, err)
	}
	return failure.Validation
}

func assertEmptyLoadResult(
	t *testing.T,
	result factorydefinitions.LoadValidatedAuthoredFactoryDefinitionResult,
) {
	t.Helper()

	if result.Source != (factorydefinitions.AuthoredFactoryDefinitionIdentity{}) ||
		result.Definition != nil || result.FactoryDir != "" || result.RuntimeBaseDir != "" ||
		len(result.BundledFileReplacements) != 0 || len(result.Validation.Targets) != 0 {
		t.Fatalf("failure returned partial result: %#v", result)
	}
}

func TestLoadValidatedAuthoredFactoryDefinitionReturnsDetachedEffectiveFacts(t *testing.T) {
	t.Parallel()

	loaded := &loadedSourceStub{
		factoryDir:     "fixtures/alpha",
		runtimeBaseDir: "fixtures/alpha",
		config: &factorydefinitions.FactoryConfig{
			Name:    "alpha",
			Workers: []factorydefinitions.FactoryWorkerConfig{{Name: "planner", Body: "original"}},
		},
		bundled: []factorydefinitions.PortableBundledFileReplacement{{TargetPath: "docs/guide.md"}},
	}
	var currentCalls, selectedCalls int
	validator := &blockingValidatorStub{}
	service := New(
		func(string, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
			currentCalls++
			return loaded, nil
		},
		func(string, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
			selectedCalls++
			return loaded, nil
		},
		validator,
	)

	result, err := service.LoadValidatedAuthoredFactoryDefinition(
		t.Context(),
		factorydefinitions.LoadValidatedAuthoredFactoryDefinitionRequest{
			SourcePath:       "fixtures/alpha/factory.yaml",
			ExecutionBaseDir: "execution",
		},
	)
	if err != nil {
		t.Fatalf("LoadValidatedAuthoredFactoryDefinition: %v", err)
	}
	if currentCalls != 0 || selectedCalls != 1 || validator.calls != 1 {
		t.Fatalf(
			"loading calls current=%d selected=%d validation=%d, want 0,1,1",
			currentCalls,
			selectedCalls,
			validator.calls,
		)
	}
	if result.Source.Path != "fixtures/alpha/factory.yaml" ||
		result.Source.Format != factorydefinitions.AuthoredFactoryFormatYAML ||
		result.FactoryDir != "fixtures/alpha" || result.RuntimeBaseDir != "execution" {
		t.Fatalf("result identity facts = %#v", result)
	}
	if result.Definition == nil || result.Definition.Workers[0].Body != "original" ||
		len(result.BundledFileReplacements) != 1 {
		t.Fatalf("result effective facts = %#v", result)
	}

	result.Definition.Workers[0].Body = "caller mutation"
	result.BundledFileReplacements[0].TargetPath = "caller.md"
	if loaded.config.Workers[0].Body != "original" || loaded.bundled[0].TargetPath != "docs/guide.md" {
		t.Fatal("returned facts mutated the loader-owned effective source")
	}

	later, err := service.LoadValidatedAuthoredFactoryDefinition(
		t.Context(),
		factorydefinitions.LoadValidatedAuthoredFactoryDefinitionRequest{
			SourcePath: "fixtures/alpha/factory.yaml",
		},
	)
	if err != nil {
		t.Fatalf("LoadValidatedAuthoredFactoryDefinition(second call): %v", err)
	}
	if later.Definition.Workers[0].Body != "original" ||
		later.BundledFileReplacements[0].TargetPath != "docs/guide.md" {
		t.Fatalf("later result retained caller mutation: %#v", later)
	}
}

type loadedSourceStub struct {
	factorydefinitions.MutableLoadedFactorySource
	factoryDir     string
	runtimeBaseDir string
	config         *factorydefinitions.FactoryConfig
	bundled        []factorydefinitions.PortableBundledFileReplacement
}

func (s *loadedSourceStub) FactoryDir() string { return s.factoryDir }

func (s *loadedSourceStub) RuntimeBaseDir() string { return s.runtimeBaseDir }

func (s *loadedSourceStub) FactoryConfig() *factorydefinitions.FactoryConfig { return s.config }

func (s *loadedSourceStub) PortableBundledFileReplacements() []factorydefinitions.PortableBundledFileReplacement {
	return s.bundled
}

type blockingValidatorStub struct {
	factorydefinitions.Validator
	calls  int
	result factorydefinitions.ValidationResult
}

func (s *blockingValidatorStub) ValidateBlockingLoad(
	context.Context,
	*factorydefinitions.FactoryConfig,
) factorydefinitions.ValidationResult {
	s.calls++
	return s.result
}
