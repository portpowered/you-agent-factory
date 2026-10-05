package customer_lifecycles_test

import "testing"

// TestMCPProtocolJourneys owns its immutable fixture until all customer scenarios finish.
func TestMCPProtocolJourneys(t *testing.T) {
	t.Parallel()
	resettransportmcpprotocol0State()
	initializeTransportmcpprotocolFixture(t)
	t.Run("TestMCPMalformedParametersReturnInvalidParams", testTransportmcpprotocolMCPMalformedParametersReturnInvalidParams)
	t.Run("TestMCPMissingFactorySessionReturnsCanonicalNotFound", testTransportmcpprotocolMCPMissingFactorySessionReturnsCanonicalNotFound)
	t.Run("TestMCPServerShutdownClosesStdioCleanly", testTransportmcpprotocolMCPServerShutdownClosesStdioCleanly)
}
