package service

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	modelhost "github.com/portpowered/infinite-you/pkg/services/models/internal/legacyhost"
	localmodels "github.com/portpowered/infinite-you/pkg/services/models/internal/local"
	runtimehost "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
	"go.uber.org/zap"
)

// ErrInvalidDependencies classifies model-service construction failures.
var ErrInvalidDependencies = errors.New("model service dependencies are invalid")

// Service owns model catalog, readiness, and pull behavior.
type Service struct {
	runtimeConfigLookup models.RuntimeConfigLoader
	host                modelhost.Host
	assetPuller         localmodels.AssetPuller
	loggerValue         *zap.Logger
	clock               func() time.Time
	pullMetrics         modelseffects.PullMetricsRecorder
}

// NewService constructs a model-domain service after validating every required
// collaborator. It applies only model-service-local defaults and performs no
// process-mode selection or application lifecycle work.
func NewService(
	runtimeConfig models.RuntimeConfigLoader,
	host modelhost.Host,
	assetPuller localmodels.AssetPuller,
	logger *zap.Logger,
	clock func() time.Time,
	pullMetrics modelseffects.PullMetricsRecorder,
) (*Service, error) {
	if runtimeConfig == nil {
		return nil, missingDependencyError("runtime configuration lookup")
	}
	if isNilDependency(host) {
		return nil, missingDependencyError("model host")
	}
	if isNilDependency(assetPuller) {
		return nil, missingDependencyError("model asset puller")
	}
	if logger == nil {
		return nil, missingDependencyError("logger")
	}
	if clock == nil {
		return nil, missingDependencyError("clock")
	}
	return &Service{
		runtimeConfigLookup: runtimeConfig,
		host:                host,
		assetPuller:         assetPuller,
		loggerValue:         logger,
		clock:               clock,
		pullMetrics:         pullMetrics,
	}, nil
}

func missingDependencyError(name string) error {
	return fmt.Errorf("%w: %s is required", ErrInvalidDependencies, name)
}

