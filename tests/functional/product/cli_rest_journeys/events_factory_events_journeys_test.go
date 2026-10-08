package cli_rest_journeys_test

import "testing"

// TestFactoryEventCLIJourneys owns its immutable fixture until all customer scenarios finish.
func TestFactoryEventCLIJourneys(t *testing.T) {
	t.Parallel()
	reseteventsfactoryevents5State()
	initializeEventsfactoryeventsFixture(t)
	eventHost := newFactoryEventSessionHost(t)
	t.Run("TestFSCP01CanonicalReconnectAndArtifactReadsIndependentOfResponseEvents", testEventsfactoryeventsFSCP01CanonicalReconnectAndArtifactReadsIndependentOfResponseEvents)
	t.Run("TestFSCP01ResponseTeardownThenCanonicalReconnectAndArtifactReads", testEventsfactoryeventsFSCP01ResponseTeardownThenCanonicalReconnectAndArtifactReads)
	t.Run("TestAPIGetFactoryEventsReturnsOrderedDurableHistory", func(t *testing.T) {
		testEventsfactoryeventsAPIGetFactoryEventsReturnsOrderedDurableHistory(t, eventHost)
	})
	t.Run("TestAPIEventCursorReturnsOnlyNewerEvents", func(t *testing.T) { testEventsfactoryeventsAPIEventCursorReturnsOnlyNewerEvents(t, eventHost) })
	t.Run("TestAPIInvalidEventCursorReturnsTypedError", func(t *testing.T) { testEventsfactoryeventsAPIInvalidEventCursorReturnsTypedError(t, eventHost) })
	t.Run("TestAPISubmitWorkEmitsCanonicalTraceAwareBatchEvent", func(t *testing.T) {
		testEventsfactoryeventsAPISubmitWorkEmitsCanonicalTraceAwareBatchEvent(t, eventHost)
	})
	t.Run("TestFactoryEventStreamIsOrderedAndClosesAtSessionTermination", func(t *testing.T) {
		testEventsfactoryeventsFactoryEventStreamIsOrderedAndClosesAtSessionTermination(t, eventHost)
	})
	t.Run("TestFactoryEventStreamReconnectHasNoGapOrDuplicate", func(t *testing.T) {
		testEventsfactoryeventsFactoryEventStreamReconnectHasNoGapOrDuplicate(t, eventHost)
	})
	t.Run("TestCanonicalTopologySnapshotsPreservePublicIdentityAndResourceEvidence", testEventsfactoryeventsCanonicalTopologySnapshotsPreservePublicIdentityAndResourceEvidence)
	t.Run("TestFactoryWebhooksInitialRecordedSessions", testEventsfactoryeventsFactoryWebhooksInitialRecordedSessions)
	t.Run("TestFactoryWebhooksRecordedFaultsAndPeerContinuation", testEventsfactoryeventsFactoryWebhooksRecordedFaultsAndPeerContinuation)
	t.Run("TestFactoryWebhooksInitialRecordedDispatchFailure", testEventsfactoryeventsFactoryWebhooksInitialRecordedDispatchFailure)
	t.Run("TestFactoryWebhooksRunThroughRootProcess", testEventsfactoryeventsFactoryWebhooksRunThroughRootProcess)

}
