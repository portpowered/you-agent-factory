package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"
	"sync/atomic"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelartifacts "github.com/portpowered/infinite-you/pkg/services/models/internal/artifacts"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	modelhost "github.com/portpowered/infinite-you/pkg/services/models/internal/legacyhost"
	localmodels "github.com/portpowered/infinite-you/pkg/services/models/internal/local"
	"go.uber.org/zap"
)

const (
	joinedLifecycleStageArtifactProvision = "ARTIFACT_PROVISION"
	joinedLifecycleStageBackendStart      = "BACKEND_START"
	joinedLifecycleStageHealth            = "HEALTH"
	joinedLifecycleStageInvoke            = "INVOKE"
	joinedLifecycleStageOutput            = "OUTPUT"
	joinedLifecycleStageRelease           = "RELEASE"
	joinedLifecycleStageTerminal          = "TERMINAL"
	joinedLifecycleOutcomeCompleted       = "COMPLETED"
	joinedLifecycleOutcomeFailed          = "FAILED"
)

// InspectRuntime returns invocation readiness for one model through the Models
// service boundary.
func (s *Service) InspectRuntime(ctx context.Context, modelName string) (models.Runtime, error) {
	if err := models.ValidateInspectRuntimeRequest(models.InspectRuntimeRequest{Name: modelName}); err != nil {
		return models.Runtime{}, err
	}
	runtimeCfg := s.runtimeConfig()
	if runtimeCfg == nil {
		return models.Runtime{}, fmt.Errorf("factory service runtime is not available")
	}
	host := s.modelHost()
	if host == nil {
		return localmodels.EnsureManagedRuntimeReadyForInvocation(
			runtimeCfg,
			modelName,
			nil,
			localmodels.DefaultManagedRuntimeSourceResolver(),
		)
	}
	snapshot, err := host.InspectReadiness(ctx, runtimeCfg, modelName)
	if err != nil {
		return models.Runtime{}, err
	}
	runtime := modelhost.ManagedRuntimeFromSnapshot(snapshot)
	if err := runtime.InvocationError(); err != nil {
		return runtime, err
	}
	return runtime, nil
}

func joinedInvocationContextError(ctx context.Context, err error) error {
	if err == nil || ctx == nil || ctx.Err() == nil || errors.Is(err, models.ErrInferenceCancelled) {
		return err
	}
	return errors.Join(models.ErrInferenceCancelled, err)
}

func joinedInvocationStart(o *Root) time.Time {
	if o != nil && o.process.Clock != nil {
		return o.process.Clock()
	}
	return time.Time{}
}

func joinedInvocationElapsed(o *Root, started time.Time) time.Duration {
	if o == nil || o.process.Clock == nil || started.IsZero() {
		return 0
	}
	ended := o.process.Clock()
	if ended.Before(started) {
		return 0
	}
	return ended.Sub(started)
}

func nextJoinedInvocationCorrelation(o *Root) string {
	if o == nil {
		return "models-invocation-0"
	}
	sequence := atomic.AddUint64(&o.correlationSequence, 1)
	return fmt.Sprintf("models-invocation-%d", sequence)
}

func joinedInvocationRevision(source string) string {
	_, revision, found := strings.Cut(strings.TrimSpace(source), "@")
	if !found {
		return ""
	}
	return strings.TrimSpace(revision)
}

func joinedInvocationLifecycleRecord(
	o *Root,
	modelName string,
	backend string,
	revision string,
	correlation string,
	operation string,
	invocation models.ModelInvocationRef,
	stage string,
	outcome string,
	elapsed time.Duration,
	err error,
) {
	if o == nil || o.process.Logger == nil {
		return
	}
	fields := []zap.Field{
		zap.String("stage", boundedLifecycleIdentity(stage)),
		zap.String("outcome", boundedLifecycleIdentity(outcome)),
		zap.Duration("duration", elapsed),
		zap.Int64("duration_millis", elapsed.Milliseconds()),
	}
	appendLifecycleIdentityField(&fields, "model_name", modelName)
	appendLifecycleIdentityField(&fields, "backend", backend)
	appendLifecycleIdentityField(&fields, "revision", revision)
	appendLifecycleIdentityField(&fields, "correlation_id", correlation)
	appendLifecycleIdentityField(&fields, "operation", operation)
	if invocationValue := boundedLifecycleIdentity(invocation.String()); invocationValue != "" {
		fields = append(fields, zap.String("invocation", invocationValue))
	}
	if err != nil {
		diagnostic := modelseffects.ProjectRuntimeFailure(
			modelseffects.WrapRuntimeFailure(modelseffects.RuntimeStageInvoke, err), elapsed,
		)
		fields = append(fields,
			zap.String("failure_class", string(diagnostic.Class)),
			zap.String("cause_sha256", diagnostic.CauseSHA256),
		)
		if diagnostic.Subcause != "" {
			fields = append(fields, zap.String("failure_subcause", string(diagnostic.Subcause)))
		}
		o.process.Logger.Warn("models invocation stage", fields...)
		return
	}
	o.process.Logger.Info("models invocation stage", fields...)
}

