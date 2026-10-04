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
	localmodels "github.com/portpowered/infinite-you/pkg/services/models/internal/local"
	scopedassets "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets"
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

func joinedInvocationContextError(ctx context.Context, err error) error {
	if err == nil || ctx == nil || ctx.Err() == nil || errors.Is(err, models.ErrInferenceCancelled) {
		return err
	}
	return errors.Join(models.ErrInferenceCancelled, err)
}

func joinedInvocationStart(o *Root) time.Time {
	if o != nil && o.now != nil {
		return o.now()
	}
	return time.Time{}
}

func joinedInvocationElapsed(o *Root, started time.Time) time.Duration {
	if o == nil || o.now == nil || started.IsZero() {
		return 0
	}
	ended := o.now()
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
	if o == nil || o.logger == nil {
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
		o.logger.Warn("models invocation stage", fields...)
		return
	}
	o.logger.Info("models invocation stage", fields...)
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
	if o == nil || o.logger == nil {
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
		o.logger.Warn("models invocation completed", fields...)
		return
	}
	fields = append(fields, zap.String("outcome", "COMPLETED"))
	o.logger.Info("models invocation completed", fields...)
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
	if strings.EqualFold(strings.TrimSpace(definition.Backend), "localai-qwen3-asr-cpp") {
		builtIn, _ := (models.BuiltInCatalog{}).ModelDefinitionFor(models.BuiltInModelNameASR)
		if strings.TrimSpace(source) == builtIn.Source {
			return []models.AssetRequirement{
				{Name: "qwen3-asr-0.6b-q8_0.gguf", Bytes: 1354082624, SHA256: "e777dacf2c23e4a3eafbec64e4e9d522b662c693c5189d4fd38ff39b92c9a334"},
				{Name: "qwen3-forced-aligner-0.6b-q8_0.gguf", Bytes: 994404608, SHA256: "5de69a8cfc49c95a6520f50f2f15cfce9af35bc4723a10c56fc64e51dc966b3a"},
			}, nil
		}
	}
	if strings.EqualFold(strings.TrimSpace(definition.Name), models.BuiltInModelNameQwen3TTSBase) &&
		strings.EqualFold(strings.TrimSpace(definition.Backend), "localai-qwen3-tts-cpp") {
		builtIn, _ := (models.BuiltInCatalog{}).ModelDefinitionFor(models.BuiltInModelNameQwen3TTSBase)
		if strings.TrimSpace(source) == builtIn.Source {
			return []models.AssetRequirement{
				{Name: "qwen-talker-0.6b-base-Q4_K_M.gguf", Bytes: 628905056, SHA256: "4b468ec7b1f62b90ef4ca316c0aa57deadfd54b2cf9651703ea753cedaf04226"},
				{Name: "qwen-tokenizer-12hz-Q4_K_M.gguf", Bytes: 254974752, SHA256: "cf3788b4d50aaa665fb6e57c170396aae03a3555fea52d2b5d0cda902d658039"},
			}, nil
		}
	}
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

func isJoinedManagedBackend(value string) bool {
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

func (o *Root) prepareJoinedGenericInvocation(
	ctx context.Context,
	request models.InvokeModelRequest,
	resolved models.ResolvedModelReference,
) (models.InvokeModelRequest, models.Operation, error) {
	definition, err := o.effectiveInvocationDefinition(ctx, request, resolved)
	if err != nil {
		return models.InvokeModelRequest{}, models.Operation{}, err
	}
	prepared, operation, err := models.PrepareGenericInvocation(request, definition)
	if err != nil {
		return models.InvokeModelRequest{}, models.Operation{}, err
	}
	if err := validateTTSReferenceCapability(prepared, definition); err != nil {
		return models.InvokeModelRequest{}, models.Operation{}, err
	}
	prepared.Operation = operation.Name
	if len(prepared.Inputs) > 0 && inferenceInputIsZero(prepared.Input) {
		prepared.Input = prepared.Inputs[0].Clone()
	}
	return prepared, operation, nil
}

func (o *Root) effectiveInvocationDefinition(
	ctx context.Context,
	request models.InvokeModelRequest,
	resolved models.ResolvedModelReference,
) (models.ModelDefinition, error) {
	definition := resolved.Definition.Clone()
	mediaSlot := genericRequestProjectorMediaSlot(request)
	if mediaSlot == "" ||
		!localmodels.VideoProjectorRequired(definition.Name, definition.Operations) ||
		o == nil || o.assets == nil {
		return definition, nil
	}
	inspection, err := o.assets.InspectRuntimeCache(ctx, models.InspectModelAssetsRequest{
		Scope: request.Scope,
		Name:  definition.Name,
	})
	if err != nil {
		if errors.Is(err, models.ErrUnsupportedOperation) {
			// Inert/unit collaborators may not expose cache inspection. Preserve
			// the pre-existing operation path for those compatibility seams.
			return definition, nil
		}
		if !isRemovableCacheAbsence(err) {
			return models.ModelDefinition{}, err
		}
		inspection = scopedassets.RuntimeCacheInspection{}
	}
	effective, _, unavailable := localmodels.ProjectEffectiveVideoDefinition(
		definition,
		localmodels.RuntimeCacheInspectionFromScoped(inspection),
	)
	if unavailable {
		return models.ModelDefinition{}, &models.InvocationFailure{
			Class:     models.InvocationFailureClassMediaCapability,
			Message:   mediaSlot + " input requires a verified projector artifact",
			Model:     request.Model,
			Operation: models.OperationOMNI,
			Slot:      mediaSlot,
			Cause:     models.ErrUnsupportedOperation,
		}
	}
	return effective, nil
}

func genericRequestProjectorMediaSlot(request models.InvokeModelRequest) string {
	inputs := request.Inputs
	if len(inputs) == 0 && !inferenceInputIsZero(request.Input) {
		inputs = []models.InferenceInput{request.Input}
	}
	for _, input := range inputs {
		for _, modality := range []models.Modality{models.ModalityImage, models.ModalityAudio, models.ModalityVideo} {
			if input.Modality == modality || strings.EqualFold(strings.TrimSpace(input.Name), string(modality)) {
				return strings.ToLower(string(modality))
			}
		}
	}
	return ""
}

func validateTTSReferenceCapability(request models.InvokeModelRequest, definition models.ModelDefinition) error {
	builtIn, _ := (models.BuiltInCatalog{}).ModelDefinitionFor(models.BuiltInModelNameTTS)
	if request.Operation != models.OperationTTS || definition.Backend != builtIn.Backend || definition.Source != builtIn.Source {
		return nil
	}
	for _, input := range request.Inputs {
		if input.Name == "voice" {
			return &models.InvocationFailure{
				Class:     models.InvocationFailureClassMediaCapability,
				Operation: models.OperationTTS, Model: request.Model, Slot: "voice",
				Message: "VibeVoice Realtime 0.5B does not support reference WAV voice cloning; choose qwen3-tts-base or configure a reference-capable model",
				Cause:   models.ErrUnsupportedOperation,
			}
		}
	}
	return nil
}
