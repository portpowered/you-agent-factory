package wire

import (
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/work/internal/requestadmission"
	owner "github.com/portpowered/infinite-you/pkg/services/work/internal/services/request_preparation/internal/service"
)

func NewContentPreparation(inner requestadmission.ContentPreparation) work.ContentPreparation {
	return owner.ContentPreparationAdapter{Inner: inner}
}
func NewRequestContentBridge(content work.ContentPreparation) requestadmission.ContentPreparation {
	return owner.RequestPreparationContentBridge{Content: content}
}
func NewRequestPreparationService(inner requestadmission.RequestPreparationService) work.RequestPreparationService {
	return owner.RequestPreparationServiceAdapter{Inner: inner}
}
