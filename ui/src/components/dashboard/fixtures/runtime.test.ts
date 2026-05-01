import { describe, expect, it } from "vitest";

import {
  activeWorkRuntimeOverlay,
  buildDashboardSnapshotFixture,
  dashboardRuntimeOverlays,
  failedOutcomeRuntimeOverlay,
  mediumBranchingDashboardTopology,
  rejectedOutcomeRuntimeOverlay,
  retryAttemptRuntimeOverlay,
} from ".";

describe("dashboard runtime fixtures", () => {
  it("applies active work overlays without mutating the base topology", () => {
    const originalTopologyJSON = JSON.stringify(mediumBranchingDashboardTopology);
    const snapshot = buildDashboardSnapshotFixture(mediumBranchingDashboardTopology, [
      activeWorkRuntimeOverlay,
    ]);

    expect(snapshot.topology).toBe(mediumBranchingDashboardTopology);
    expect(JSON.stringify(mediumBranchingDashboardTopology)).toBe(originalTopologyJSON);
    expect(snapshot.runtime.in_flight_dispatch_count).toBe(1);
    expect(snapshot.runtime.active_workstation_node_ids).toContain("review");
    expect(snapshot.runtime.active_executions_by_dispatch_id?.["dispatch-review-active"])
      .toMatchObject({
        workstation_node_id: "review",
        work_items: [{ work_id: "work-active-story" }],
      });
    expect(snapshot.runtime.current_work_items_by_place_id?.["story:implemented"]).toEqual([
      expect.objectContaining({ work_id: "work-active-story" }),
    ]);
    expect(snapshot.runtime.session.provider_sessions?.map((attempt) => attempt.outcome))
      .toEqual(["ACCEPTED"]);
  });

  it("builds retry, failure, and rejected snapshots with observable session outcomes", () => {
    const retrySnapshot = buildDashboardSnapshotFixture(mediumBranchingDashboardTopology, [
      retryAttemptRuntimeOverlay,
    ]);
    const failedSnapshot = buildDashboardSnapshotFixture(mediumBranchingDashboardTopology, [
      failedOutcomeRuntimeOverlay,
    ]);
    const rejectedSnapshot = buildDashboardSnapshotFixture(mediumBranchingDashboardTopology, [
      rejectedOutcomeRuntimeOverlay,
    ]);

    expect(retrySnapshot.runtime.session.provider_sessions?.map((attempt) => attempt.outcome))
      .toContain("RETRY");
    expect(failedSnapshot.runtime.session.failed_count).toBe(1);
    expect(failedSnapshot.runtime.session.failed_by_work_type).toEqual({ story: 1 });
    expect(failedSnapshot.runtime.session.failed_work_labels).toContain("Failed Story");
    expect(failedSnapshot.runtime.session.provider_sessions?.map((attempt) => attempt.outcome))
      .toContain("FAILED");
    expect(rejectedSnapshot.runtime.session.provider_sessions?.map((attempt) => attempt.outcome))
      .toContain("REJECTED");
  });

  it("composes semantic overlays against one shared topology", () => {
    const snapshot = buildDashboardSnapshotFixture(
      mediumBranchingDashboardTopology,
      Object.values(dashboardRuntimeOverlays),
    );

    expect(snapshot.topology.workstation_node_ids).toEqual(
      mediumBranchingDashboardTopology.workstation_node_ids,
    );
    expect(snapshot.runtime.active_dispatch_ids).toContain("dispatch-review-active");
    expect(snapshot.runtime.session.provider_sessions?.map((attempt) => attempt.outcome))
      .toEqual(["ACCEPTED", "RETRY", "FAILED", "REJECTED"]);
    expect(snapshot.runtime.session.failed_count).toBe(1);
  });
});
