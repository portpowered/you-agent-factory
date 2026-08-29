// Package wire is the Models service composition boundary.
//
// Wire performs construction only, returns the singular models.Service root
// interface, and starts no lifecycle components. Parent-private runtime_scopes,
// catalog, assets, runtime_host, and inference owner wiring stays inside the
// owner service assembly path; peers depend on models.Service rather than owner
// internals or construction ports.
package wire

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	platformgrpc "github.com/portpowered/infinite-you/pkg/platform/grpc"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	platformrandom "github.com/portpowered/infinite-you/pkg/platform/random"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	localai "github.com/portpowered/infinite-you/pkg/services/models/internal/backends/localai"
	modelcodecs "github.com/portpowered/infinite-you/pkg/services/models/internal/backends/localai/codecs"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	modelhost "github.com/portpowered/infinite-you/pkg/services/models/internal/legacyhost"
	localmodels "github.com/portpowered/infinite-you/pkg/services/models/internal/local"
	modelsruntime "github.com/portpowered/infinite-you/pkg/services/models/internal/runtime"
	modelsservice "github.com/portpowered/infinite-you/pkg/services/models/internal/service"
	scopedassets "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets"
	assetswire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets/wire"
	catalog "github.com/portpowered/infinite-you/pkg/services/models/internal/services/catalog"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/catalog/wire"
	inference "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference"
	inferencewire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference/wire"
	runtimehost "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host"
	runtimehostwire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host/wire"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
	runtimescopeswire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes/wire"
	"go.uber.org/zap"
)

const (
	defaultAssetBaseURL    = "https://huggingface.co"
	defaultAssetAPIBaseURL = "https://huggingface.co/api"
)

// InvocationBackend is the narrow external effect used to execute one
// prepared generic request. The Models service owns preparation, lifecycle,
// output normalization, and lease release; an injected backend supplies only
// detached provider-neutral output facts.
type InvocationBackend func(
	context.Context,
	models.InvokeModelRequest,
) ([]models.InferenceContent, []models.InferenceArtifact, error)

// ASRBackend is the typed external effect used by the Models-owned ASR
// codec/runtime. Its request and response remain provider-neutral at this
// construction boundary.
type ASRBackend func(
	context.Context,
	models.ASRBackendRequest,
) (models.ASRBackendResponse, error)

// EmbeddingBackend is the typed external effect used by the Models-owned
// EMBED codec/runtime. Its request and response remain provider-neutral at the
// construction boundary.
type EmbeddingBackend func(
	context.Context,
	models.EmbeddingBackendRequest,
) (models.EmbeddingBackendResponse, error)

// InvocationProtocolClient is the provider-neutral construction port for the
// pinned generic protocol. Protocol-native request types stay inside the
// private LocalAI adapter.
type InvocationProtocolClient interface {
	Predict(context.Context, models.InvocationProtocolRequest) (models.InvocationProtocolResponse, error)
}

// InvocationProtocolDialer is the policy-free transport port used by the
// pinned LocalAI adapter when no provider-neutral fixture client is supplied.
type InvocationProtocolDialer = platformgrpc.Dialer

// NewPinnedGRPCHostProtocolNegotiator exposes the LocalAI-owned readiness
// adapter through the Models construction boundary without exporting any
// backend-native message types.
func NewPinnedGRPCHostProtocolNegotiator(
	dialer InvocationProtocolDialer,
) HostProtocolNegotiator {
	return localai.NewPinnedGRPCHostProtocolNegotiator(dialer)
}

type invocationRuntimeOptions struct {
	Backend   InvocationBackend
	ASR       ASRBackend
	Embedding EmbeddingBackend
	Client    InvocationProtocolClient
	Dialer    InvocationProtocolDialer
}

type invocationRuntime interface {
	Invoke(context.Context, inference.InvocationRuntimeRequest) (inference.InvocationRuntimeResult, error)
}

