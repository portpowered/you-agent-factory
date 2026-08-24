package factory

import orchestratorcontract "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/orchestratorcontract"

const (
	FieldPrompt           = orchestratorcontract.FieldPrompt
	FieldLabel            = orchestratorcontract.FieldLabel
	FieldPreset           = orchestratorcontract.FieldPreset
	FieldExecutorProvider = orchestratorcontract.FieldExecutorProvider
	FieldModelProvider    = orchestratorcontract.FieldModelProvider
	FieldModel            = orchestratorcontract.FieldModel
	FieldReasoningEffort  = orchestratorcontract.FieldReasoningEffort
	FieldResourceID       = orchestratorcontract.FieldResourceID
	FieldSchema           = orchestratorcontract.FieldSchema
	FieldPermissions      = orchestratorcontract.FieldPermissions

	JavaScriptChildPermissionDefault         = orchestratorcontract.JavaScriptChildPermissionDefault
	JavaScriptChildPermissionSkipPermissions = orchestratorcontract.JavaScriptChildPermissionSkipPermissions
)

type (
	JavaScriptChildFieldDescriptor = orchestratorcontract.JavaScriptChildFieldDescriptor
	JavaScriptChildPermission      = orchestratorcontract.JavaScriptChildPermission
	JavaScriptChildSpec            = orchestratorcontract.JavaScriptChildSpec
)

var (
	JavaScriptChildFieldDescriptors        = orchestratorcontract.JavaScriptChildFieldDescriptors
	JavaScriptChildSupportedFields         = orchestratorcontract.JavaScriptChildSupportedFields
	IsJavaScriptChildSupportedField        = orchestratorcontract.IsJavaScriptChildSupportedField
	UnsupportedJavaScriptChildFieldMessage = orchestratorcontract.UnsupportedJavaScriptChildFieldMessage
	NormalizeJavaScriptChild               = orchestratorcontract.NormalizeJavaScriptChild
)
