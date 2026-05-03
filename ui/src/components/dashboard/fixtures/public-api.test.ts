import { describe, expect, it } from "vitest";

import { failureAnalysisTimelineEvents } from "./failure-analysis-events";
import {
  activeWorkRuntimeOverlay,
  buildDashboardSnapshotFixture,
  dashboardRuntimeOverlays,
  dashboardSemanticSnapshotFixtures,
} from "./runtime";
import {
  runtimeDetailsFixtureIDs,
  runtimeDetailsTimelineEvents,
} from "./runtime-details-events";
import { runtimeDetailsBackendWorkstationRequestsByDispatchID } from "./runtime-details-backend-world-view";
import {
  scriptDashboardIntegrationBackendWorkstationRequestsByDispatchID,
} from "./script-dashboard-integration-backend-world-view";
import {
  scriptDashboardIntegrationFixtureIDs,
  scriptDashboardIntegrationTimelineEvents,
} from "./script-dashboard-integration-events";
import {
  dashboardTopologyFixtures,
  mediumBranchingDashboardTopology,
  oneNodeDashboardTopology,
  twentyNodeDashboardTopology,
} from "./topologies";
import { dashboardWorkstationRequestFixtures } from "./workstation-requests";

describe("dashboard fixture catalog", () => {
  it("exports documented fixture catalog entries for direct Storybook and Vitest imports", () => {
    const activeSnapshot = buildDashboardSnapshotFixture(mediumBranchingDashboardTopology, [
      activeWorkRuntimeOverlay,
    ]);

    expect(dashboardTopologyFixtures.oneNode).toBe(oneNodeDashboardTopology);
    expect(dashboardTopologyFixtures.mediumBranching).toBe(mediumBranchingDashboardTopology);
    expect(dashboardTopologyFixtures.twentyNode).toBe(twentyNodeDashboardTopology);
    expect(Object.keys(dashboardRuntimeOverlays)).toEqual([
      "activeWork",
      "retryAttempt",
      "failedOutcome",
      "rejectedOutcome",
    ]);
    expect(activeSnapshot.runtime.active_workstation_node_ids).toContain("review");
    expect(dashboardSemanticSnapshotFixtures.activeWork.runtime.in_flight_dispatch_count).toBe(1);
    expect(failureAnalysisTimelineEvents.map((event) => event.type)).toContain(
      "DISPATCH_RESPONSE",
    );
    expect(runtimeDetailsTimelineEvents.map((event) => event.type)).toContain(
      "DISPATCH_RESPONSE",
    );
    expect(scriptDashboardIntegrationTimelineEvents.map((event) => event.type)).toContain(
      "SCRIPT_REQUEST",
    );
    expect(scriptDashboardIntegrationTimelineEvents.map((event) => event.type)).toContain(
      "SCRIPT_RESPONSE",
    );
    expect(
      runtimeDetailsBackendWorkstationRequestsByDispatchID[
        runtimeDetailsFixtureIDs.failedDispatchID
      ]?.response?.failure_reason,
    ).toBe(runtimeDetailsFixtureIDs.failedFailureReason);
    expect(
      scriptDashboardIntegrationBackendWorkstationRequestsByDispatchID[
        scriptDashboardIntegrationFixtureIDs.failedDispatchID
      ]?.response?.failure_reason,
    ).toBe(scriptDashboardIntegrationFixtureIDs.failedFailureReason);
    expect(runtimeDetailsFixtureIDs.completedSystemPromptHash).toMatch(/^sha256:/);
    expect(Object.keys(dashboardWorkstationRequestFixtures)).toEqual([
      "noResponse",
      "ready",
      "rejected",
      "errored",
      "scriptFailed",
      "scriptPending",
      "scriptSuccess",
    ]);
    expect(
      new Set(
        Object.values(dashboardWorkstationRequestFixtures).map((request) => request.dispatch_id),
      ).size,
    ).toBe(Object.keys(dashboardWorkstationRequestFixtures).length);
    expect(dashboardWorkstationRequestFixtures.ready.request_id).toBe("request-ready-story");
    expect(dashboardWorkstationRequestFixtures.rejected.outcome).toBe("REJECTED");
  });
});

