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
    expect(replayFixtureCatalog.weirdNumberSummary.fileName).toBe(
      "weird-number-summary-replay.jsonl",
    );
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

  it("projects canonical workstation behavior from maintained replay fixtures", () => {
    const snapshot = buildReplayFixtureTimelineSnapshot(
      "runtimeConfigInterfaceConsolidation",
      0,
    );

    expect(
      snapshot.dashboard.topology.workstation_nodes_by_id.cleaner?.workstation_kind,
    ).toBe("CRON");
    expect(
      snapshot.dashboard.topology.workstation_nodes_by_id.process?.workstation_kind,
    ).toBe("REPEATER");
  });

  it("replays the weird-number-summary regression through the canonical timeline seam", () => {
    const snapshot = buildReplayFixtureTimelineSnapshot("weirdNumberSummary", 4);

    expect(snapshot.dashboard.runtime.session.dispatched_count).toBe(1);
    expect(snapshot.dashboard.runtime.session.completed_count).toBe(0);
    expect(snapshot.dashboard.runtime.session.failed_count).toBe(3);
    expect(snapshot.dashboard.runtime.session.failed_by_work_type).toEqual({
      story: 3,
    });
    expect(snapshot.dashboard.runtime.session.failed_work_labels).toEqual([
      "Blocked Story",
      "Rejected Story",
      "Reworked Story",
    ]);
  });
});
