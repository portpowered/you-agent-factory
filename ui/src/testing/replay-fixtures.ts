import baselineReplayFixtureText from "../../integration/fixtures/event-stream-replay.jsonl?raw";
import failureAnalysisReplayFixtureText from "../../integration/fixtures/failure-analysis-replay.jsonl?raw";
import graphStateReplayFixtureText from "../../integration/fixtures/graph-state-smoke-replay.jsonl?raw";
import runtimeConfigReplayFixtureText from "../../integration/fixtures/event-stream-replay-2.jsonl?raw";
import runtimeDetailsReplayFixtureText from "../../integration/fixtures/runtime-details-replay.jsonl?raw";
import weirdNumberSummaryReplayFixtureText from "../../integration/fixtures/weird-number-summary-replay.jsonl?raw";

import type { FactoryEvent } from "../api/events";
import {
  buildFactoryTimelineSnapshot,
  type FactoryTimelineSnapshot,
} from "../state/factoryTimelineStore";
import { replayFixtureCatalog, type ReplayFixtureID } from "./replay-fixture-catalog";

export { replayFixtureCatalog, type ReplayFixtureID } from "./replay-fixture-catalog";

export const REPLAY_FIXTURE_DIRECTORY = "integration/fixtures";

const replayFixtureTexts: Record<ReplayFixtureID, string> = {
  baseline: baselineReplayFixtureText,
  failureAnalysis: failureAnalysisReplayFixtureText,
  graphStateSmoke: graphStateReplayFixtureText,
  runtimeConfigInterfaceConsolidation: runtimeConfigReplayFixtureText,
  runtimeDetails: runtimeDetailsReplayFixtureText,
  weirdNumberSummary: weirdNumberSummaryReplayFixtureText,
};

export function parseReplayFixtureEvents(fixtureText: string): FactoryEvent[] {
  return fixtureText
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter((line) => line.length > 0)
    .map((line) => JSON.parse(line) as FactoryEvent);
}

export function loadReplayFixtureEvents(fixtureID: ReplayFixtureID): FactoryEvent[] {
  const fixtureText = replayFixtureTexts[fixtureID];
  if (!fixtureText) {
    throw new Error(`Unknown replay fixture: ${fixtureID}`);
  }

  return parseReplayFixtureEvents(fixtureText);
}

export function buildReplayFixtureTimelineSnapshot(
  fixtureID: ReplayFixtureID,
  selectedTick?: number,
): FactoryTimelineSnapshot {
  const events = loadReplayFixtureEvents(fixtureID);
  const resolvedTick =
    selectedTick ?? events.reduce((latestTick, event) => Math.max(latestTick, event.context.tick), 0);

  return buildFactoryTimelineSnapshot(events, resolvedTick);
}
