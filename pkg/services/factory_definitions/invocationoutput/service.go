// Package invocationoutput is a transitional re-export surface for packaged
// invocation-output shaping policy. Implementation is owned by nested
// internal/services/invocation_policy/invocationoutput; deletion is deferred to DEL packets.
package invocationoutput

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	invocationpolicyoutput "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy/invocationoutput"
)

func NewService() factorydefinitions.InvocationOutputShapingService {
	return invocationpolicyoutput.NewService()
}

var (
	ShouldFormatInvocationSummary      = invocationpolicyoutput.ShouldFormatInvocationSummary
	SummaryContentFromWorkerOutput     = invocationpolicyoutput.SummaryContentFromWorkerOutput
	ShouldFormatInvocationResponse     = invocationpolicyoutput.ShouldFormatInvocationResponse
	ResponseContentFromWorkerOutput    = invocationpolicyoutput.ResponseContentFromWorkerOutput
	ShouldFormatTTSInvocationMetadata  = invocationpolicyoutput.ShouldFormatTTSInvocationMetadata
	TTSBackendLabelFromWorker          = invocationpolicyoutput.TTSBackendLabelFromWorker
	TTSMetadataContentFromWorkerOutput = invocationpolicyoutput.TTSMetadataContentFromWorkerOutput
)