func appendLifecycleIdentityField(fields *[]zap.Field, key string, value string) {
	if fields == nil {
		return
	}
	if value = boundedLifecycleIdentity(value); value != "" {
		*fields = append(*fields, zap.String(key, value))
	}
}

func boundedLifecycleIdentity(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 128 {
		return ""
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			char == '_' || char == '-' || char == '.' {
			continue
		}
		return ""
	}
	return value
}

func joinedInvocationRecord(
	o *Root,
	modelName string,
	operation string,
	invocation models.ModelInvocationRef,
	backend string,
	revision string,
	correlation string,
	stage modelseffects.RuntimeStage,
	err error,
	elapsed time.Duration,
) {
	if o == nil || o.process.Logger == nil {
		return
	}
	fields := []zap.Field{
		zap.String("stage", joinedLifecycleStageTerminal),
		zap.String("evidence_kind", joinedLifecycleStageTerminal),
		zap.String("runtime_stage", string(stage)),
		zap.Duration("duration", elapsed),
		zap.Int64("duration_millis", elapsed.Milliseconds()),
	}
	appendLifecycleIdentityField(&fields, "model_name", modelName)
	appendLifecycleIdentityField(&fields, "operation", operation)
	appendLifecycleIdentityField(&fields, "invocation", invocation.String())
	appendLifecycleIdentityField(&fields, "backend", backend)
	appendLifecycleIdentityField(&fields, "revision", revision)
	appendLifecycleIdentityField(&fields, "correlation_id", correlation)
	if err != nil {
		diagnostic := modelseffects.ProjectRuntimeFailure(
			modelseffects.WrapRuntimeFailure(stage, err), elapsed,
		)
		fields = append(fields,
			zap.String("outcome", "FAILED"),
			zap.String("runtime_stage", string(diagnostic.Stage)),
			zap.String("failure_class", string(diagnostic.Class)),
			zap.String("cause_sha256", diagnostic.CauseSHA256),
		)
		if diagnostic.Subcause != "" {
			fields = append(fields, zap.String("failure_subcause", string(diagnostic.Subcause)))
		}
		o.process.Logger.Warn("models invocation completed", fields...)
		return
	}
	fields = append(fields, zap.String("outcome", "COMPLETED"))
	o.process.Logger.Info("models invocation completed", fields...)
}

func joinedAssetReference(
	reference models.ModelReference,
	resolved models.ResolvedModelReference,
) models.ModelReference {
	// Local source details are deliberately redacted from the resolved public
	// definition. Keep the original request at the asset boundary so the
	// private scope overlay can resolve the actual local path there.
	if resolved.Provenance.SourceKind == models.ModelReferenceSourceLocalPath ||
		resolved.Provenance.SourceKind == models.ModelReferenceSourceFileURI {
		return reference
	}
	resolvedSource := strings.TrimSpace(resolved.Definition.Source)
	if isJoinedSourceReference(resolvedSource) {
		return models.ModelReference{NameOrURI: resolved.Definition.Source}
	}
	return reference
}

func joinedAssetPreparationRequestWithBackend(
	request models.InvokeModelRequest,
	modelName string,
	resolved models.ResolvedModelReference,
	backendArtifact modelseffects.BackendArtifactSelection,
) (models.PrepareModelAssetsRequest, error) {
	configuration := modelseffects.ResolvedHostConfiguration{
		Scope:           request.Scope,
		ModelName:       modelName,
		Source:          joinedAssetReference(request.Model, resolved),
		Backend:         strings.TrimSpace(resolved.Definition.Backend),
		BackendArtifact: backendArtifact,
	}
	return joinedAssetPreparationRequestWithConfiguration(request, configuration, resolved)
}

func joinedAssetPreparationRequestWithConfiguration(
	request models.InvokeModelRequest,
	configuration modelseffects.ResolvedHostConfiguration,
	resolved models.ResolvedModelReference,
) (models.PrepareModelAssetsRequest, error) {
	assetReference := configuration.Source
	if assetReference.IsZero() {
		assetReference = joinedAssetReference(request.Model, resolved)
	}
	modelRequirements, err := joinedModelAssetRequirements(
		resolved.Definition, assetReference.NameOrURI,
	)
	if err != nil {
		return models.PrepareModelAssetsRequest{}, err
	}
	prepared := models.PrepareModelAssetsRequest{
		Scope:     configuration.Scope,
		Name:      configuration.ModelName,
		Reference: assetReference,
		Offline:   request.Offline,
		Backend:   configuration.Backend,
		Artifacts: modelRequirements,
	}
	if configuration.BackendArtifact.Name != "" {
		prepared.BackendReference = models.ModelReference{NameOrURI: configuration.BackendArtifact.Location}
		prepared.BackendArtifacts = []models.AssetRequirement{{
			Name:   configuration.BackendArtifact.Name,
			Bytes:  configuration.BackendArtifact.Bytes,
			SHA256: configuration.BackendArtifact.SHA256,
		}}
		return prepared, nil
	}
	if backend := strings.TrimSpace(configuration.Backend); isJoinedSourceReference(backend) {
		prepared.Backend = ""
		prepared.BackendReference = models.ModelReference{NameOrURI: backend}
		prepared.BackendArtifacts = joinedSourceAssetRequirements(backend)
	}
	return prepared, nil
}

