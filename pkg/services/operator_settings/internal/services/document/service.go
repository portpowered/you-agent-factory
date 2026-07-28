// Package settingsdocument defines the Operator Settings-owned document
// capability. Consumers outside Operator Settings use the Operator Settings root
// service instead of this parent-private subservice contract.
package settingsdocument

import (
	"context"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

// Service is the singular document subservice contract for the Operator
// Settings root. It owns strict load, semantic update, and atomic persist of
// the operator document behind the parent-private boundary.
type Service interface {
	LoadDocument(operatorsettings.LoadDocumentRequest) (operatorsettings.LoadDocumentResult, error)
	MergeDocumentProviderModel(
		operatorsettings.Document,
		operatorsettings.DocumentProviderModelUpdate,
	) (operatorsettings.Document, error)
	ApplyDocumentUpdate(
		operatorsettings.ApplyDocumentUpdateRequest,
	) (operatorsettings.ApplyDocumentUpdateResult, error)
	PersistDocument(context.Context, operatorsettings.PersistDocumentRequest) error
}
