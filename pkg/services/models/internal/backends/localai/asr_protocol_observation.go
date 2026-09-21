package localai

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path"
	"sort"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

const (
	asrEvidenceBackendWhisper = "LOCALAI_WHISPER"
	asrEvidenceBackendLlama   = "LOCALAI_LLAMACPP"
	asrEvidenceBackendVoice   = "LOCALAI_VIBEVOICE"
	asrEvidenceBackendOther   = "OTHER"
	asrEvidenceBackendUnknown = "UNKNOWN"

	asrEvidencePlatformWindowsAMD64 = "WINDOWS_AMD64"
	asrEvidencePlatformLinuxAMD64   = "LINUX_AMD64"
	asrEvidencePlatformDarwinARM64  = "DARWIN_ARM64"
	asrEvidencePlatformOther        = "OTHER"
	asrEvidencePlatformUnknown      = "UNKNOWN"
)

type asrProtocolObservation struct {
	recorder    modelseffects.RuntimeEvidenceRecorder
	correlation *modelseffects.ASRLiveCorrelationController
	record      modelseffects.RuntimeASRProtocolObservation
	stagedPath  string
}

func newASRProtocolObservation(
	ctx context.Context,
	request models.ASRBackendRequest,
) *asrProtocolObservation {
	recorder, configuration, ok := modelseffects.RuntimeObservationFromContext(ctx)
	correlation := modelseffects.ASRLiveCorrelationFromContext(ctx)
	if !ok && correlation == nil {
		return nil
	}
	observation := &asrProtocolObservation{
		recorder:    recorder,
		correlation: correlation,
		record: modelseffects.RuntimeASRProtocolObservation{
			Phase:            modelseffects.RuntimeASRPhaseStageAudio,
			RPCMethod:        modelseffects.RuntimeASRPCMethodAudioTranscription,
			SelectedBackend:  selectedASREvidenceBackend(configuration.Backend),
			SelectedPlatform: selectedASREvidencePlatform(configuration.Platform.OperatingSystem, configuration.Platform.Architecture),
			RPCStatus:        modelseffects.RuntimeASRStatusNotAttempted,
		},
	}
	if ok {
		observation.recordResolvedConfiguration(configuration)
	}
	if len(request.Audio) > 0 {
		observation.record.AudioBytes = uint64(len(request.Audio))
		observation.record.AudioSHA256 = asrEvidenceSHA256(request.Audio)
	}
	return observation
}

func (observation *asrProtocolObservation) recordResolvedConfiguration(
	configuration modelseffects.ResolvedHostConfiguration,
) {
	modelIdentity, _ := json.Marshal(struct {
		Name     string
		Source   string
		Revision string
	}{configuration.ModelName, configuration.Source.NameOrURI, configuration.Revision})
	if len(modelIdentity) > 0 {
		observation.record.ModelIdentitySHA256 = asrEvidenceSHA256(modelIdentity)
	}
	if configuration.ModelPath != "" {
		observation.record.ModelPathSHA256 = asrEvidenceStringSHA256(configuration.ModelPath)
	}
	observation.record.ModelFileCount = uint32(len(configuration.ModelFiles))
	observation.record.ModelFileNamesSHA256 = asrEvidenceFileNamesSHA256(configuration.ModelFiles)
	observation.record.BackendFileCount = uint32(len(configuration.BackendFiles))
	observation.record.BackendFileNamesSHA256 = asrEvidenceFileNamesSHA256(configuration.BackendFiles)
	observation.record.BackendArtifactSHA256 = normalizeASREvidenceDigest(configuration.BackendArtifact.SHA256)
	if configuration.BackendArtifact.Bytes > 0 {
		observation.record.BackendArtifactBytes = uint64(configuration.BackendArtifact.Bytes)
	}
	if configuration.ProtocolVersion != "" {
		observation.record.ProtocolVersionSHA256 = asrEvidenceStringSHA256(configuration.ProtocolVersion)
	}
	observation.record.ResolvedConfigurationSHA256 = resolvedASRConfigurationSHA256(observation.record)
}

func resolvedASRConfigurationSHA256(
	observation modelseffects.RuntimeASRProtocolObservation,
) string {
	configuration, _ := json.Marshal(struct {
		Backend                string
		Platform               string
		ModelIdentitySHA256    string
		ModelPathSHA256        string
		ModelFileCount         uint32
		ModelFileNamesSHA256   string
		BackendArtifactSHA256  string
		BackendArtifactBytes   uint64
		BackendFileCount       uint32
		BackendFileNamesSHA256 string
		ProtocolVersionSHA256  string
	}{
		Backend: observation.SelectedBackend, Platform: observation.SelectedPlatform,
		ModelIdentitySHA256:    observation.ModelIdentitySHA256,
		ModelPathSHA256:        observation.ModelPathSHA256,
		ModelFileCount:         observation.ModelFileCount,
		ModelFileNamesSHA256:   observation.ModelFileNamesSHA256,
		BackendArtifactSHA256:  observation.BackendArtifactSHA256,
		BackendArtifactBytes:   observation.BackendArtifactBytes,
		BackendFileCount:       observation.BackendFileCount,
		BackendFileNamesSHA256: observation.BackendFileNamesSHA256,
		ProtocolVersionSHA256:  observation.ProtocolVersionSHA256,
	})
	if len(configuration) == 0 {
		return ""
	}
	return asrEvidenceSHA256(configuration)
}