// NewService constructs an inert Models root from construction and process-edge
// ports. It composes the accepted root through parent-private runtime_scopes,
// catalog, assets, runtime_host, and inference owner construction without
// publishing owner types on the returned peer surface. Missing required
// construction ports fail with a deterministic construction error and a nil
// service.
func NewService(
	assetPlatform models.AssetHostPlatform,
	assetHTTP AssetHTTPDoer,
	assetEndpoints models.RuntimeAssetEndpoints,
	assetMkdirAll AssetMakeDirectories,
	assetStat AssetInspectPath,
	assetHome AssetResolveHomeDirectory,
	assetWriteFile AssetWriteFile,
	assetRename AssetRenamePath,
	assetRemove AssetRemovePath,
	assetReadFile AssetReadFile,
	assetReadDir AssetReadDirectory,
	assetCreate AssetCreateFile,
	assetOpen AssetOpenFile,
	processLauncher HostProcessLauncher,
	hostHTTP HostHTTPDoer,
	hostClock HostClock,
	runtimeRunner platformprocess.CommandRunner,
	runtimeHTTP RuntimeHTTPDoer,
	runtimeInspect RuntimeInspectFile,
	runtimeTempDir RuntimeTempDirectory,
	runtimeTempFile RuntimeCreateTempFile,
	logger *zap.Logger,
	now func() time.Time,
	issuerEntropy platformrandom.Source,
	pullMetrics PullMetricsRecorder,
	hostLogger HostDiagnosticLogger,
	hostMetrics HostMetricsRecorder,
	localHooks LocalRuntimeHooks,
	resolveEnvironment AssetResolveEnvironment,
	protocolNegotiator HostProtocolNegotiator,
	compatibilityChecker HostCompatibilityChecker,
	assetCoordination AssetStagingCoordination,
	revisionResolvers ...func(context.Context, string) (string, error),
) (models.Service, error) {
	return newService(
		assetPlatform, assetHTTP, assetEndpoints, assetMkdirAll, assetStat, assetHome,
		assetWriteFile, assetRename, assetRemove, assetReadFile, assetReadDir,
		assetCreate, assetOpen, processLauncher, hostHTTP, hostClock, runtimeRunner,
		runtimeHTTP, runtimeInspect, runtimeTempDir, runtimeTempFile, logger, now,
		issuerEntropy, pullMetrics, hostLogger, hostMetrics, localHooks,
		resolveEnvironment, protocolNegotiator, compatibilityChecker, assetCoordination, nil, invocationRuntimeOptions{},
		revisionResolvers...,
	)
}

// NewServiceWithBackendArtifactResolver constructs the Models root with the
// exact pinned backend selector used by the joined invocation path.
func NewServiceWithBackendArtifactResolver(
	assetPlatform models.AssetHostPlatform,
	assetHTTP AssetHTTPDoer,
	assetEndpoints models.RuntimeAssetEndpoints,
	assetMkdirAll AssetMakeDirectories,
	assetStat AssetInspectPath,
	assetHome AssetResolveHomeDirectory,
	assetWriteFile AssetWriteFile,
	assetRename AssetRenamePath,
	assetRemove AssetRemovePath,
	assetReadFile AssetReadFile,
	assetReadDir AssetReadDirectory,
	assetCreate AssetCreateFile,
	assetOpen AssetOpenFile,
	processLauncher HostProcessLauncher,
	hostHTTP HostHTTPDoer,
	hostClock HostClock,
	runtimeRunner platformprocess.CommandRunner,
	runtimeHTTP RuntimeHTTPDoer,
	runtimeInspect RuntimeInspectFile,
	runtimeTempDir RuntimeTempDirectory,
	runtimeTempFile RuntimeCreateTempFile,
	logger *zap.Logger,
	now func() time.Time,
	issuerEntropy platformrandom.Source,
	pullMetrics PullMetricsRecorder,
	hostLogger HostDiagnosticLogger,
	hostMetrics HostMetricsRecorder,
	localHooks LocalRuntimeHooks,
	resolveEnvironment AssetResolveEnvironment,
	protocolNegotiator HostProtocolNegotiator,
	compatibilityChecker HostCompatibilityChecker,
	assetCoordination AssetStagingCoordination,
	backendResolver BackendArtifactResolver,
	revisionResolvers ...func(context.Context, string) (string, error),
) (models.Service, error) {
	return newService(
		assetPlatform, assetHTTP, assetEndpoints, assetMkdirAll, assetStat, assetHome,
		assetWriteFile, assetRename, assetRemove, assetReadFile, assetReadDir,
		assetCreate, assetOpen, processLauncher, hostHTTP, hostClock, runtimeRunner,
		runtimeHTTP, runtimeInspect, runtimeTempDir, runtimeTempFile, logger, now,
		issuerEntropy, pullMetrics, hostLogger, hostMetrics, localHooks,
		resolveEnvironment, protocolNegotiator, compatibilityChecker, assetCoordination, backendResolver, invocationRuntimeOptions{},
		revisionResolvers...,
	)
}

