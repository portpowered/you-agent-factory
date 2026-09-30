package service

import factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"

// SessionInspectionService exposes the process-owned durable inspection
// capability to transport composition.
func (r *Root) SessionInspectionService() factorysessions.SessionInspectionService {
	if r == nil || r.Assembly == nil {
		return nil
	}
	inspection, _ := r.Assembly.SessionGateway.(factorysessions.SessionInspectionService)
	return inspection
}
