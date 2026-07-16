package runtime_api

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/internal/functionalevidence"
)

func TestCanonicalSessionFactoryEventStream_StreamsHistoryAtCustomerSSEBoundary(t *testing.T) {
	dir := support.ScaffoldFactory(t, simplePipelineConfig())
	server := startFunctionalServer(t, dir, true, factory.WithServiceMode())

	stream := openDefaultSessionFactoryEventHTTPStream(t, server.URL())
	requireFunctionalEventStreamPrelude(t, stream)

	functionalevidence.Covers(t, "sse/getEventsBySessionId")
}