// NewServiceWithBackendArtifactResolverAndInvocationProtocolAndDialer adds
// the production transport used by the pinned LocalAI adapter while retaining
// the injected generic and ASR backends used by deterministic fixtures.
func NewServiceWithBackendArtifactResolverAndInvocationProtocolAndDialer(
	assetPlatform models.AssetHostPlatform,
	assetHTTP AssetHTTPDoer,
	assetEndpoints models.RuntimeAssetEndpoints,
	assetMkdirAll AssetMakeDirectories,
	assetStat AssetInspectPath,
	assetHome AssetResolveHomeDirectory,
	assetWriteFile AssetWriteFile,
	assetRename AssetRenamePath,
	assetRemove AssetRemovePath,
	assetReadFile AssetReadFile,
	assetReadDir AssetReadDirectory,
	assetCreate AssetCreateFile,
	assetOpen AssetOpenFile,
	processLauncher HostProcessLauncher,
	hostHTTP HostHTTPDoer,
	hostClock HostClock,
	runtimeRunner platformprocess.CommandRunner,
	runtimeHTTP RuntimeHTTPDoer,
	runtimeInspect RuntimeInspectFile,
	runtimeTempDir RuntimeTempDirectory,
	runtimeTempFile RuntimeCreateTempFile,
	logger *zap.Logger,
	now func() time.Time,
	issuerEntropy platformrandom.Source,
	pullMetrics PullMetricsRecorder,
	hostLogger HostDiagnosticLogger,
	hostMetrics HostMetricsRecorder,
	localHooks LocalRuntimeHooks,
	resolveEnvironment AssetResolveEnvironment,
	protocolNegotiator HostProtocolNegotiator,
	compatibilityChecker HostCompatibilityChecker,
	assetCoordination AssetStagingCoordination,
	backendResolver BackendArtifactResolver,
	invocationProtocol InvocationProtocolClient,
	protocolDialer InvocationProtocolDialer,
	invocationBackend InvocationBackend,
	asrBackend ASRBackend,
	embeddingBackend EmbeddingBackend,
	revisionResolvers ...func(context.Context, string) (string, error),
) (models.Service, error) {
	return newService(
		assetPlatform, assetHTTP, assetEndpoints, assetMkdirAll, assetStat, assetHome,
		assetWriteFile, assetRename, assetRemove, assetReadFile, assetReadDir,
		assetCreate, assetOpen, processLauncher, hostHTTP, hostClock, runtimeRunner,
		runtimeHTTP, runtimeInspect, runtimeTempDir, runtimeTempFile, logger, now,
		issuerEntropy, pullMetrics, hostLogger, hostMetrics, localHooks,
		resolveEnvironment, protocolNegotiator, compatibilityChecker, assetCoordination, backendResolver,
		invocationRuntimeOptions{
			Backend: invocationBackend, ASR: asrBackend, Embedding: embeddingBackend,
			Client: invocationProtocol, Dialer: protocolDialer,
		}, revisionResolvers...,
	)
}

func newService(
	assetPlatform models.AssetHostPlatform,
	assetHTTP AssetHTTPDoer,
	assetEndpoints models.RuntimeAssetEndpoints,
	assetMkdirAll AssetMakeDirectories,
	assetStat AssetInspectPath,
	assetHome AssetResolveHomeDirectory,
	assetWriteFile AssetWriteFile,
	assetRename AssetRenamePath,
	assetRemove AssetRemovePath,
	assetReadFile AssetReadFile,
	assetReadDir AssetReadDirectory,
	assetCreate AssetCreateFile,
	assetOpen AssetOpenFile,
	processLauncher HostProcessLauncher,
	hostHTTP HostHTTPDoer,
	hostClock HostClock,
	runtimeRunner platformprocess.CommandRunner,
	runtimeHTTP RuntimeHTTPDoer,
	runtimeInspect RuntimeInspectFile, runtimeTempDir RuntimeTempDirectory,
	runtimeTempFile RuntimeCreateTempFile,
	logger *zap.Logger,
	now func() time.Time,
	issuerEntropy platformrandom.Source,
	pullMetrics PullMetricsRecorder,
	hostLogger HostDiagnosticLogger,
	hostMetrics HostMetricsRecorder,
	localHooks LocalRuntimeHooks,
	resolveEnvironment AssetResolveEnvironment,
	protocolNegotiator HostProtocolNegotiator,
	compatibilityChecker HostCompatibilityChecker,
	assetCoordination AssetStagingCoordination,
	backendResolver BackendArtifactResolver,
	runtimeOptions invocationRuntimeOptions,
	revisionResolvers ...func(context.Context, string) (string, error),
) (models.Service, error) {
	if err := validateConstructionInputs(
		assetPlatform,
		assetHTTP,
		assetMkdirAll,
		assetStat,
		assetHome,
		assetWriteFile,
		assetRename,
		assetRemove,
		assetReadFile,
		assetReadDir,
		assetCreate,
		assetOpen,
		processLauncher,
		hostHTTP,
		hostClock,
		runtimeRunner,
		runtimeHTTP,
		runtimeInspect,
		runtimeTempDir,
		runtimeTempFile,
		now,
		issuerEntropy,
	); err != nil {
		return nil, err
	}
	return composeModelsService(
		assetPlatform,
		assetHTTP,
		assetEndpoints,
		assetMkdirAll,
		assetStat,
		assetHome,
		assetWriteFile,
		assetRename,
		assetRemove,
		assetReadFile,
		assetReadDir,
		assetCreate,
		assetOpen,
		processLauncher,
		hostHTTP,
		hostClock,
		runtimeRunner,
		runtimeHTTP,
		runtimeInspect,
		runtimeTempDir,
		runtimeTempFile,
		logger,
		now,
		issuerEntropy,
		pullMetrics,
		hostLogger,
		hostMetrics,
		localHooks,
		resolveEnvironment,
		protocolNegotiator,
		compatibilityChecker,
		assetCoordination, backendResolver, runtimeOptions, revisionResolvers...,
	)
}