func isNilDependency(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

func (o *Root) CloseRuntimeScope(
	ctx context.Context,
	request models.CloseRuntimeScopeRequest,
) (models.CloseRuntimeScopeResult, error) {
	if o == nil || o.runtimeScopes == nil {
		return models.CloseRuntimeScopeResult{}, models.ErrUnsupportedOperation
	}
	if err := ctx.Err(); err != nil {
		return models.CloseRuntimeScopeResult{}, err
	}
	if request.Scope.IsZero() {
		return models.CloseRuntimeScopeResult{}, models.ErrRuntimeScopeInvalid
	}
	err := o.runtimeScopes.Close(runtimescopes.Reference(request.Scope.String()))
	if err != nil {
		return models.CloseRuntimeScopeResult{}, runtimeScopeError(err)
	}
	o.runtimeMu.Lock()
	delete(o.runtimeByScope, request.Scope)
	o.runtimeMu.Unlock()
	if closer, ok := o.runtimeHost.(interface {
		CloseRuntimeScope(context.Context, models.RuntimeScopeRef) error
	}); ok {
		if err := closer.CloseRuntimeScope(context.WithoutCancel(ctx), request.Scope); err != nil {
			return models.CloseRuntimeScopeResult{}, err
		}
	}
	return models.CloseRuntimeScopeResult{Scope: request.Scope, Closed: true}, nil
}

func (s *Service) runtimeConfig() *models.RuntimeConfig {
	if s == nil || s.runtimeConfigLookup == nil {
		return nil
	}
	return s.runtimeConfigLookup()
}

func (s *Service) modelHost() modelhost.Host {
	if s == nil {
		return nil
	}
	return s.host
}

func (s *Service) now() time.Time {
	return s.clock()
}

type modelSourceReference struct {
	Kind       models.ModelReferenceSourceKind
	SafeSource string
	LocalPath  string
	Owner      string
	Repository string
	File       string
	Revision   string
}

func (o *Root) ResolveModelReference(
	ctx context.Context,
	request models.ResolveModelReferenceRequest,
) (models.ResolveModelReferenceResult, error) {
	if o == nil || o.runtimeScopes == nil {
		return models.ResolveModelReferenceResult{}, models.ErrUnsupportedOperation
	}
	if err := request.Validate(); err != nil {
		return models.ResolveModelReferenceResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return models.ResolveModelReferenceResult{}, err
	}
	binding, err := o.runtimeScopes.Resolve(runtimescopes.Reference(request.Scope.String()))
	if err != nil {
		return models.ResolveModelReferenceResult{}, runtimeScopeError(err)
	}
	resolved, err := resolveModelReference(
		ctx,
		request.Reference,
		binding.OperatorModels,
		o.resolveHuggingFaceRevision,
	)
	if err != nil {
		return models.ResolveModelReferenceResult{}, err
	}
	return models.ResolveModelReferenceResult{Resolved: resolved}.Clone(), nil
}

// PreflightModelAssets resolves a name-only request into the same effective
// model/backend asset request used by joined invocation, then delegates the
// zero-body planning operation to Models Assets. Explicit asset requests are
// retained unchanged for callers that already performed resolution.
func (o *Root) PreflightModelAssets(
	ctx context.Context,
	request models.PrepareModelAssetsRequest,
) (models.PreflightModelAssetsResult, error) {
	if o == nil || o.assets == nil {
		return models.PreflightModelAssetsResult{}, models.ErrUnsupportedOperation
	}
	if err := request.Validate(); err != nil {
		return models.PreflightModelAssetsResult{}, err
	}
	prepared, err := o.normalizeAssetPreflightRequest(ctx, request)
	if err != nil {
		return models.PreflightModelAssetsResult{}, err
	}
	return o.assets.PreflightModelAssets(ctx, prepared)
}

func (o *Root) normalizeAssetPreflightRequest(
	ctx context.Context,
	request models.PrepareModelAssetsRequest,
) (models.PrepareModelAssetsRequest, error) {
	if !request.Reference.IsZero() || request.Artifacts != nil ||
		request.BackendArtifacts != nil || !request.BackendReference.IsZero() ||
		strings.TrimSpace(request.Backend) != "" {
		return request, nil
	}
	resolution, err := o.ResolveModelReference(ctx, models.ResolveModelReferenceRequest{
		Scope: request.Scope,
		Reference: models.ModelReference{
			NameOrURI: request.Name,
		},
	})
	if err != nil {
		return models.PrepareModelAssetsRequest{}, err
	}
	backendArtifact, err := o.resolveJoinedBackendArtifact(ctx, resolution.Resolved.Definition)
	if err != nil {
		return models.PrepareModelAssetsRequest{}, err
	}
	return joinedAssetPreparationRequestWithBackend(
		models.InvokeModelRequest{
			Scope:   request.Scope,
			Model:   models.ModelReference{NameOrURI: request.Name},
			Offline: request.Offline,
		},
		resolution.Resolved.Definition.Name,
		resolution.Resolved,
		backendArtifact,
	), nil
}

func joinedInvocationAssetError(
	request models.InvokeModelRequest,
	err error,
) error {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	class := models.InvocationFailureClassAssetPreparation
	message := "model asset preparation failed"
	switch {
	case errors.Is(err, models.ErrAssetBackendNotReady):
		class = models.InvocationFailureClassBackendReadiness
		message = "managed model backend is unavailable"
	case errors.Is(err, models.ErrAssetOffline):
		class = models.InvocationFailureClassOfflineCache
		message = "required model assets are unavailable offline"
	case errors.Is(err, models.ErrModelRevisionUnresolved):
		class = models.InvocationFailureClassRevisionResolution
		message = "model source revision could not be resolved to an immutable commit"
	}
	return &models.InvocationFailure{
		Class:     class,
		Message:   message,
		Model:     request.Model,
		Operation: strings.TrimSpace(request.Operation),
		Cause:     err,
	}
}

func resolveModelReference(
	ctx context.Context,
	reference models.ModelReference,
	overlays map[string]models.ModelOverlay,
	revisionResolver func(context.Context, string) (string, error),
) (models.ResolvedModelReference, error) {
	if err := ctx.Err(); err != nil {
		return models.ResolvedModelReference{}, err
	}
	raw := strings.TrimSpace(reference.NameOrURI)
	if strings.ContainsRune(raw, '\x00') {
		return models.ResolvedModelReference{}, invalidReferenceFailure()
	}
	if source, isSource, err := parseModelSource(raw); isSource {
		if err != nil {
			return models.ResolvedModelReference{}, err
		}
		return resolveSourceReference(ctx, source, revisionResolver)
	}

	canonicalName := strings.ToLower(raw)
	overlayName, overlay, hasOverlay, duplicateOverlay := findOverlay(overlays, canonicalName)
	builtIn, isBuiltIn := models.BuiltInCatalog{}.ModelDefinitionFor(canonicalName)
	if hasOverlay || isBuiltIn {
		if hasOverlay && !safeModelName(canonicalName) {
			return models.ResolvedModelReference{}, configurationFailure(
				overlayName, "name", "must contain only letters, digits, dots, hyphens, or underscores",
			)
		}
		if duplicateOverlay {
			return models.ResolvedModelReference{}, configurationFailure(
				overlayName, "name", "duplicates another entry after case and whitespace normalization",
			)
		}
		return resolveNamedReference(ctx, canonicalName, builtIn, isBuiltIn, overlay, hasOverlay, revisionResolver)
	}
	if looksLikeLocalPath(raw) {
		source := modelSourceReference{
			Kind:       models.ModelReferenceSourceLocalPath,
			SafeSource: "local://path",
			LocalPath:  raw,
		}
		return resolveSourceReference(ctx, source, revisionResolver)
	}
	return models.ResolvedModelReference{}, unknownModelFailure(overlays)
}

func resolveNamedReference(
	ctx context.Context,
	canonicalName string,
	builtIn models.ModelDefinition,
	isBuiltIn bool,
	overlay models.ModelOverlay,
	hasOverlay bool,
	revisionResolver func(context.Context, string) (string, error),
) (models.ResolvedModelReference, error) {
	definition := builtIn.Clone()
	if !isBuiltIn {
		definition = models.ModelDefinition{Name: canonicalName}
	}
	if hasOverlay {
		if err := validateOverlay(canonicalName, overlay, isBuiltIn); err != nil {
			return models.ResolvedModelReference{}, err
		}
		applyOverlay(&definition, overlay)
	}
	definition.Name = canonicalName
	if strings.TrimSpace(definition.Source) == "" {
		return models.ResolvedModelReference{}, configurationFailure(
			canonicalName, "source", "is required",
		)
	}
	source, err := parseConfiguredSource(definition.Source)
	if err != nil {
		if hasOverlay {
			return models.ResolvedModelReference{}, configurationFailure(
				canonicalName, "source", "uses an unsupported source reference",
			)
		}
		return models.ResolvedModelReference{}, err
	}
	resolved, err := resolveSourceReference(ctx, source, revisionResolver)
	if err != nil {
		return models.ResolvedModelReference{}, err
	}
	effectiveSource := resolved.Definition.Source
	resolved.Definition = definition.Clone()
	resolved.Definition.Source = effectiveSource
	resolved.Definition.Backend = strings.TrimSpace(definition.Backend)
	resolved.Provenance.Kind = models.ModelReferenceSourceNamed
	resolved.Provenance.Name = canonicalName
	return resolved.Clone(), nil
}

func applyOverlay(definition *models.ModelDefinition, overlay models.ModelOverlay) {
	if overlay.Source != nil {
		definition.Source = strings.TrimSpace(*overlay.Source)
	}
	if overlay.Backend != nil {
		definition.Backend = strings.TrimSpace(*overlay.Backend)
	}
	if overlay.LoadPolicy != nil {
		definition.LoadPolicy = models.LoadPolicy(strings.ToUpper(strings.TrimSpace(string(*overlay.LoadPolicy))))
	}
	if overlay.Operations != nil {
		definition.Operations = genericOperations(overlay.Operations)
	}
}

func validateOverlay(name string, overlay models.ModelOverlay, builtIn bool) error {
	if overlay.Source != nil && strings.TrimSpace(*overlay.Source) == "" {
		return configurationFailure(name, "source", "must be omitted or non-empty")
	}
	if overlay.Backend != nil && strings.TrimSpace(*overlay.Backend) == "" {
		return configurationFailure(name, "backend", "must be omitted or non-empty")
	}
	if overlay.LoadPolicy != nil {
		loadPolicy := strings.ToUpper(strings.TrimSpace(string(*overlay.LoadPolicy)))
		if loadPolicy != string(models.LoadPolicyOnDemand) && loadPolicy != string(models.LoadPolicyKeepWarm) {
			return configurationFailure(name, "loadPolicy", "must be ON_DEMAND or KEEP_WARM")
		}
	}
	if overlay.Operations != nil && len(overlay.Operations) == 0 {
		return configurationFailure(name, "operations", "must contain at least one operation")
	}
	for _, operation := range overlay.Operations {
		if _, ok := (models.GenericOperationCatalog{}).GenericOperationContract(operation); !ok {
			return configurationFailure(name, "operations", fmt.Sprintf("unsupported operation %q", operation))
		}
	}
	if !builtIn {
		for _, field := range []struct {
			name    string
			present bool
		}{
			{name: "source", present: overlay.Source != nil},
			{name: "backend", present: overlay.Backend != nil},
			{name: "loadPolicy", present: overlay.LoadPolicy != nil},
			{name: "operations", present: overlay.Operations != nil},
		} {
			if !field.present {
				return configurationFailure(name, field.name, "is required for a new model entry")
			}
		}
	}
	return nil
}

func genericOperations(names []string) []models.Operation {
	operations := make([]models.Operation, 0, len(names))
	for _, name := range names {
		operation, ok := models.GenericOperationCatalog{}.GenericOperationContract(name)
		if ok {
			operations = append(operations, operation)
		}
	}
	return operations
}

func resolveSourceReference(
	ctx context.Context,
	source modelSourceReference,
	revisionResolver func(context.Context, string) (string, error),
) (models.ResolvedModelReference, error) {
	if err := ctx.Err(); err != nil {
		return models.ResolvedModelReference{}, err
	}
	provenance := sourceProvenance(source)
	if source.Kind == models.ModelReferenceSourceHuggingFace {
		immutable, err := resolveImmutableRevision(ctx, source, revisionResolver)
		if err != nil {
			return models.ResolvedModelReference{}, err
		}
		provenance.ImmutableRevision = immutable
	}
	definition := models.ModelDefinition{
		Name:       sourceSafeName(source),
		Source:     source.SafeSource,
		Backend:    defaultBackendForSource(source),
		LoadPolicy: models.LoadPolicyOnDemand,
	}
	if source.Kind == models.ModelReferenceSourceHuggingFace && provenance.ImmutableRevision != "" {
		definition.Source = safeHuggingFaceSource(source, provenance.ImmutableRevision)
	}
	provenance.Name = definition.Name
	return models.ResolvedModelReference{
		Definition: definition,
		Provenance: provenance,
		Readiness:  models.ReadinessStateMissing,
	}.Clone(), nil
}

func resolveImmutableRevision(
	ctx context.Context,
	source modelSourceReference,
	revisionResolver func(context.Context, string) (string, error),
) (string, error) {
	if isImmutableRevision(source.Revision) {
		return source.Revision, nil
	}
	if revisionResolver == nil {
		return "", revisionFailure()
	}
	resolved, err := revisionResolver(ctx, source.SafeSource)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", revisionFailure()
	}
	if !safeRevision(resolved) {
		return "", revisionFailure()
	}
	return strings.TrimSpace(resolved), nil
}

