package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/models"
	runtimescopeswire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes/wire"
)

func TestResolveModelReferenceAppliesOperatorPrecedenceAndDetachesResult(t *testing.T) {
	t.Parallel()

	digest := strings.Repeat("a", 40)
	source := "hf://operator/repository/model.gguf@" + digest
	backend := "operator-backend"
	loadPolicy := models.LoadPolicyOnDemand
	operations := []string{models.OperationTTS}
	overlays := map[string]models.ModelOverlay{
		" LLM ": {
			Source:     &source,
			Backend:    &backend,
			LoadPolicy: &loadPolicy,
			Operations: operations,
		},
	}

	result, err := resolveModelReference(
		context.Background(),
		models.ModelReference{NameOrURI: "llm"},
		overlays,
		nil,
	)
	if err != nil {
		t.Fatalf("resolveModelReference: %v", err)
	}
	if result.Definition.Name != models.BuiltInModelNameLLM || result.Definition.Backend != backend {
		t.Fatalf("definition identity/policy = %#v", result.Definition)
	}
	if result.Definition.Source != source {
		t.Fatalf("definition source = %q, want %q", result.Definition.Source, source)
	}
	if len(result.Definition.Operations) != 1 || result.Definition.Operations[0].Name != models.OperationTTS {
		t.Fatalf("definition operations = %#v, want TTS", result.Definition.Operations)
	}
	if result.Provenance.Kind != models.ModelReferenceSourceNamed ||
		result.Provenance.SourceKind != models.ModelReferenceSourceHuggingFace ||
		result.Provenance.ImmutableRevision != digest {
		t.Fatalf("provenance = %#v", result.Provenance)
	}

	updatedSource := "hf://operator/updated/model.gguf@" + digest
	*overlays[" LLM "].Source = updatedSource
	result.Definition.Operations[0].Name = "MUTATED"
	second, err := resolveModelReference(
		context.Background(),
		models.ModelReference{NameOrURI: "llm"},
		overlays,
		nil,
	)
	if err != nil {
		t.Fatalf("second resolveModelReference: %v", err)
	}
	if second.Definition.Source != updatedSource {
		t.Fatalf("second definition source = %q, want updated input source", second.Definition.Source)
	}
	if second.Definition.Operations[0].Name != models.OperationTTS {
		t.Fatalf("second definition operations = %#v, want detached TTS", second.Definition.Operations)
	}
}

func TestResolveModelReferenceAddsOperatorModel(t *testing.T) {
	t.Parallel()

	backend := "custom-backend"
	loadPolicy := models.LoadPolicyOnDemand
	result, err := resolveModelReference(
		context.Background(),
		models.ModelReference{NameOrURI: "custom-model"},
		map[string]models.ModelOverlay{
			"custom-model": {
				Source:     stringPointer("./custom-model.gguf"),
				Backend:    &backend,
				LoadPolicy: &loadPolicy,
				Operations: []string{models.OperationASR},
			},
		},
		nil,
	)
	if err != nil {
		t.Fatalf("resolveModelReference: %v", err)
	}
	if result.Definition.Name != "custom-model" || result.Definition.Backend != backend ||
		result.Definition.Source != "local://path" {
		t.Fatalf("definition = %#v", result.Definition)
	}
	if result.Provenance.Kind != models.ModelReferenceSourceNamed ||
		result.Provenance.SourceKind != models.ModelReferenceSourceLocalPath {
		t.Fatalf("provenance = %#v", result.Provenance)
	}
	if len(result.Definition.Operations) != 1 || result.Definition.Operations[0].Name != models.OperationASR {
		t.Fatalf("operations = %#v, want ASR", result.Definition.Operations)
	}
}

