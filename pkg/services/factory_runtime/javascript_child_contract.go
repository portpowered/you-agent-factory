package factory

import orchestratorcontract "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/orchestratorcontract"

const (
	FieldPrompt          = orchestratorcontract.FieldPrompt
	FieldLabel           = orchestratorcontract.FieldLabel
	FieldPreset          = orchestratorcontract.FieldPreset
	FieldModelProvider   = orchestratorcontract.FieldModelProvider
	FieldModel           = orchestratorcontract.FieldModel
	FieldReasoningEffort = orchestratorcontract.FieldReasoningEffort
	FieldResourceID      = orchestratorcontract.FieldResourceID
	FieldSchema          = orchestratorcontract.FieldSchema
	FieldPermissions     = orchestratorcontract.FieldPermissions
	FieldSkipPermissions = orchestratorcontract.FieldSkipPermissions
)

type (
	JavaScriptChildPermission = orchestratorcontract.JavaScriptChildPermission
	JavaScriptChildSpec       = orchestratorcontract.JavaScriptChildSpec
)

const (
	JavaScriptChildPermissionDefault         = orchestratorcontract.JavaScriptChildPermissionDefault
	JavaScriptChildPermissionSkipPermissions = orchestratorcontract.JavaScriptChildPermissionSkipPermissions
)

var (
	JavaScriptChildSupportedFields  = orchestratorcontract.JavaScriptChildSupportedFields
	IsJavaScriptChildSupportedField = orchestratorcontract.IsJavaScriptChildSupportedField
	NormalizeJavaScriptChild        = orchestratorcontract.NormalizeJavaScriptChild
)