func defaultHuggingFaceRevision(ctx context.Context, source string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	parsed, err := parseConfiguredSource(source)
	if err != nil || !isImmutableRevision(parsed.Revision) {
		return "", models.ErrModelRevisionUnresolved
	}
	return parsed.Revision, nil
}

func parseConfiguredSource(value string) (modelSourceReference, error) {
	source, isSource, err := parseModelSource(strings.TrimSpace(value))
	if !isSource || err != nil {
		return modelSourceReference{}, invalidReferenceFailure()
	}
	return source, err
}

func parseModelSource(value string) (modelSourceReference, bool, error) {
	switch {
	case strings.HasPrefix(strings.ToLower(value), "hf://"):
		source := parseHuggingFaceSource(value)
		if source.SafeSource == "" {
			return modelSourceReference{}, true, invalidReferenceFailure()
		}
		return source, true, nil
	case strings.HasPrefix(strings.ToLower(value), "file://"):
		source := parseFileSource(value)
		if source.SafeSource == "" {
			return modelSourceReference{}, true, invalidReferenceFailure()
		}
		return source, true, nil
	case strings.Contains(value, "://"):
		return modelSourceReference{}, true, invalidReferenceFailure()
	case looksLikeLocalPath(value):
		if strings.TrimSpace(value) == "" {
			return modelSourceReference{}, true, invalidReferenceFailure()
		}
		return modelSourceReference{
			Kind:       models.ModelReferenceSourceLocalPath,
			SafeSource: "local://path",
			LocalPath:  value,
		}, true, nil
	default:
		return modelSourceReference{}, false, nil
	}
}

