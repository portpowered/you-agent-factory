package factorydefinitions

import "github.com/portpowered/infinite-you/pkg/services/work"

const (
	// PackagedTTSFactoryProject identifies the built-in @you/tts factory project id.
	PackagedTTSFactoryProject = "builtin-tts"
	// PackagedTTSInvokeWorkstationName is the MODEL_INVOKE workstation that runs TTS.
	PackagedTTSInvokeWorkstationName = "execute-tts"
	// DefaultTTSModelName is the default managed local TTS model for @you/tts.
	DefaultTTSModelName = "OMNIVOICE_Q4_K_M"
	// DefaultTTSBackendName is the default managed local TTS backend for @you/tts.
	DefaultTTSBackendName = "LLAMACPP"
)

// TTSInvocationMetadata is the default primary invocation result for successful
// @you/tts runs.
type TTSInvocationMetadata struct {
	ArtifactPath string `json:"artifactPath"`
	MediaType    string `json:"mediaType"`
	Backend      string `json:"backend"`
	TraceID      string `json:"traceId,omitempty"`
	SessionID    string `json:"sessionId,omitempty"`
}

// InvocationOutputShapingService owns the packaged-factory policy that turns
// raw worker output into canonical terminal Work content.
type InvocationOutputShapingService interface {
	ShouldFormatInvocationSummary(*FactoryWorkstationConfig) bool
	SummaryContentFromWorkerOutput(string, string) ([]work.WorkContentPart, error)
	ShouldFormatInvocationResponse(*FactoryWorkstationConfig) bool
	ResponseContentFromWorkerOutput(string, string) ([]work.WorkContentPart, error)
	ShouldFormatTTSInvocationMetadata(*FactoryWorkstationConfig) bool
	TTSBackendLabelFromWorker(*FactoryWorkerConfig) string
	TTSMetadataContentFromWorkerOutput(string, string, string, string) ([]work.WorkContentPart, error)
}