func (observation *asrProtocolObservation) observeStagedPath(path string) {
	if observation == nil || path == "" {
		return
	}
	observation.stagedPath = path
	observation.record.StagedPathSHA256 = asrEvidenceStringSHA256(path)
}

func (observation *asrProtocolObservation) observeRequest(
	message proto.Message,
	payload []byte,
	method string,
) error {
	if observation == nil {
		return nil
	}
	observation.record.Phase = modelseffects.RuntimeASRPhaseBuildRequest
	observation.record.RPCMethod = asrEvidenceRPCMethod(method)
	observation.record.RequestBytes = uint64(len(payload))
	observation.record.RequestSHA256 = asrEvidenceSHA256(payload)
	request, ok := message.(*TranscriptRequest)
	if !ok || request == nil {
		return nil
	}
	observation.record.PromptBytes = uint64(len(request.GetPrompt()))
	observation.record.Threads = request.GetThreads()
	if destination := request.GetDst(); destination != "" {
		observation.record.RequestDestinationSHA256 = asrEvidenceStringSHA256(destination)
		if observation.stagedPath != "" {
			observation.record.DestinationCompared = true
			observation.record.DestinationMatchesStaged = destination == observation.stagedPath
		}
	}
	observation.record.RequestSemanticSHA256 = semanticASRRequestSHA256(message)
	if observation.correlation != nil {
		return observation.correlation.RecordRequestSemanticSHA256(observation.record.RequestSemanticSHA256)
	}
	return nil
}

func (observation *asrProtocolObservation) observeEndpoint(ctx context.Context, address string) error {
	if observation == nil || observation.correlation == nil {
		return nil
	}
	return observation.correlation.ObserveEndpoint(ctx, address)
}

func (observation *asrProtocolObservation) awaitDecodedResponse(
	ctx context.Context,
	response models.ASRBackendResponse,
) error {
	if observation == nil || observation.correlation == nil {
		return nil
	}
	semanticPayload, err := json.Marshal(struct {
		Text     string                     `json:"text"`
		Segments []models.ASRBackendSegment `json:"segments"`
	}{Text: response.Text, Segments: response.Segments})
	if err != nil {
		return modelseffects.ErrASRLiveCorrelationInvalid
	}
	return observation.correlation.AwaitResponseRelease(
		ctx,
		observation.record.RequestSemanticSHA256,
		asrEvidenceSHA256(semanticPayload),
	)
}

func semanticASRRequestSHA256(message proto.Message) string {
	request, ok := proto.Clone(message).(*TranscriptRequest)
	if !ok || request == nil {
		return ""
	}
	request.Dst = ""
	semanticPayload, err := proto.Marshal(request)
	if err != nil {
		return ""
	}
	return asrEvidenceSHA256(semanticPayload)
}

func (observation *asrProtocolObservation) observeDial() {
	if observation != nil {
		observation.record.Phase = modelseffects.RuntimeASRPhaseDial
	}
}

func (observation *asrProtocolObservation) observeRPCResult(err error) {
	if observation == nil {
		return
	}
	observation.record.Phase = modelseffects.RuntimeASRPhaseRPC
	if err == nil {
		observation.record.RPCStatus = "OK"
		return
	}
	observation.record.RPCStatus = asrEvidenceRPCStatus(status.Code(err))
}

func (observation *asrProtocolObservation) observeResponse(payload []byte) {
	if observation == nil {
		return
	}
	observation.record.ResponseReceived = true
	observation.record.ResponseBytes = uint64(len(payload))
	observation.record.ResponseSHA256 = asrEvidenceSHA256(payload)
}

func (observation *asrProtocolObservation) observeDecodedResponse(response *TranscriptResult) {
	if observation == nil {
		return
	}
	observation.record.Phase = modelseffects.RuntimeASRPhaseDecodeResponse
	observation.record.ResponseDecoded = true
	if response == nil {
		return
	}
	observation.record.TranscriptTextBytes = uint64(len(response.GetText()))
	observation.record.SegmentCount = uint32(len(response.GetSegments()))
}