func composeModelsService(
	assetPlatform models.AssetHostPlatform,
	assetHTTP AssetHTTPDoer,
	assetEndpoints models.RuntimeAssetEndpoints,
	assetMkdirAll AssetMakeDirectories,
	assetStat AssetInspectPath,
	assetHome AssetResolveHomeDirectory,
	assetWriteFile AssetWriteFile,
	assetRename AssetRenamePath,
	assetRemove AssetRemovePath,
	assetReadFile AssetReadFile,
	assetReadDir AssetReadDirectory,
	assetCreate AssetCreateFile,
	assetOpen AssetOpenFile,
	processLauncher HostProcessLauncher,
	hostHTTP HostHTTPDoer,
	hostClock HostClock,
	runtimeRunner platformprocess.CommandRunner,
	runtimeHTTP RuntimeHTTPDoer,
	runtimeInspect RuntimeInspectFile,
	runtimeTempDir RuntimeTempDirectory,
	runtimeTempFile RuntimeCreateTempFile,
	logger *zap.Logger,
	now func() time.Time,
	issuerEntropy platformrandom.Source,
	pullMetrics PullMetricsRecorder,
	hostLogger HostDiagnosticLogger,
	hostMetrics HostMetricsRecorder,
	localHooks LocalRuntimeHooks,
	resolveEnvironment AssetResolveEnvironment,
	protocolNegotiator HostProtocolNegotiator,
	compatibilityChecker HostCompatibilityChecker,
	assetCoordination AssetStagingCoordination,
	backendResolver BackendArtifactResolver,
	runtimeOptions invocationRuntimeOptions,
	revisionResolvers ...func(context.Context, string) (string, error),
) (models.Service, error) {
	resolvedEndpoints := resolveAssetEndpoints(assetEndpoints)
	launcher, clock, createTempFile := adaptConstructionPorts(
		processLauncher, hostClock, runtimeTempFile,
	)
	components, err := buildModelsServiceComponents(
		assetPlatform, assetHTTP, resolvedEndpoints, assetMkdirAll, assetStat,
		assetHome, assetWriteFile, assetRename, assetRemove, assetReadFile,
		assetReadDir, assetCreate, assetOpen, processLauncher, hostHTTP, hostClock,
		hostLogger, hostMetrics, resolveEnvironment, protocolNegotiator,
		compatibilityChecker, assetCoordination, runtimeOptions, now, issuerEntropy,
		firstRevisionResolver(revisionResolvers),
	)
	if err != nil {
		return nil, err
	}
	return modelsservice.NewRoot(
		launcher, hostHTTP, clock,
		runtimeRunner, runtimeHTTP, localmodels.InspectFile(runtimeInspect),
		localmodels.TempDirectory(runtimeTempDir), createTempFile,
		components.runtimeScopes, components.catalog, components.assets, components.runtimeHost, components.inference,
		modelseffects.ProcessDependencies{
			Logger: logger, Clock: now, PullMetrics: pullMetrics,
			HostLogger: hostLogger, HostMetrics: hostMetrics, LocalHooks: localHooks,
			ResolveHuggingFaceRevision: firstRevisionResolver(revisionResolvers),
			ResolveBackendArtifact:     backendResolver,
			BackendArtifactPlatform:    assetPlatform,
		},
	)
}

type backendInvocationRuntime struct {
	backend InvocationBackend
}

func (runtime backendInvocationRuntime) Invoke(
	ctx context.Context,
	request inference.InvocationRuntimeRequest,
) (inference.InvocationRuntimeResult, error) {
	content, artifacts, err := runtime.backend(ctx, request.Request)
	if err != nil {
		return inference.InvocationRuntimeResult{}, err
	}
	return inference.InvocationRuntimeResult{
		Content:   content,
		Artifacts: invocationArtifactSources(artifacts),
	}, nil
}

type operationInvocationRuntime struct {
	generic   invocationRuntime
	omni      invocationRuntime
	asr       invocationRuntime
	embedding invocationRuntime
}

