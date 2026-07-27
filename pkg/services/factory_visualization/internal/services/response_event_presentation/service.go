// Package responseeventpresentation defines the Factory Visualization-owned
// response/event presentation capability. Consumers outside Factory
// Visualization use the Visualization root presentation seam instead of this
// parent-private subservice contract.
package responseeventpresentation

import (
	"io"

	"github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/contracts"
)

// Service owns best-effort/lossless output queues, drain policy, and
// Factory-event stream serialization behind the Visualization root facade.
type Service interface {
	OpenBestEffortOutput(io.Writer) contracts.Output
	OpenLosslessOutput(io.Writer) contracts.Output
	OpenBestEffortFactoryEventStream(io.Writer, contracts.FactoryEventEncoder) contracts.FactoryEventStream
	OpenLosslessFactoryEventStream(io.Writer, contracts.FactoryEventEncoder) contracts.FactoryEventStream
}
