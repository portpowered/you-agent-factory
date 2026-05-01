import { describe, expect, it } from "vitest";

import { FACTORY_EVENT_TYPES } from "../api/events";
import {
  buildReplayFixtureTimelineSnapshot,
  loadReplayFixtureEvents,
  replayFixtureCatalog,
  REPLAY_FIXTURE_DIRECTORY,
} from "./replay-fixtures";

describe("replay fixture helpers", () => {
  it("loads typed events from the canonical replay fixture catalog", () => {
    const events = loadReplayFixtureEvents("runtimeDetails");

    expect(replayFixtureCatalog.runtimeDetails.fileName).toBe("runtime-details-replay.jsonl");
    expect(replayFixtureCatalog.failureAnalysis.surfaces).toContain("failure-rendering");
    expect(replayFixtureCatalog.graphStateSmoke.surfaces).toContain("graph-state");
    expect(REPLAY_FIXTURE_DIRECTORY).toBe("integration/fixtures");
    expect(events[0]?.type).toBe(FACTORY_EVENT_TYPES.initialStructureRequest);
    expect(events.every((event) => typeof event.id === "string")).toBe(true);
  });

  it("builds timeline snapshots through the canonical replay projection seam", () => {
    const snapshot = buildReplayFixtureTimelineSnapshot(
      "runtimeConfigInterfaceConsolidation",
      8,
    );

    expect(Object.keys(snapshot.tracesByWorkID).sort()).toEqual(
      expect.arrayContaining(["work-task-1", "work-task-2"]),
    );
    expect(snapshot.dashboard.runtime.workstation_requests_by_dispatch_id).toHaveProperty(
      "17c38f40-de4e-4d5f-bd44-649a2bf4a284",
    );
  });
});
