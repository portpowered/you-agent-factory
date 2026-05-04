package submission

import (
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// WorkRequestFromSubmitRequests wraps internal normalized submit records in the
// canonical FACTORY_REQUEST_BATCH contract.
func WorkRequestFromSubmitRequests(requests []interfaces.SubmitRequest) interfaces.WorkRequest {
	return factory.WorkRequestFromSubmitRequests(requests)
}