func TestResolveModelReferenceResolvesAllBuiltInsWithImmutableRevisionLookup(t *testing.T) {
	t.Parallel()

	digest := strings.Repeat("b", 64)
	var calls []string
	resolver := func(ctx context.Context, source string) (string, error) {
		calls = append(calls, source)
		return digest, nil
	}
	for _, definition := range (models.BuiltInCatalog{}).ModelDefinitions() {
		result, err := resolveModelReference(
			context.Background(),
			models.ModelReference{NameOrURI: definition.Name},
			nil,
			resolver,
		)
		if err != nil {
			t.Fatalf("resolve built-in %q: %v", definition.Name, err)
		}
		if result.Definition.Name != definition.Name || result.Readiness != models.ReadinessStateMissing {
			t.Fatalf("built-in %q result = %#v", definition.Name, result)
		}
		if result.Provenance.SourceKind != models.ModelReferenceSourceHuggingFace ||
			result.Provenance.ImmutableRevision == "" {
			t.Fatalf("built-in %q provenance = %#v", definition.Name, result.Provenance)
		}
	}
	if len(calls) != 3 {
		t.Fatalf("revision resolver calls = %d, want 3 for unpinned built-ins", len(calls))
	}
}

func TestResolveModelReferenceAcceptsSupportedSourceFormsWithoutLeakingPaths(t *testing.T) {
	t.Parallel()

	digest := strings.Repeat("c", 40)
	absolutePath := filepath.Join(t.TempDir(), "weights.gguf")
	uriPath := filepath.ToSlash(absolutePath)
	if !strings.HasPrefix(uriPath, "/") {
		uriPath = "/" + uriPath
	}
	fileURI := (&url.URL{Scheme: "file", Path: uriPath}).String()
	tests := []struct {
		name string
		ref  string
		kind models.ModelReferenceSourceKind
	}{
		{name: "relative bare", ref: "weights.gguf", kind: models.ModelReferenceSourceLocalPath},
		{name: "relative path", ref: "./weights.gguf", kind: models.ModelReferenceSourceLocalPath},
		{name: "absolute path", ref: absolutePath, kind: models.ModelReferenceSourceLocalPath},
		{name: "file URI", ref: fileURI, kind: models.ModelReferenceSourceFileURI},
		{
			name: "hugging face",
			ref:  "hf://owner/repository/model.gguf@" + digest,
			kind: models.ModelReferenceSourceHuggingFace,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := resolveModelReference(
				context.Background(), models.ModelReference{NameOrURI: test.ref}, nil, nil,
			)
			if err != nil {
				t.Fatalf("resolveModelReference: %v", err)
			}
			if result.Provenance.Kind != test.kind || result.Provenance.SourceKind != test.kind {
				t.Fatalf("provenance = %#v, want %q", result.Provenance, test.kind)
			}
			if strings.Contains(fmt.Sprintf("%#v", result), absolutePath) {
				t.Fatalf("resolved result leaked local path: %#v", result)
			}
		})
	}
}

func TestResolveModelReferenceResolvesUnpinnedRevisionAndPreservesProvenance(t *testing.T) {
	t.Parallel()

	immutable := strings.Repeat("d", 64)
	var requested string
	result, err := resolveModelReference(
		context.Background(),
		models.ModelReference{NameOrURI: "hf://owner/repository/model.gguf@main"},
		nil,
		func(ctx context.Context, source string) (string, error) {
			if err := ctx.Err(); err != nil {
				return "", err
			}
			requested = source
			return immutable, nil
		},
	)
	if err != nil {
		t.Fatalf("resolveModelReference: %v", err)
	}
	if requested != "hf://owner/repository/model.gguf@main" {
		t.Fatalf("revision lookup source = %q", requested)
	}
	if result.Provenance.Revision != "main" || result.Provenance.ImmutableRevision != immutable {
		t.Fatalf("provenance = %#v", result.Provenance)
	}
	if result.Definition.Source != "hf://owner/repository/model.gguf@"+immutable {
		t.Fatalf("resolved source = %q", result.Definition.Source)
	}
}

