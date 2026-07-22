package tts

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/invocationoutput"
	builtin "github.com/portpowered/infinite-you/pkg/services/factory_definitions/packages/definitions/tts"
)

var BuiltInFactoryJSON = builtin.BuiltInFactoryJSON

const (
	PackagedFactoryProject        = factorydefinitions.PackagedTTSFactoryProject
	PackagedInvokeWorkstationName = factorydefinitions.PackagedTTSInvokeWorkstationName
	DefaultModelName              = factorydefinitions.DefaultTTSModelName
	DefaultBackendName            = factorydefinitions.DefaultTTSBackendName
)

type InvocationMetadata = factorydefinitions.TTSInvocationMetadata

var (
	ShouldFormatInvocationMetadata  = invocationoutput.ShouldFormatTTSInvocationMetadata
	BackendLabelFromWorker          = invocationoutput.TTSBackendLabelFromWorker
	MetadataContentFromWorkerOutput = invocationoutput.TTSMetadataContentFromWorkerOutput
)