func parseHuggingFaceSource(value string) modelSourceReference {
	invalid := modelSourceReference{Kind: models.ModelReferenceSourceHuggingFace}
	rest := strings.TrimSpace(value[len("hf://"):])
	if !validHuggingFaceReference(rest) {
		return invalid
	}
	base, revision, _ := splitHuggingFaceReference(rest)
	parts := strings.Split(base, "/")
	owner, repository := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	file := strings.Join(parts[2:], "/")
	safe := "hf://" + owner + "/" + repository
	if file != "" {
		safe += "/" + file
	}
	if revision != "" {
		safe += "@" + revision
	}
	return modelSourceReference{
		Kind:       models.ModelReferenceSourceHuggingFace,
		SafeSource: safe,
		Owner:      owner, Repository: repository, File: file, Revision: revision,
	}
}

func validHuggingFaceReference(rest string) bool {
	if rest == "" || strings.ContainsAny(rest, "\x00?#\\") {
		return false
	}
	base, revision, hasRevision := splitHuggingFaceReference(rest)
	parts := strings.Split(base, "/")
	if len(parts) < 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return false
	}
	if hasRevision && revision == "" {
		return false
	}
	for _, part := range parts {
		if invalidHuggingFacePathPart(part) {
			return false
		}
	}
	return !strings.ContainsAny(revision, "\x00?#\\@ \t\r\n")
}

