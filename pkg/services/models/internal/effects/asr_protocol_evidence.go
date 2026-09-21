package effects

import "strings"

const (
	// RuntimeEvidenceKindASRProtocol identifies one private, redacted ASR
	// request/response observation emitted only when runtime evidence is enabled.
	RuntimeEvidenceKindASRProtocol = "ASR_PROTOCOL"
	// RuntimeASRPCMethodAudioTranscription identifies the pinned ASR method.
	RuntimeASRPCMethodAudioTranscription = "AUDIO_TRANSCRIPTION"

	// RuntimeASRPhase* values describe the last bounded protocol phase reached.
	RuntimeASRPhaseStageAudio     = "STAGE_AUDIO"
	RuntimeASRPhaseBuildRequest   = "BUILD_REQUEST"
	RuntimeASRPhaseDial           = "DIAL"
	RuntimeASRPhaseRPC            = "RPC"
	RuntimeASRPhaseDecodeResponse = "DECODE_RESPONSE"
	RuntimeASRPhaseMapResponse    = "MAP_RESPONSE"
	RuntimeASRPhaseComplete       = "COMPLETE"
	RuntimeASRStatusNotAttempted  = "NOT_ATTEMPTED"
)

// RuntimeASRProtocolObservation contains only allow-listed ASR facts. It is
// private Models evidence and deliberately has no field for a path, prompt,
// transcript, endpoint, token, or backend error message.
type RuntimeASRProtocolObservation struct {
	Phase                       string `json:"phase"`
	FailureClass                string `json:"failure_class,omitempty"`
	RPCMethod                   string `json:"rpc_method"`
	SelectedBackend             string `json:"selected_backend"`
	SelectedPlatform            string `json:"selected_platform"`
	ModelIdentitySHA256         string `json:"model_identity_sha256,omitempty"`
	ModelPathSHA256             string `json:"model_path_sha256,omitempty"`
	ModelFileCount              uint32 `json:"model_file_count,omitempty"`
	ModelFileNamesSHA256        string `json:"model_file_names_sha256,omitempty"`
	BackendArtifactSHA256       string `json:"backend_artifact_sha256,omitempty"`
	BackendArtifactBytes        uint64 `json:"backend_artifact_bytes,omitempty"`
	BackendFileCount            uint32 `json:"backend_file_count,omitempty"`
	BackendFileNamesSHA256      string `json:"backend_file_names_sha256,omitempty"`
	ProtocolVersionSHA256       string `json:"protocol_version_sha256,omitempty"`
	ResolvedConfigurationSHA256 string `json:"resolved_configuration_sha256,omitempty"`
	StagedPathSHA256            string `json:"staged_path_sha256,omitempty"`
	AudioBytes                  uint64 `json:"audio_bytes,omitempty"`
	AudioSHA256                 string `json:"audio_sha256,omitempty"`
	RequestBytes                uint64 `json:"request_bytes,omitempty"`
	RequestSHA256               string `json:"request_sha256,omitempty"`
	RequestSemanticSHA256       string `json:"request_semantic_sha256,omitempty"`
	RequestDestinationSHA256    string `json:"request_destination_sha256,omitempty"`
	DestinationCompared         bool   `json:"destination_compared,omitempty"`
	DestinationMatchesStaged    bool   `json:"destination_matches_staged,omitempty"`
	PromptBytes                 uint64 `json:"prompt_bytes,omitempty"`
	Threads                     uint32 `json:"threads,omitempty"`
	RPCStatus                   string `json:"rpc_status"`
	ResponseReceived            bool   `json:"response_received,omitempty"`
	ResponseBytes               uint64 `json:"response_bytes,omitempty"`
	ResponseSHA256              string `json:"response_sha256,omitempty"`
	ResponseDecoded             bool   `json:"response_decoded,omitempty"`
	TranscriptTextBytes         uint64 `json:"transcript_text_bytes,omitempty"`
	SegmentCount                uint32 `json:"segment_count,omitempty"`
}