func TestResolveModelReferenceClassifiesUnknownAndInvalidEntries(t *testing.T) {
	t.Parallel()

	_, err := resolveModelReference(
		context.Background(), models.ModelReference{NameOrURI: "missing"},
		map[string]models.ModelOverlay{
			"custom": {Source: stringPointer("./custom.gguf"), Backend: stringPointer("backend"), LoadPolicy: loadPolicyPointer(), Operations: []string{models.OperationOMNI}},
		}, nil,
	)
	var unknown *models.InvocationFailure
	if !errors.As(err, &unknown) || !errors.Is(err, models.ErrModelReferenceUnknown) {
		t.Fatalf("unknown error = %v, want typed unknown reference", err)
	}
	if !reflect.DeepEqual(unknown.ValidNames, []string{"asr", "custom", "embed", "llm", "tts"}) {
		t.Fatalf("valid names = %#v", unknown.ValidNames)
	}

	_, err = resolveModelReference(
		context.Background(), models.ModelReference{NameOrURI: "broken"},
		map[string]models.ModelOverlay{"broken": {}}, nil,
	)
	var configuration models.ModelConfigurationFailure
	if !errors.As(err, &configuration) || configuration.ModelName != "broken" || configuration.Field != "source" {
		t.Fatalf("configuration error = %v, want broken/source", err)
	}
	if _, err = resolveModelReference(
		context.Background(), models.ModelReference{NameOrURI: models.BuiltInModelNameLLM},
		map[string]models.ModelOverlay{"broken": {}}, nil,
	); err != nil {
		t.Fatalf("unrelated valid built-in failed: %v", err)
	}

	_, err = resolveModelReference(
		context.Background(), models.ModelReference{NameOrURI: "bad name"},
		map[string]models.ModelOverlay{"bad name": {}}, nil,
	)
	if !errors.As(err, &configuration) || configuration.ModelName != "bad name" || configuration.Field != "name" {
		t.Fatalf("invalid name error = %v, want bad name/name", err)
	}

	_, err = resolveModelReference(
		context.Background(), models.ModelReference{NameOrURI: "http://private.example/model.gguf"}, nil, nil,
	)
	if !errors.Is(err, models.ErrModelReferenceInvalid) || strings.Contains(err.Error(), "private.example") {
		t.Fatalf("invalid source error = %v, want redacted typed error", err)
	}
}

func TestResolveModelReferenceHonorsCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	_, err := resolveModelReference(
		ctx,
		models.ModelReference{NameOrURI: "hf://owner/repository/model.gguf@main"}, nil,
		func(context.Context, string) (string, error) {
			called = true
			return strings.Repeat("e", 40), nil
		},
	)
	if !errors.Is(err, context.Canceled) || called {
		t.Fatalf("canceled resolve = (%v, called=%t), want context.Canceled without lookup", err, called)
	}
}

func TestRootResolveModelReferenceUsesDetachedScopeOverlay(t *testing.T) {
	t.Parallel()

	scopes, err := runtimescopeswire.NewService(func() string { return "model-resolution-scope-test" })
	if err != nil {
		t.Fatalf("runtime scopes: %v", err)
	}
	root := &Root{
		runtimeScopes: scopes,
		resolveHuggingFaceRevision: func(context.Context, string) (string, error) {
			return strings.Repeat("f", 40), nil
		},
	}
	source := "hf://operator/repository/model.gguf@" + strings.Repeat("a", 40)
	expectedSource := source
	backend := "detached-backend"
	overlays := map[string]models.ModelOverlay{
		"llm": {Source: &source, Backend: &backend, LoadPolicy: loadPolicyPointer(), Operations: []string{models.OperationOMNI}},
	}
	opened, err := root.OpenRuntimeScope(context.Background(), models.OpenRuntimeScopeRequest{
		Config: models.RuntimeScopeConfig{
			OperatorModels: overlays,
			Runtime:        models.RuntimeConfig{},
		},
	})
	if err != nil {
		t.Fatalf("OpenRuntimeScope: %v", err)
	}
	*overlays["llm"].Source = "http://mutated.example/model.gguf"
	result, err := root.ResolveModelReference(context.Background(), models.ResolveModelReferenceRequest{
		Scope:     opened.Scope,
		Reference: models.ModelReference{NameOrURI: "llm"},
	})
	if err != nil {
		t.Fatalf("ResolveModelReference: %v", err)
	}
	if result.Resolved.Definition.Source != expectedSource || result.Resolved.Definition.Backend != backend {
		t.Fatalf("resolved detached scope = %#v", result.Resolved)
	}
}

func stringPointer(value string) *string { return &value }

func loadPolicyPointer() *models.LoadPolicy {
	policy := models.LoadPolicyOnDemand
	return &policy
}