func splitHuggingFaceReference(rest string) (base, revision string, hasRevision bool) {
	base = rest
	if at := strings.LastIndex(rest, "@"); at >= 0 {
		hasRevision = true
		base, revision = rest[:at], strings.TrimSpace(rest[at+1:])
	}
	return base, revision, hasRevision
}

func invalidHuggingFacePathPart(part string) bool {
	return strings.TrimSpace(part) == "" || part == "." || part == ".." ||
		strings.ContainsAny(part, " \t\r\n@")
}

func parseFileSource(value string) modelSourceReference {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "file" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return modelSourceReference{Kind: models.ModelReferenceSourceFileURI}
	}
	localPath := parsed.Path
	if parsed.Host != "" && !strings.EqualFold(parsed.Host, "localhost") {
		if len(parsed.Host) != 2 || parsed.Host[1] != ':' || !isASCIIAlpha(parsed.Host[0]) {
			return modelSourceReference{Kind: models.ModelReferenceSourceFileURI}
		}
		localPath = parsed.Host + localPath
	}
	if localPath == "" {
		return modelSourceReference{Kind: models.ModelReferenceSourceFileURI}
	}
	return modelSourceReference{
		Kind:       models.ModelReferenceSourceFileURI,
		SafeSource: "file://local",
		LocalPath:  localPath,
	}
}

func isASCIIAlpha(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z'
}

func safeHuggingFaceSource(source modelSourceReference, revision string) string {
	value := "hf://" + source.Owner + "/" + source.Repository
	if source.File != "" {
		value += "/" + source.File
	}
	return value + "@" + revision
}

func defaultBackendForSource(source modelSourceReference) string {
	if source.Kind != models.ModelReferenceSourceHuggingFace {
		return "auto"
	}
	if strings.HasSuffix(strings.ToLower(source.File), ".bin") {
		return "auto-audio"
	}
	return "auto"
}

func findOverlay(
	overlays map[string]models.ModelOverlay,
	canonicalName string,
) (string, models.ModelOverlay, bool, bool) {
	matching := make([]string, 0, 1)
	for rawName := range overlays {
		if strings.ToLower(strings.TrimSpace(rawName)) == canonicalName {
			matching = append(matching, rawName)
		}
	}
	if len(matching) == 0 {
		return "", models.ModelOverlay{}, false, false
	}
	sort.Strings(matching)
	return matching[0], overlays[matching[0]].Clone(), true, len(matching) > 1
}

func unknownModelFailure(overlays map[string]models.ModelOverlay) error {
	names := sortedModelNames(overlays)
	return &models.InvocationFailure{
		Class:      models.InvocationFailureClassInvalidModelReference,
		Message:    "unknown model name; valid names: " + strings.Join(names, ", "),
		ValidNames: names,
		Cause:      models.ErrModelReferenceUnknown,
	}
}

func invalidReferenceFailure() error {
	return &models.InvocationFailure{
		Class:   models.InvocationFailureClassInvalidModelReference,
		Message: "model reference must be a configured name, local path, file URI, or hf source",
		Cause:   models.ErrModelReferenceInvalid,
	}
}