func (runtime operationInvocationRuntime) Invoke(
	ctx context.Context,
	request inference.InvocationRuntimeRequest,
) (inference.InvocationRuntimeResult, error) {
	if runtime.asr != nil && isASROperation(request) {
		return runtime.asr.Invoke(ctx, request)
	}
	if runtime.omni != nil && isOMNIOperation(request) {
		return runtime.omni.Invoke(ctx, request)
	}
	if runtime.embedding != nil && isEmbeddingOperation(request) {
		return runtime.embedding.Invoke(ctx, request)
	}
	return runtime.generic.Invoke(ctx, request)
}

func inferenceRuntime(options invocationRuntimeOptions) (invocationRuntime, error) {
	generic := genericInvocationRuntime(options.Backend)
	runtime := operationInvocationRuntime{
		generic: generic,
		omni:    newInvocationRuntime(options.Client, options.Dialer),
	}
	if options.ASR != nil {
		asr, err := newASRInvocationRuntime(options.ASR)
		if err != nil {
			return nil, err
		}
		runtime.asr = asr
	}
	if options.Embedding != nil {
		embedding, err := newEmbeddingInvocationRuntime(options.Embedding)
		if err != nil {
			return nil, err
		}
		runtime.embedding = embedding
	}
	return runtime, nil
}

func genericInvocationRuntime(backend InvocationBackend) invocationRuntime {
	if backend == nil {
		return inference.InputEchoInvocationRuntime{}
	}
	return backendInvocationRuntime{backend: backend}
}

func newASRInvocationRuntime(backend ASRBackend) (invocationRuntime, error) {
	return modelsruntime.New(func(
		ctx context.Context,
		request modelcodecs.ASRRequest,
	) (modelcodecs.ASRResponse, []models.InferenceArtifact, error) {
		response, err := backend(ctx, models.ASRBackendRequest{
			Audio: append([]byte(nil), request.Audio...), MediaType: request.MediaType,
			Prompt: request.Prompt, Parameters: cloneInvocationParameters(request.Parameters),
		})
		if err != nil {
			return modelcodecs.ASRResponse{}, nil, err
		}
		segments := make([]modelcodecs.ASRSegment, len(response.Segments))
		for index, segment := range response.Segments {
			segments[index] = modelcodecs.ASRSegment{
				ID: segment.ID, Start: segment.Start, End: segment.End, Text: segment.Text,
			}
		}
		return modelcodecs.ASRResponse{Text: response.Text, Segments: segments}, response.Artifacts, nil
	})
}

func newEmbeddingInvocationRuntime(backend EmbeddingBackend) (invocationRuntime, error) {
	return modelsruntime.NewEmbedding(func(
		ctx context.Context,
		request modelcodecs.EmbeddingRequest,
	) (modelcodecs.EmbeddingResponse, error) {
		response, err := backend(ctx, models.EmbeddingBackendRequest{
			Text:       request.Prompt,
			Parameters: cloneInvocationParameters(request.Parameters),
		})
		if err != nil {
			return modelcodecs.EmbeddingResponse{}, err
		}
		return modelcodecs.EmbeddingResponse{
			Embeddings: append([]float64(nil), response.Embeddings...),
		}, nil
	})
}

type omniInvocationRuntime struct {
	codec    *localai.OmniCodec
	fallback invocationRuntime
}

// newInvocationRuntime keeps OMNI on the pinned protocol path. A missing
// client fails closed for OMNI while non-OMNI operations retain the generic
// input-echo behavior used by lightweight composition tests.
func newInvocationRuntime(
	client InvocationProtocolClient,
	dialer InvocationProtocolDialer,
) invocationRuntime {
	fallback := inference.InputEchoInvocationRuntime{}
	if isNilDependency(client) {
		client = nil
	}
	var protocolClient localai.ProtocolClient
	if client != nil {
		protocolClient = invocationProtocolAdapter{client: client}
	} else if dialer != nil {
		protocolClient = localai.NewPinnedGRPCProtocolClient(dialer)
	}
	return omniInvocationRuntime{
		codec:    localai.NewPinnedOmniCodec(protocolClient),
		fallback: fallback,
	}
}

func (runtime omniInvocationRuntime) Invoke(
	ctx context.Context,
	request inference.InvocationRuntimeRequest,
) (inference.InvocationRuntimeResult, error) {
	if !isOMNIOperation(request) {
		return runtime.fallback.Invoke(ctx, request)
	}
	if runtime.codec == nil {
		return inference.InvocationRuntimeResult{}, models.ErrUnavailable
	}
	ctx = localai.WithInvocationEndpoint(ctx, request.HostSlot.Endpoint)
	content, err := runtime.codec.Invoke(ctx, request.Request, request.Operation)
	if err != nil {
		return inference.InvocationRuntimeResult{}, err
	}
	return inference.InvocationRuntimeResult{Content: content}, nil
}

type invocationProtocolAdapter struct {
	client InvocationProtocolClient
}

