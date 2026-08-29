package agy

import (
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
)

func (fixture *agySharedProcessFixture) registerRoleColdWatchRoutes(t *testing.T) {
	t.Helper()
	fixture.addRoleRoute(t, "role-cold-watch-complete", agyColdWatchFactoryName,
		"clip-fixture.mp4", agyColdWatchCompleteReportTrace(t))
	fixture.addRoleRoute(t, "role-cold-watch-incomplete-video-watch", agyColdWatchFactoryName,
		"clip-fixture.mp4", readAgyGoldenAsset(t, "agy-trace-video-watch.stream.jsonl"))
	fixture.addRoleRoute(t, "role-cold-watch-incomplete-groundtruth-video", agyColdWatchFactoryName,
		"groundtruth-fixture.mp4", readAgyGoldenAsset(t, "agy-trace-groundtruth-verbose.stream.jsonl"))
	fixture.addRoleRoute(t, "role-cold-watch-missing-file", agyColdWatchFactoryName,
		"does-not-exist-xyz.mp4", readAgyGoldenAsset(t, "agy-trace-missing-file.stream.jsonl"))
}

func (fixture *agySharedProcessFixture) registerRoleClipQARoutes(t *testing.T) {
	t.Helper()
	fixture.addRoleRoute(t, "role-clipqa-pass", agyClipQAFactoryName, "clip-fixture.mp4",
		alignAgyClipQAReplaySchema(t, readAgyGoldenAsset(t, "agy-trace-clipqa-schema.stream.jsonl")))
	verdict := validAgyClipQAVerdictPayload()
	verdict["action_completed"] = false
	verdict["spec_deviations"] = []string{"the specified action did not finish"}
	verdict["verdict"] = "reroll"
	verdict["confidence"] = 0.82
	fixture.addRoleRoute(t, "role-clipqa-reroll", agyClipQAFactoryName, "clip-fixture.mp4",
		alignAgyClipQAReplaySchema(t, agyClipQAVerdictTrace(t, verdict)))
}

func (fixture *agySharedProcessFixture) registerRoleClipQAInvalidRoutes(t *testing.T) {
	t.Helper()
	for _, test := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{name: "confidence below zero", mutate: func(verdict map[string]any) { verdict["confidence"] = -0.01 }},
		{name: "confidence above one", mutate: func(verdict map[string]any) { verdict["confidence"] = 1.01 }},
		{name: "pass with incomplete action", mutate: func(verdict map[string]any) { verdict["action_completed"] = false }},
		{name: "pass with specification deviation", mutate: func(verdict map[string]any) { verdict["spec_deviations"] = []string{"wrong action"} }},
		{name: "pass with temporal artifact", mutate: func(verdict map[string]any) { verdict["temporal_artifacts"] = []string{"flash"} }},
		{name: "pass with unexpected speech", mutate: func(verdict map[string]any) { verdict["unexpected_speech"] = true }},
		{name: "reroll with provider failure status", mutate: func(verdict map[string]any) {
			verdict["action_completed"] = false
			verdict["verdict"] = "reroll"
			verdict["status"] = "error"
		}},
	} {
		verdict := validAgyClipQAVerdictPayload()
		test.mutate(verdict)
		selector := "role-clipqa-invalid-" + agyRouteSlug(test.name)
		fixture.addRoleRoute(t, selector, agyClipQAFactoryName, "clip-fixture.mp4",
			alignAgyClipQAReplaySchema(t, agyClipQAVerdictTrace(t, verdict)))
	}
}

func (fixture *agySharedProcessFixture) registerRoleClipQAEdgeRoutes(t *testing.T) {
	t.Helper()
	fixture.addRoleRoute(t, "role-clipqa-missing-file", agyClipQAFactoryName,
		"does-not-exist-xyz.mp4",
		alignAgyClipQAReplaySchema(t, readAgyGoldenAsset(t, "agy-trace-missing-file.stream.jsonl")))
	fixture.addRoleRoute(t, "role-clipqa-schema-invalid", agyClipQAFactoryName,
		"clip-fixture.mp4", readAgyGoldenAsset(t, "agy-trace-structured.json"))
	fixture.addRoleRouteWithOutcomes(t, "role-clipqa-provider-failure", agyClipQAFactoryName,
		"clip-fixture.mp4", agySharedCommandOutcome{
			result: platformprocess.CommandResult{Stderr: []byte("agy unavailable"), ExitCode: 17},
		})
}