func revisionFailure() error {
	return &models.InvocationFailure{
		Class:   models.InvocationFailureClassRevisionResolution,
		Message: "model source revision could not be resolved to an immutable commit",
		Cause:   models.ErrModelRevisionUnresolved,
	}
}

func configurationFailure(name, field, message string) error {
	return models.ModelConfigurationFailure{
		ModelName: name, Field: field, Message: message,
	}
}

func safeRevision(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && !strings.ContainsAny(value, "\x00/@\\?# ")
}

func isImmutableRevision(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func sourceProvenance(source modelSourceReference) models.ModelReferenceProvenance {
	return models.ModelReferenceProvenance{
		Kind:       source.Kind,
		SourceKind: source.Kind,
		Owner:      source.Owner,
		Repository: source.Repository,
		File:       source.File,
		Revision:   source.Revision,
	}
}

func sourceSafeName(source modelSourceReference) string {
	switch source.Kind {
	case models.ModelReferenceSourceHuggingFace:
		return source.Owner + "/" + source.Repository
	case models.ModelReferenceSourceLocalPath, models.ModelReferenceSourceFileURI:
		return "local-model"
	default:
		return ""
	}
}

func looksLikeLocalPath(value string) bool {
	return filepath.IsAbs(value) || strings.HasPrefix(value, "./") || strings.HasPrefix(value, "../") ||
		strings.HasPrefix(value, ".\\") || strings.HasPrefix(value, "..\\") ||
		strings.ContainsAny(value, `/\\`) || filepath.Ext(value) != ""
}

func sortedModelNames(overlays map[string]models.ModelOverlay) []string {
	names := []string{
		models.BuiltInModelNameASR,
		models.BuiltInModelNameEmbed,
		models.BuiltInModelNameLLM,
		models.BuiltInModelNameTTS,
	}
	seen := make(map[string]struct{}, len(names)+len(overlays))
	for index, name := range names {
		names[index] = strings.ToLower(name)
		seen[names[index]] = struct{}{}
	}
	for rawName := range overlays {
		name := strings.ToLower(strings.TrimSpace(rawName))
		if name == "" || !safeModelName(name) {
			continue
		}
		if _, exists := seen[name]; !exists {
			names = append(names, name)
			seen[name] = struct{}{}
		}
	}
	sort.Strings(names)
	return names
}

func safeModelName(value string) bool {
	for index, character := range value {
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			(index > 0 && (character == '.' || character == '_' || character == '-')) {
			continue
		}
		return false
	}
	return value != ""
}

func (o *Root) scopedRuntime(scope models.RuntimeScopeRef) (models.Service, error) {
	return o.scopedRuntimeWithBuilder(scope, func(binding models.RuntimeBinding) (models.Service, error) {
		assets, err := localmodels.NewScopedAssetPuller(o.assets, scope)
		if err != nil {
			return nil, err
		}
		return o.runtimeForBindingWithAssets(scope, binding, assets)
	})
}

func (o *Root) scopedRuntimeWithBuilder(
	scope models.RuntimeScopeRef,
	builder func(models.RuntimeBinding) (models.Service, error),
) (models.Service, error) {
	if o == nil || o.runtimeScopes == nil {
		return nil, models.ErrUnsupportedOperation
	}
	if scope.IsZero() {
		return nil, models.ErrRuntimeScopeInvalid
	}
	binding, err := o.runtimeScopes.Resolve(runtimescopes.Reference(scope.String()))
	if err != nil {
		return nil, runtimeScopeError(err)
	}
	o.runtimeMu.RLock()
	runtime := o.runtimeByScope[scope]
	o.runtimeMu.RUnlock()
	if runtime != nil {
		return runtime, nil
	}
	runtime, err = builder(binding)
	if err != nil {
		return nil, err
	}
	o.runtimeMu.Lock()
	if _, err := o.runtimeScopes.Resolve(runtimescopes.Reference(scope.String())); err != nil {
		o.runtimeMu.Unlock()
		return nil, runtimeScopeError(err)
	}
	if existing := o.runtimeByScope[scope]; existing != nil {
		runtime = existing
	} else {
		o.runtimeByScope[scope] = runtime
	}
	o.runtimeMu.Unlock()
	return runtime, nil
}

func newRuntimeWithHostEdges(
	scope models.RuntimeScopeRef,
	runtimeConfig models.RuntimeConfigLoader,
	logger *zap.Logger,
	now func() time.Time,
	pullMetrics modelseffects.PullMetricsRecorder,
	hostLogger modelseffects.HostDiagnosticLogger,
	hostMetrics modelseffects.HostMetricsRecorder,
	hooks modelseffects.LocalRuntimeHooks,
	assetPuller localmodels.AssetPuller,
	localRuntime localmodels.Runtime,
	runtimeHost runtimehost.Service,
	host modelhost.Host,
) (models.Service, error) {
	if assetPuller == nil {
		return nil, missingDependencyError("model asset puller")
	}
	if localRuntime == nil {
		return nil, missingDependencyError("local model runtime")
	}
	manager, err := localmodels.NewManagedRuntime(assetPuller, localRuntime, hooks, now)
	if err != nil {
		return nil, err
	}
	resources, err := localmodels.NewResourceLimiter(hooks, now)
	if err != nil {
		return nil, err
	}
	modelHost := host
	if modelHost == nil {
		gateway := modelhost.NewLocalAssetGateway(assetPuller)
		modelHost, err = modelhost.NewScopedCompatHost(
			scope,
			runtimeHost,
			gateway,
			modelhost.DefaultManagedRuntimeSourceResolverAdapter(),
			modelhost.Diagnostics{Logger: hostLogger, Metrics: hostMetrics},
		)
		if err != nil {
			return nil, err
		}
	}
	modelService, err := NewService(
		runtimeConfig,
		modelHost,
		assetPuller,
		logger,
		now,
		pullMetrics,
	)
	if err != nil {
		return nil, err
	}
	localExecutor, err := newLocalExecutor(
		runtimeConfig,
		modelHost,
		assetPuller,
		localRuntime,
		manager,
		resources,
		hooks,
		now,
	)
	if err != nil {
		return nil, err
	}
	return &runtimeService{Service: modelService, local: localExecutor}, nil
}

type runtimeService struct {
	*Service
	local *localExecutor
}

var _ models.Service = (*runtimeService)(nil)

func (s *runtimeService) OpenRuntimeScope(
	context.Context,
	models.OpenRuntimeScopeRequest,
) (models.OpenRuntimeScopeResult, error) {
	return models.OpenRuntimeScopeResult{}, models.ErrUnsupportedOperation
}

func (s *runtimeService) CloseRuntimeScope(
	context.Context,
	models.CloseRuntimeScopeRequest,
) (models.CloseRuntimeScopeResult, error) {
	return models.CloseRuntimeScopeResult{}, models.ErrUnsupportedOperation
}

func (s *runtimeService) PrepareModelAssets(
	context.Context,
	models.PrepareModelAssetsRequest,
) (models.PrepareModelAssetsResult, error) {
	return models.PrepareModelAssetsResult{}, models.ErrUnsupportedOperation
}

func (s *runtimeService) ResolveModelReference(
	context.Context,
	models.ResolveModelReferenceRequest,
) (models.ResolveModelReferenceResult, error) {
	return models.ResolveModelReferenceResult{}, models.ErrUnsupportedOperation
}

func (s *runtimeService) PullModelForScope(
	ctx context.Context,
	request models.PullModelRequest,
) (models.PullResult, error) {
	if err := models.ValidatePullModelRequest(request); err != nil {
		return models.PullResult{}, err
	}
	return s.PullModel(ctx, request.Name)
}

func (s *runtimeService) InspectModelAssets(
	context.Context,
	models.InspectModelAssetsRequest,
) (models.InspectModelAssetsResult, error) {
	return models.InspectModelAssetsResult{}, models.ErrUnsupportedOperation
}

func (s *runtimeService) RemoveModelAssets(
	context.Context,
	models.RemoveModelAssetsRequest,
) (models.RemoveModelAssetsResult, error) {
	return models.RemoveModelAssetsResult{}, models.ErrUnsupportedOperation
}

func (s *runtimeService) InvokeModel(
	context.Context,
	models.InvokeModelRequest,
) (models.InvokeModelResult, error) {
	return models.InvokeModelResult{}, models.ErrUnsupportedOperation
}

func (s *runtimeService) InvokeLocal(ctx context.Context, request models.LocalInvocationRequest) (models.LocalInvocationResult, error) {
	if err := models.ValidateLocalInvocationRequest(request); err != nil {
		return models.LocalInvocationResult{}, err
	}
	return s.local.InvokeLocal(ctx, request)
}