func normalizeRuntimeASRProtocolEvidenceRecord(
	record RuntimeEvidenceRecord,
) (RuntimeEvidenceRecord, bool) {
	if record.ASRProtocol == nil {
		return RuntimeEvidenceRecord{}, false
	}
	observation := *record.ASRProtocol
	observation.Phase = strings.ToUpper(strings.TrimSpace(observation.Phase))
	observation.FailureClass = strings.ToUpper(strings.TrimSpace(observation.FailureClass))
	observation.RPCMethod = strings.ToUpper(strings.TrimSpace(observation.RPCMethod))
	observation.SelectedBackend = strings.ToUpper(strings.TrimSpace(observation.SelectedBackend))
	observation.SelectedPlatform = strings.ToUpper(strings.TrimSpace(observation.SelectedPlatform))
	observation.RPCStatus = strings.ToUpper(strings.TrimSpace(observation.RPCStatus))
	record.ASRProtocol = &observation
	if !validRuntimeASRProtocolEvidenceRecord(record) {
		return RuntimeEvidenceRecord{}, false
	}
	if record.DurationMillis < 0 {
		record.DurationMillis = 0
	}
	return record, true
}

func validRuntimeASRProtocolEvidenceRecord(record RuntimeEvidenceRecord) bool {
	if record.Kind != RuntimeEvidenceKindASRProtocol || record.Stage != RuntimeStageInvoke ||
		record.ASRProtocol == nil || (record.Outcome != RuntimeEvidenceOutcomeCompleted &&
		record.Outcome != RuntimeEvidenceOutcomeFailed) || !emptyASRProtocolEvidenceFailure(record) {
		return false
	}
	return validRuntimeASRProtocolObservation(*record.ASRProtocol, record.Outcome)
}

func emptyASRProtocolEvidenceFailure(record RuntimeEvidenceRecord) bool {
	return record.Class == "" && record.Subcause == "" && record.CauseSHA256 == "" &&
		record.ExitClass == "" && record.ExitCode == 0 && !record.ExitCodeKnown &&
		record.StdoutBytes == 0 && record.StdoutSHA256 == "" && !record.StdoutTruncated &&
		record.StderrBytes == 0 && record.StderrSHA256 == "" && !record.StderrTruncated &&
		record.CauseCode == "" && record.CauseMessage == "" && !record.CauseMessageRedacted
}

func validRuntimeASRProtocolObservation(
	observation RuntimeASRProtocolObservation,
	outcome string,
) bool {
	if !isRuntimeASRPhase(observation.Phase) || !isRuntimeASRPCMethod(observation.RPCMethod) ||
		!isRuntimeASRBackend(observation.SelectedBackend) ||
		!isRuntimeASRPlatform(observation.SelectedPlatform) || !isRuntimeASRPCStatus(observation.RPCStatus) ||
		!validRuntimeASRFailureClass(observation.FailureClass) {
		return false
	}
	if outcome == RuntimeEvidenceOutcomeCompleted {
		if observation.Phase != RuntimeASRPhaseComplete || observation.FailureClass != "" ||
			observation.RPCStatus != "OK" || !observation.ResponseReceived || !observation.ResponseDecoded {
			return false
		}
	} else if observation.FailureClass == "" || observation.Phase == RuntimeASRPhaseComplete {
		return false
	}
	return validRuntimeASRDigests(observation) && validRuntimeASRByteFacts(observation)
}

func isRuntimeASRPCMethod(method string) bool {
	return method == RuntimeASRPCMethodAudioTranscription || method == "OTHER"
}

