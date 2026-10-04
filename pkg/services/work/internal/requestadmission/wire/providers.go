package wire

import "github.com/portpowered/infinite-you/pkg/services/work/internal/requestadmission"

func NewContentPolicy() requestadmission.ContentPreparation {
	return requestadmission.NewContentPreparation()
}
func NewRequestPolicy(content requestadmission.ContentPreparation) requestadmission.RequestPreparationService {
	return requestadmission.NewRequestPreparationService(content)
}