func (adapter invocationProtocolAdapter) Predict(
	ctx context.Context,
	request localai.PredictRequest,
) (localai.PredictResponse, error) {
	inputs := make([]models.InvocationProtocolInput, len(request.Inputs))
	for index, input := range request.Inputs {
		inputs[index] = models.InvocationProtocolInput{
			Slot: input.Slot, Modality: input.Modality, MediaType: input.MediaType,
			Content: input.Content, Reference: input.Reference,
		}
	}
	response, err := adapter.client.Predict(ctx, models.InvocationProtocolRequest{
		Operation: models.OperationOMNI,
		Prompt:    request.Prompt, Inputs: inputs, Parameters: request.Parameters,
	})
	if err != nil {
		return localai.PredictResponse{}, err
	}
	return localai.PredictResponse{Text: response.Text, Usage: response.Usage}, nil
}

func isOMNIOperation(request inference.InvocationRuntimeRequest) bool {
	operation := request.Operation.Name
	if operation == "" {
		operation = request.Request.Operation
	}
	return strings.EqualFold(strings.TrimSpace(operation), models.OperationOMNI)
}

func isASROperation(request inference.InvocationRuntimeRequest) bool {
	operation := request.Operation.Name
	if operation == "" {
		operation = request.Request.Operation
	}
	return strings.EqualFold(strings.TrimSpace(operation), models.OperationASR)
}

func isEmbeddingOperation(request inference.InvocationRuntimeRequest) bool {
	operation := request.Operation.Name
	if operation == "" {
		operation = request.Request.Operation
	}
	return strings.EqualFold(strings.TrimSpace(operation), models.OperationEMBED)
}

func cloneInvocationParameters(parameters map[string]any) map[string]any {
	if parameters == nil {
		return nil
	}
	cloned := make(map[string]any, len(parameters))
	for name, value := range parameters {
		cloned[name] = cloneInvocationParameterValue(value)
	}
	return cloned
}

func cloneInvocationParameterValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneInvocationParameters(typed)
	case []any:
		cloned := make([]any, len(typed))
		for index, item := range typed {
			cloned[index] = cloneInvocationParameterValue(item)
		}
		return cloned
	default:
		return value
	}
}

func invocationArtifactSources(artifacts []models.InferenceArtifact) []inference.InvocationArtifactSource {
	if len(artifacts) == 0 {
		return nil
	}
	sources := make([]inference.InvocationArtifactSource, 0, len(artifacts))
	for _, artifact := range artifacts {
		sources = append(sources, inference.InvocationArtifactSource{
			RefValue:   artifact.Artifact.String(),
			Name:       artifact.Name,
			MediaType:  artifact.MediaType,
			SizeBytes:  artifact.SizeBytes,
			Properties: artifact.Properties,
		})
	}
	return sources
}

type modelsServiceComponents struct {
	runtimeScopes runtimescopes.Service
	assets        scopedassets.Service
	catalog       catalog.Service
	runtimeHost   runtimehost.Service
	inference     inference.Service
}

func buildModelsServiceComponents(
	assetPlatform models.AssetHostPlatform,
	assetHTTP AssetHTTPDoer,
	assetEndpoints models.RuntimeAssetEndpoints,
	assetMkdirAll AssetMakeDirectories,
	assetStat AssetInspectPath,
	assetHome AssetResolveHomeDirectory,
	assetWriteFile AssetWriteFile,
	assetRename AssetRenamePath,
	assetRemove AssetRemovePath,
	assetReadFile AssetReadFile,
	assetReadDir AssetReadDirectory,
	assetCreate AssetCreateFile,
	assetOpen AssetOpenFile,
	processLauncher HostProcessLauncher,
	hostHTTP HostHTTPDoer,
	hostClock HostClock,
	hostLogger HostDiagnosticLogger,
	hostMetrics HostMetricsRecorder,
	resolveEnvironment AssetResolveEnvironment,
	protocolNegotiator HostProtocolNegotiator,
	compatibilityChecker HostCompatibilityChecker,
	assetCoordination AssetStagingCoordination,
	runtimeOptions invocationRuntimeOptions,
	now func() time.Time,
	issuerEntropy platformrandom.Source,
	revisionResolver func(context.Context, string) (string, error),
) (modelsServiceComponents, error) {
	issuerID, err := runtimeScopeIssuerID(issuerEntropy)
	if err != nil {
		return modelsServiceComponents{}, fmt.Errorf("construct Models Runtime Scopes issuer identity: %w", err)
	}
	runtimeScopes, err := runtimescopeswire.NewService(func() string { return issuerID })
	if err != nil {
		return modelsServiceComponents{}, err
	}
	assetService, err := assetswire.NewService(
		runtimeScopes, assetPlatform, assetHTTP, assetEndpoints,
		assetMkdirAll, assetStat, assetHome, assetWriteFile, assetRename,
		assetRemove, assetReadFile, assetReadDir, assetCreate, assetOpen,
		scopedassets.ConstructionOptions{
			ResolveEnvironment: resolveEnvironment,
			ResolveRevision:    revisionResolver,
			Coordination:       assetCoordination,
		},
	)
	if err != nil {
		return modelsServiceComponents{}, err
	}
	catalogService, err := catalogwire.NewService(runtimeScopes, newCatalogReadinessQuery(assetService))
	if err != nil {
		return modelsServiceComponents{}, err
	}
	runtimeHost, err := runtimehostwire.NewService(
		runtimeScopes, assetService, processLauncher, hostHTTP, hostClock, hostLogger, hostMetrics,
		runtimehost.Options{
			Platform: assetPlatform, ProtocolNegotiator: protocolNegotiator,
			CompatibilityChecker: compatibilityChecker,
		},
	)
	if err != nil {
		return modelsServiceComponents{}, err
	}
	runtime, err := inferenceRuntime(runtimeOptions)
	if err != nil {
		return modelsServiceComponents{}, err
	}
	inferenceService, err := inferencewire.NewService(
		runtimeScopes, assetService, catalogService, runtimeHost,
		runtime, inference.InertArtifactFileSystem{}, now,
	)
	if err != nil {
		return modelsServiceComponents{}, err
	}
	return modelsServiceComponents{
		runtimeScopes: runtimeScopes, assets: assetService, catalog: catalogService,
		runtimeHost: runtimeHost, inference: inferenceService,
	}, nil
}

