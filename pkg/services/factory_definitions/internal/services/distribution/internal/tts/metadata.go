package tts

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	invocationpolicywire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy/wire"
)

const (
	PackagedFactoryProject        = factorydefinitions.PackagedTTSFactoryProject
	PackagedInvokeWorkstationName = factorydefinitions.PackagedTTSInvokeWorkstationName
	DefaultModelName              = factorydefinitions.DefaultTTSModelName
	DefaultBackendName            = factorydefinitions.DefaultTTSBackendName
)

type InvocationMetadata = factorydefinitions.TTSInvocationMetadata

var (
	ShouldFormatInvocationMetadata  = invocationpolicywire.ShouldFormatTTSInvocationMetadata
	BackendLabelFromWorker          = invocationpolicywire.TTSBackendLabelFromWorker
	MetadataContentFromWorkerOutput = invocationpolicywire.TTSMetadataContentFromWorkerOutput
)
