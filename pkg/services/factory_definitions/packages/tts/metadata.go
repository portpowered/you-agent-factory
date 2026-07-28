// Package tts is a transitional shim over the Distribution-owned TTS package
// asset/metadata implementation.
package tts

import (
	distributiontts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution/tts"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

const (
	PackagedFactoryProject        = distributiontts.PackagedFactoryProject
	PackagedInvokeWorkstationName = distributiontts.PackagedInvokeWorkstationName
	DefaultModelName              = distributiontts.DefaultModelName
	DefaultBackendName            = distributiontts.DefaultBackendName
)

type InvocationMetadata = distributiontts.InvocationMetadata

var (
	ShouldFormatInvocationMetadata  = distributiontts.ShouldFormatInvocationMetadata
	BackendLabelFromWorker          = distributiontts.BackendLabelFromWorker
	MetadataContentFromWorkerOutput = distributiontts.MetadataContentFromWorkerOutput
)

// PackagedFactoryName is re-exported for transitional callers.
const PackagedFactoryName = factorydefinitions.PackagedTTSFactoryName