func validRuntimeASRDigests(observation RuntimeASRProtocolObservation) bool {
	digests := []string{
		observation.ModelIdentitySHA256, observation.ModelPathSHA256,
		observation.ModelFileNamesSHA256, observation.BackendArtifactSHA256,
		observation.BackendFileNamesSHA256, observation.ProtocolVersionSHA256,
		observation.ResolvedConfigurationSHA256, observation.StagedPathSHA256,
		observation.AudioSHA256, observation.RequestSHA256,
		observation.RequestSemanticSHA256, observation.RequestDestinationSHA256,
		observation.ResponseSHA256,
	}
	for _, digest := range digests {
		if digest != "" && !validRuntimeCauseSHA256(digest) {
			return false
		}
	}
	return digestMatchesCount(observation.ModelFileCount, observation.ModelFileNamesSHA256) &&
		digestMatchesCount(observation.BackendFileCount, observation.BackendFileNamesSHA256) &&
		byteDigestMatches(observation.AudioBytes, observation.AudioSHA256) &&
		byteDigestMatches(observation.RequestBytes, observation.RequestSHA256) &&
		responseDigestMatches(observation)
}

func responseDigestMatches(observation RuntimeASRProtocolObservation) bool {
	if observation.ResponseReceived {
		return validRuntimeCauseSHA256(observation.ResponseSHA256)
	}
	return observation.ResponseBytes == 0 && observation.ResponseSHA256 == ""
}

func validRuntimeASRByteFacts(observation RuntimeASRProtocolObservation) bool {
	if observation.DestinationCompared &&
		(observation.StagedPathSHA256 == "" || observation.RequestDestinationSHA256 == "") {
		return false
	}
	if observation.DestinationMatchesStaged && !observation.DestinationCompared {
		return false
	}
	if observation.ResponseDecoded && !observation.ResponseReceived {
		return false
	}
	if observation.ResponseReceived != (observation.RPCStatus == "OK") {
		return false
	}
	if !observation.ResponseReceived &&
		(observation.ResponseBytes != 0 || observation.ResponseSHA256 != "" || observation.ResponseDecoded) {
		return false
	}
	return true
}

func digestMatchesCount(count uint32, digest string) bool {
	return (count == 0 && digest == "") || (count > 0 && validRuntimeCauseSHA256(digest))
}

func byteDigestMatches(bytes uint64, digest string) bool {
	return (bytes == 0 && digest == "") || (bytes > 0 && validRuntimeCauseSHA256(digest))
}

func isRuntimeASRPhase(phase string) bool {
	switch phase {
	case RuntimeASRPhaseStageAudio, RuntimeASRPhaseBuildRequest, RuntimeASRPhaseDial,
		RuntimeASRPhaseRPC, RuntimeASRPhaseDecodeResponse, RuntimeASRPhaseMapResponse,
		RuntimeASRPhaseComplete:
		return true
	default:
		return false
	}
}

func isRuntimeASRBackend(backend string) bool {
	switch backend {
	case "LOCALAI_WHISPER", "LOCALAI_LLAMACPP", "LOCALAI_VIBEVOICE", "OTHER", "UNKNOWN":
		return true
	default:
		return false
	}
}

func isRuntimeASRPlatform(platform string) bool {
	switch platform {
	case "WINDOWS_AMD64", "LINUX_AMD64", "DARWIN_ARM64", "OTHER", "UNKNOWN":
		return true
	default:
		return false
	}
}

func isRuntimeASRPCStatus(status string) bool {
	switch status {
	case "NOT_ATTEMPTED", "OK", "CANCELED", "UNKNOWN", "INVALID_ARGUMENT",
		"DEADLINE_EXCEEDED", "NOT_FOUND", "ALREADY_EXISTS", "PERMISSION_DENIED",
		"RESOURCE_EXHAUSTED", "FAILED_PRECONDITION", "ABORTED", "OUT_OF_RANGE",
		"UNAUTHENTICATED", "INTERNAL", "UNAVAILABLE", "DATA_LOSS", "UNIMPLEMENTED":
		return true
	default:
		return false
	}
}

func validRuntimeASRFailureClass(class string) bool {
	switch class {
	case "", "BACKEND_PROTOCOL", "MALFORMED_RESPONSE", "INVALID_PARAMETER", "CANCELLED", "TIMEOUT", "OTHER":
		return true
	default:
		return false
	}
}