func firstRevisionResolver(
	resolvers []func(context.Context, string) (string, error),
) func(context.Context, string) (string, error) {
	if len(resolvers) == 0 {
		return nil
	}
	return resolvers[0]
}

func resolveAssetEndpoints(overrides models.RuntimeAssetEndpoints) models.RuntimeAssetEndpoints {
	resolved := models.RuntimeAssetEndpoints{
		BaseURL: defaultAssetBaseURL, APIBaseURL: defaultAssetAPIBaseURL,
	}
	if overrides.BaseURL != "" {
		resolved.BaseURL = overrides.BaseURL
	}
	if overrides.APIBaseURL != "" {
		resolved.APIBaseURL = overrides.APIBaseURL
	}
	return resolved
}

func adaptConstructionPorts(
	processLauncher HostProcessLauncher,
	hostClock HostClock,
	runtimeTempFile RuntimeCreateTempFile,
) (modelhost.ProcessLauncher, modelhost.Clock, localmodels.CreateTempFile) {
	var launcher modelhost.ProcessLauncher
	if processLauncher != nil {
		launcher = hostProcessLauncher{next: processLauncher}
	}
	var clock modelhost.Clock
	if hostClock != nil {
		clock = hostClockAdapter{next: hostClock}
	}
	var createTempFile localmodels.CreateTempFile
	if runtimeTempFile != nil {
		createTempFile = runtimeTempFileAdapter{next: runtimeTempFile}.create
	}
	return launcher, clock, createTempFile
}

