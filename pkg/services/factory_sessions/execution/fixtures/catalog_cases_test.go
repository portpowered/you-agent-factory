package fixtures_test

import (
	fse "github.com/portpowered/infinite-you/pkg/services/factory_sessions/execution"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/execution/fixtures"
)

type stableFixtureIdentityCase struct {
	purpose         fixtures.FixtureScenarioPurpose
	scenarioID      string
	requestID       string
	sessionID       string
	lifecycleStatus fse.LifecycleStatus
	resultStatus    fse.ResultStatus
	projectionHash  string
	dispatchIDs     []string
	artifactIDs     []string
	eventIDs        []string
}

func stableFixtureIdentityCases() []stableFixtureIdentityCase {
	return append(stableFixtureIdentityCasesHead(), stableFixtureIdentityCasesTail()...)
}

func stableFixtureIdentityCasesHead() []stableFixtureIdentityCase {
	return []stableFixtureIdentityCase{
		{
			purpose:         fixtures.FixturePurposeValidationFailure,
			scenarioID:      fixtures.FixtureScenarioValidationFailure,
			requestID:       "req-missing-source-001",
			sessionID:       "dur-sess-missing-source-001",
			lifecycleStatus: fse.LifecycleStatusFailed,
			resultStatus:    fse.ResultStatusUnavailable,
			projectionHash:  "sha256:58291583771069f4b4572f667f24c5cd70294f70b2f01300a50dcc106608a8a7",
		},
		{
			purpose:         fixtures.FixturePurposeAsyncRunning,
			scenarioID:      fixtures.FixtureScenarioAsyncRunning,
			requestID:       "req-js-run-n-001",
			sessionID:       "dur-sess-js-run-n-001",
			lifecycleStatus: fse.LifecycleStatusRunning,
			resultStatus:    fse.ResultStatusPartial,
			projectionHash:  "sha256:5e5d07440546bcfe8c67a0b270b27bdaeec8158346a61ac98273441f60f2e0ca",
			dispatchIDs:     []string{"disp-js-001", "disp-js-002", "disp-js-003"},
			eventIDs: []string{
				"session-result-updated/dur-sess-js-run-n-001",
				"session-started/dur-sess-js-run-n-001",
			},
		},
		{
			purpose:         fixtures.FixturePurposeSyncSuccess,
			scenarioID:      fixtures.FixtureScenarioSyncSuccess,
			requestID:       "req-petri-success-001",
			sessionID:       "dur-sess-petri-success-001",
			lifecycleStatus: fse.LifecycleStatusSucceeded,
			resultStatus:    fse.ResultStatusFinal,
			projectionHash:  "sha256:80683379f0ad28cb98d0adce69606d3d7fa249df7e2dd45300517bd5be1cf064",
			dispatchIDs:     []string{"disp-petri-success-001"},
			artifactIDs:     []string{"art-petri-final-001"},
		},
		{
			purpose:         fixtures.FixturePurposeSyncTimeout,
			scenarioID:      fixtures.FixtureScenarioSyncTimeout,
			requestID:       "req-js-timeout-001",
			sessionID:       "dur-sess-js-timeout-001",
			lifecycleStatus: fse.LifecycleStatusRunning,
			resultStatus:    fse.ResultStatusNotReady,
			projectionHash:  "sha256:798e5ae557b537dd488032e7fb545f9a2bdd20a9e7e646d43ed1d258758d261c",
			dispatchIDs:     []string{"disp-js-timeout-001"},
		},
		{
			purpose:         fixtures.FixturePurposeFailedRecoverable,
			scenarioID:      fixtures.FixtureScenarioFailedRecoverable,
			requestID:       "req-js-interrupted-001",
			sessionID:       "dur-sess-js-interrupted-001",
			lifecycleStatus: fse.LifecycleStatusInterrupted,
			resultStatus:    fse.ResultStatusPartial,
			projectionHash:  "sha256:3b7a2fa4485d9f0e51f9dc7ad328cf6d390fd84d98fe06d3b00da0527573704f",
			dispatchIDs:     []string{"disp-js-interrupted-001", "disp-js-interrupted-002"},
		},
	}
}

func stableFixtureIdentityCasesTail() []stableFixtureIdentityCase {
	return []stableFixtureIdentityCase{
		{
			purpose:         fixtures.FixturePurposeDispatchInspection,
			scenarioID:      fixtures.FixtureScenarioDispatchInspection,
			requestID:       "req-petri-success-001",
			sessionID:       "dur-sess-petri-success-001",
			lifecycleStatus: fse.LifecycleStatusSucceeded,
			resultStatus:    fse.ResultStatusFinal,
			projectionHash:  "sha256:80683379f0ad28cb98d0adce69606d3d7fa249df7e2dd45300517bd5be1cf064",
			dispatchIDs:     []string{"disp-petri-success-001"},
			artifactIDs:     []string{"art-petri-final-001"},
		},
		{
			purpose:         fixtures.FixturePurposeArtifactInspection,
			scenarioID:      fixtures.FixtureScenarioArtifactInspection,
			requestID:       "req-js-paused-001",
			sessionID:       "dur-sess-js-paused-001",
			lifecycleStatus: fse.LifecycleStatusPaused,
			resultStatus:    fse.ResultStatusPartial,
			projectionHash:  "sha256:56cf36fbe81354e200dbb63c299c30de1d059a6d233fd6c977d956c6a646868c",
			dispatchIDs:     []string{"disp-js-pause-001", "disp-js-pause-002"},
			artifactIDs:     []string{"art-js-pause-001"},
		},
		{
			purpose:         fixtures.FixturePurposeEventReconnect,
			scenarioID:      fixtures.FixtureScenarioEventReconnect,
			requestID:       "req-js-run-n-001",
			sessionID:       "dur-sess-js-run-n-001",
			lifecycleStatus: fse.LifecycleStatusRunning,
			resultStatus:    fse.ResultStatusPartial,
			projectionHash:  "sha256:5e5d07440546bcfe8c67a0b270b27bdaeec8158346a61ac98273441f60f2e0ca",
			dispatchIDs:     []string{"disp-js-001", "disp-js-002", "disp-js-003"},
			eventIDs: []string{
				"session-result-updated/dur-sess-js-run-n-001",
				"session-started/dur-sess-js-run-n-001",
			},
		},
		{
			purpose:         fixtures.FixturePurposeLifecycleControl,
			scenarioID:      fixtures.FixtureScenarioLifecycleControl,
			requestID:       "req-js-paused-001",
			sessionID:       "dur-sess-js-paused-001",
			lifecycleStatus: fse.LifecycleStatusPaused,
			resultStatus:    fse.ResultStatusPartial,
			projectionHash:  "sha256:56cf36fbe81354e200dbb63c299c30de1d059a6d233fd6c977d956c6a646868c",
			dispatchIDs:     []string{"disp-js-pause-001", "disp-js-pause-002"},
			artifactIDs:     []string{"art-js-pause-001"},
		},
	}
}