func joinedModelAssetRequirements(
	definition models.ModelDefinition,
	source string,
) ([]models.AssetRequirement, error) {
	if !strings.EqualFold(strings.TrimSpace(definition.Name), models.BuiltInModelNameTTS) ||
		!strings.EqualFold(strings.TrimSpace(definition.Backend), "localai-vibevoice") {
		return joinedSourceAssetRequirements(source), nil
	}
	manifest, err := modelartifacts.DefaultModelRoleManifest()
	if err != nil {
		return nil, err
	}
	roleModel, ok := manifest.Model(models.BuiltInModelNameTTS)
	if !ok {
		return joinedSourceAssetRequirements(source), nil
	}
	if strings.TrimSpace(source) != roleModel.Source.URI {
		// Operator model overlays may point the built-in TTS model at a local
		// controlled bundle. Leave local/file sources unexpanded here so the
		// asset service can enumerate the directory and preserve all role files;
		// the pinned source below remains an explicit three-role contract.
		if isJoinedSourceReference(source) &&
			!strings.HasPrefix(strings.ToLower(strings.TrimSpace(source)), "hf://") {
			return nil, nil
		}
		return joinedSourceAssetRequirements(source), nil
	}
	requirements := make([]models.AssetRequirement, 0, len(roleModel.Artifacts))
	for _, role := range []string{"model", "tokenizer", "voice"} {
		artifact, ok := roleModel.Artifact(role)
		if !ok {
			return nil, fmt.Errorf("%w: missing TTS role %q", modelartifacts.ErrModelRoleManifestMalformed, role)
		}
		requirements = append(requirements, models.AssetRequirement{
			Name: artifact.Path, Bytes: artifact.SizeBytes, SHA256: artifact.SHA256,
		})
	}
	return requirements, nil
}

func isJoinedPinnedBackend(value string) bool {
	canonical := strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(canonical, "localai-") || canonical == "localai" ||
		canonical == "localai_grpc" || canonical == "localai-grpc"
}

func isJoinedSourceReference(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(lower, "hf://") || strings.HasPrefix(lower, "file://") ||
		strings.HasPrefix(lower, "./") || strings.HasPrefix(lower, "../") ||
		strings.HasPrefix(lower, "/") || strings.HasPrefix(lower, "\\") ||
		(len(lower) > 2 && lower[1] == ':')
}

func joinedSourceAssetRequirements(source string) []models.AssetRequirement {
	source = strings.TrimSpace(source)
	if source == "" {
		return nil
	}
	if strings.HasPrefix(strings.ToLower(source), "hf://") {
		rest := strings.TrimPrefix(source, "hf://")
		if at := strings.LastIndex(rest, "@"); at >= 0 {
			rest = rest[:at]
		}
		parts := strings.Split(rest, "/")
		if len(parts) > 2 {
			name := path.Clean(strings.Join(parts[2:], "/"))
			if name != "." && name != "" {
				return []models.AssetRequirement{{Name: name}}
			}
		}
		return nil
	}
	if strings.HasPrefix(strings.ToLower(source), "file://") {
		parsed, err := url.Parse(source)
		if err != nil || parsed.Path == "" {
			return nil
		}
		return []models.AssetRequirement{{Name: path.Base(parsed.Path)}}
	}
	if strings.Contains(source, "://") {
		return nil
	}
	return nil
}

func inferenceInputIsZero(input models.InferenceInput) bool {
	return input.Name == "" && input.Modality == "" && input.ContentType == "" &&
		input.MediaType == "" && input.Content == "" && input.Artifact == nil
}

func joinedInvocationLeaseReleased(result models.InvokeModelResult) bool {
	return result.LeaseDisposition == models.InvocationLeaseReleased ||
		result.LeaseDisposition == models.InvocationLeaseExpired
}

func (o *Root) releaseJoinedLease(
	ctx context.Context,
	scope models.RuntimeScopeRef,
	lease models.ModelLeaseRef,
) error {
	if o == nil || o.runtimeHost == nil || lease.IsZero() {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	releaseContext := context.WithoutCancel(ctx)
	modelseffects.MarkRuntimeLeaseReleaseAttempted(releaseContext)
	_, err := o.ReleaseModelLease(releaseContext, models.ReleaseModelLeaseRequest{
		Scope: scope, Lease: lease,
	})
	return err
}
