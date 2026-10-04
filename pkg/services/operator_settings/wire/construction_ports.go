package wire

import (
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

// Construction/process-edge port aliases for owner wire and process-edge bags.
// Canonical definitions live on the thin Operator Settings root; peers depend
// on Service rather than these ports.
type (
	TemporaryFile       = operatorsettings.TemporaryFile
	FileSystem          = operatorsettings.FileSystem
	CreateTemporaryFile = operatorsettings.CreateTemporaryFile
	ProviderCatalog     = operatorsettings.ProviderCatalog
	ProviderModelPrompt = operatorsettings.ProviderModelPrompt
	IDGenerator         = operatorsettings.IDGenerator
	ConfigDecoder       = operatorsettings.ConfigDecoder
	ConfigEncoder       = operatorsettings.ConfigEncoder
	ConfigLoader        = operatorsettings.ConfigLoader
	BackendScopeEnsurer = operatorsettings.BackendScopeEnsurer
)
