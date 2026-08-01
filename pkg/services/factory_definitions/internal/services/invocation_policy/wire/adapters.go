package wire

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryeffects "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/effects"
	invocationoutput "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy/internal/invocationoutput"
	ttsobservability "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy/internal/ttsobservability"
	workstationexecution "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy/internal/workstationexecution"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

// These representation-neutral policy operations are exposed through the
// invocation_policy owner wire package so sibling owners never reach into its
// implementation tree.
var (
	ShouldFormatInvocationSummary      = invocationoutput.ShouldFormatInvocationSummary
	SummaryContentFromWorkerOutput     = invocationoutput.SummaryContentFromWorkerOutput
	ShouldFormatInvocationResponse     = invocationoutput.ShouldFormatInvocationResponse
	ResponseContentFromWorkerOutput    = invocationoutput.ResponseContentFromWorkerOutput
	ShouldFormatTTSInvocationMetadata  = invocationoutput.ShouldFormatTTSInvocationMetadata
	TTSBackendLabelFromWorker          = invocationoutput.TTSBackendLabelFromWorker
	TTSMetadataContentFromWorkerOutput = invocationoutput.TTSMetadataContentFromWorkerOutput
	IsPackagedTTSFactory               = ttsobservability.IsPackagedTTSFactory
	TTSBackendRuntimeLabel             = ttsobservability.TTSBackendRuntimeLabel
	ClassifyTTSInvocationWait          = ttsobservability.ClassifyTTSInvocationWait
	IsTTSModelNotReadyFailure          = ttsobservability.IsTTSModelNotReadyFailure
)

func NormalizeExecutionLimit(cfg *factorydefinitions.FactoryWorkstationConfig) {
	workstationexecution.NormalizeExecutionLimit(cfg)
}

func NewWorkstationExecutionPolicy() factoryeffects.WorkstationExecutionPolicyService {
	return workstationexecution.NewService()
}

var _ func(*factorydefinitions.FactoryWorkstationConfig) bool = ShouldFormatInvocationSummary
var _ func(string, string) ([]work.WorkContentPart, error) = SummaryContentFromWorkerOutput