func newCatalogReadinessQuery(assetService scopedassets.Service) catalog.ReadinessQuery {
	return func(
		ctx context.Context,
		scopeRef models.RuntimeScopeRef,
		scope models.RuntimeScopeConfig,
		detail models.Detail,
	) (models.Runtime, error) {
		puller, err := localmodels.NewScopedAssetPuller(assetService, scopeRef)
		if err != nil {
			return models.Runtime{}, err
		}
		readiness, readinessErr := localmodels.ManagedRuntimeReadinessForFactoryContext(
			ctx,
			&scope.Runtime,
			detail.Name,
			puller,
			localmodels.DefaultManagedRuntimeSourceResolver(),
		)
		if readinessErr != nil && errors.Is(readinessErr, models.ErrNotFound) &&
			detail.Diagnostics["catalogSource"] == "EFFECTIVE_DEFINITION" {
			return localmodels.ManagedRuntimeReadinessForEffectiveDefinitionContext(
				ctx,
				detail.ManagedRuntime,
				&scope.Runtime,
				detail.Name,
				puller,
			)
		}
		return readiness, readinessErr
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func validateConstructionInputs(
	assetPlatform models.AssetHostPlatform,
	assetHTTP modelseffects.AssetHTTPDoer,
	assetMkdirAll modelseffects.AssetMakeDirectories,
	assetStat modelseffects.AssetInspectPath,
	assetHome modelseffects.AssetResolveHomeDirectory,
	assetWriteFile modelseffects.AssetWriteFile,
	assetRename modelseffects.AssetRenamePath,
	assetRemove modelseffects.AssetRemovePath,
	assetReadFile modelseffects.AssetReadFile,
	assetReadDirectory modelseffects.AssetReadDirectory,
	assetCreate modelseffects.AssetCreateFile,
	assetOpen modelseffects.AssetOpenFile,
	processLauncher modelseffects.HostProcessLauncher,
	hostHTTP modelseffects.HostHTTPDoer,
	hostClock modelseffects.HostClock,
	runtimeRunner platformprocess.CommandRunner,
	runtimeHTTP modelseffects.RuntimeHTTPDoer,
	runtimeInspect modelseffects.RuntimeInspectFile,
	runtimeTempDir modelseffects.RuntimeTempDirectory,
	runtimeTempFile modelseffects.RuntimeCreateTempFile,
	now func() time.Time,
	issuerEntropy platformrandom.Source,
) error {
	switch {
	case issuerEntropy == nil:
		return fmt.Errorf("construct Models: issuer entropy is required")
	case assetPlatform.OperatingSystem == "" || assetPlatform.Architecture == "":
		return fmt.Errorf("construct Models: asset host platform is required")
	case isNilDependency(assetHTTP):
		return fmt.Errorf("construct Models: asset HTTP client is required")
	case isNilDependency(assetMkdirAll):
		return fmt.Errorf("construct Models: asset make-directories effect is required")
	case isNilDependency(assetStat):
		return fmt.Errorf("construct Models: asset inspect-path effect is required")
	case isNilDependency(assetHome):
		return fmt.Errorf("construct Models: asset resolve-home effect is required")
	case isNilDependency(assetWriteFile):
		return fmt.Errorf("construct Models: asset write-file effect is required")
	case isNilDependency(assetRename):
		return fmt.Errorf("construct Models: asset rename-path effect is required")
	case isNilDependency(assetRemove):
		return fmt.Errorf("construct Models: asset remove-path effect is required")
	case isNilDependency(assetReadFile):
		return fmt.Errorf("construct Models: asset read-file effect is required")
	case isNilDependency(assetReadDirectory):
		return fmt.Errorf("construct Models: asset read-directory effect is required")
	case isNilDependency(assetCreate):
		return fmt.Errorf("construct Models: asset create-file effect is required")
	case isNilDependency(assetOpen):
		return fmt.Errorf("construct Models: asset open-file effect is required")
	case isNilDependency(processLauncher):
		return fmt.Errorf("construct Models: model host process launcher is required")
	case isNilDependency(hostHTTP):
		return fmt.Errorf("construct Models: model host HTTP client is required")
	case isNilDependency(hostClock):
		return fmt.Errorf("construct Models: model host clock is required")
	case isNilDependency(runtimeRunner):
		return fmt.Errorf("construct Models: model runtime command runner is required")
	case isNilDependency(runtimeHTTP):
		return fmt.Errorf("construct Models: model runtime HTTP client is required")
	case isNilDependency(runtimeInspect):
		return fmt.Errorf("construct Models: model runtime file inspector is required")
	case isNilDependency(runtimeTempDir):
		return fmt.Errorf("construct Models: model runtime temporary directory resolver is required")
	case isNilDependency(runtimeTempFile):
		return fmt.Errorf("construct Models: model runtime temporary file creator is required")
	case isNilDependency(now):
		return fmt.Errorf("construct Models: process clock is required")
	default:
		return nil
	}
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

func runtimeScopeIssuerID(entropy platformrandom.Source) (string, error) {
	var identity [16]byte
	for index := range identity {
		value, err := entropy.Int63n(256)
		if err != nil {
			return "", err
		}
		identity[index] = byte(value)
	}
	return hex.EncodeToString(identity[:]), nil
}

// NewInvocationArtifactExporter constructs the Models-owned invocation artifact exporter.
func NewInvocationArtifactExporter(fileSystem InvocationArtifactFileSystem) (InvocationArtifactExporter, error) {
	return inferencewire.NewInvocationArtifactExporter(fileSystem)
}

type hostProcessLauncher struct {
	next modelseffects.HostProcessLauncher
}

func (a hostProcessLauncher) Start(ctx context.Context, spec modelhost.ProcessStartSpec) (modelhost.ManagedProcess, error) {
	process, err := a.next.Start(ctx, modelseffects.HostProcessStartSpec{
		Command: spec.Command, Args: spec.Args, Env: spec.Env, WorkDir: spec.WorkDir, HealthEndpoint: spec.HealthEndpoint,
	})
	if err != nil {
		return nil, err
	}
	return process, nil
}

type hostClockAdapter struct{ next modelseffects.HostClock }

func (a hostClockAdapter) Now() time.Time { return a.next.Now() }
func (a hostClockAdapter) NewTimer(duration time.Duration) modelhost.Timer {
	return a.next.NewTimer(duration)
}

type runtimeTempFileAdapter struct {
	next modelseffects.RuntimeCreateTempFile
}

func (a runtimeTempFileAdapter) create(dir, pattern string) (localmodels.TempFile, error) {
	return a.next(dir, pattern)
}