func (observation *asrProtocolObservation) recordInvocation(err error) {
	if observation == nil || observation.recorder == nil {
		return
	}
	outcome := modelseffects.RuntimeEvidenceOutcomeCompleted
	if err != nil {
		outcome = modelseffects.RuntimeEvidenceOutcomeFailed
		observation.record.FailureClass = asrEvidenceFailureClass(err)
	} else {
		observation.record.Phase = modelseffects.RuntimeASRPhaseComplete
	}
	observation.recorder.RecordRuntimeEvidence(modelseffects.RuntimeEvidenceRecord{
		Kind:        modelseffects.RuntimeEvidenceKindASRProtocol,
		Stage:       modelseffects.RuntimeStageInvoke,
		Outcome:     outcome,
		ASRProtocol: &observation.record,
	})
}

func asrEvidenceFailureClass(err error) string {
	if errors.Is(err, context.Canceled) {
		return "CANCELLED"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "TIMEOUT"
	}
	var failure *models.InvocationFailure
	if errors.As(err, &failure) && failure != nil {
		switch failure.Class {
		case models.InvocationFailureClassBackendProtocol:
			return "BACKEND_PROTOCOL"
		case models.InvocationFailureClassMalformedResponse:
			return "MALFORMED_RESPONSE"
		case models.InvocationFailureClassInvalidParameter:
			return "INVALID_PARAMETER"
		}
	}
	return "OTHER"
}

func selectedASREvidenceBackend(backend string) string {
	switch strings.ToLower(strings.TrimSpace(backend)) {
	case "localai-whisper":
		return asrEvidenceBackendWhisper
	case "localai-llamacpp":
		return asrEvidenceBackendLlama
	case "localai-vibevoice":
		return asrEvidenceBackendVoice
	case "":
		return asrEvidenceBackendUnknown
	default:
		return asrEvidenceBackendOther
	}
}

func selectedASREvidencePlatform(operatingSystem, architecture string) string {
	switch strings.ToLower(strings.TrimSpace(operatingSystem)) + "/" + strings.ToLower(strings.TrimSpace(architecture)) {
	case "windows/amd64":
		return asrEvidencePlatformWindowsAMD64
	case "linux/amd64":
		return asrEvidencePlatformLinuxAMD64
	case "darwin/arm64":
		return asrEvidencePlatformDarwinARM64
	case "/":
		return asrEvidencePlatformUnknown
	default:
		return asrEvidencePlatformOther
	}
}

func asrEvidenceRPCStatus(code codes.Code) string {
	switch code {
	case codes.OK:
		return "OK"
	case codes.Canceled:
		return "CANCELED"
	case codes.Unknown:
		return "UNKNOWN"
	case codes.InvalidArgument:
		return "INVALID_ARGUMENT"
	case codes.DeadlineExceeded:
		return "DEADLINE_EXCEEDED"
	case codes.NotFound:
		return "NOT_FOUND"
	case codes.AlreadyExists:
		return "ALREADY_EXISTS"
	case codes.PermissionDenied:
		return "PERMISSION_DENIED"
	case codes.ResourceExhausted:
		return "RESOURCE_EXHAUSTED"
	case codes.FailedPrecondition:
		return "FAILED_PRECONDITION"
	case codes.Aborted:
		return "ABORTED"
	case codes.OutOfRange:
		return "OUT_OF_RANGE"
	case codes.Unauthenticated:
		return "UNAUTHENTICATED"
	case codes.Internal:
		return "INTERNAL"
	case codes.Unavailable:
		return "UNAVAILABLE"
	case codes.DataLoss:
		return "DATA_LOSS"
	case codes.Unimplemented:
		return "UNIMPLEMENTED"
	default:
		return "UNKNOWN"
	}
}

func asrEvidenceRPCMethod(method string) string {
	if method == localAITranscriptionMethod {
		return modelseffects.RuntimeASRPCMethodAudioTranscription
	}
	return "OTHER"
}

func asrEvidenceFileNamesSHA256(files []string) string {
	if len(files) == 0 {
		return ""
	}
	names := make([]string, 0, len(files))
	for _, file := range files {
		name := path.Base(strings.ReplaceAll(strings.TrimSpace(file), "\\", "/"))
		names = append(names, name)
	}
	if len(names) == 0 {
		return ""
	}
	sort.Strings(names)
	payload, err := json.Marshal(names)
	if err != nil {
		return ""
	}
	return asrEvidenceSHA256(payload)
}

func normalizeASREvidenceDigest(digest string) string {
	digest = strings.ToLower(strings.TrimSpace(digest))
	if len(digest) != sha256.Size*2 {
		return ""
	}
	if _, err := hex.DecodeString(digest); err != nil {
		return ""
	}
	return digest
}

func asrEvidenceStringSHA256(value string) string {
	return asrEvidenceSHA256([]byte(value))
}

func asrEvidenceSHA256(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}
