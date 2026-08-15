import { describe, expect, it } from "bun:test";

import type { DashboardCardStateContext } from "./dashboard-card-state";
import {
  dashboardSnapshotHasRecords,
  resolveDashboardCardAsyncState,
  resolveDashboardCardContentState,
} from "./dashboard-card-state";

const liveContext: DashboardCardStateContext = {
  hasAuthoritativeSnapshot: true,
  recoveryPending: false,
  streamStatus: "live",
  timelineMode: "current",
};

describe("dashboard card async projection", () => {
  it("keeps loading separate from an authoritative known-empty input", () => {
    expect(
      resolveDashboardCardContentState({
        authoritative: false,
        hasRecords: false,
      }),
    ).toBe("loading");
    expect(
      resolveDashboardCardContentState({
        authoritative: true,
        hasRecords: false,
      }),
    ).toBe("known-empty");
    expect(
      resolveDashboardCardContentState({
        authoritative: true,
        hasRecords: true,
      }),
    ).toBe("populated");
  });

  it("distinguishes known-empty, reconnecting, and stale card states", () => {
    expect(
      resolveDashboardCardAsyncState({
        ...liveContext,
        content: "known-empty",
      }),
    ).toEqual({
      content: "known-empty",
      freshness: "fresh",
      temporal: "live",
    });

    expect(
      resolveDashboardCardAsyncState({
        ...liveContext,
        content: "populated",
        streamStatus: "reconnecting",
      }).freshness,
    ).toBe("reconnecting");

    expect(
      resolveDashboardCardAsyncState({
        ...liveContext,
        content: "known-empty",
        streamStatus: "reconnecting",
      }),
    ).toMatchObject({ content: "loading", freshness: "reconnecting" });

    expect(
      resolveDashboardCardAsyncState({
        ...liveContext,
        content: "populated",
        streamStatus: "offline",
      }).freshness,
    ).toBe("stale");

    expect(
      resolveDashboardCardAsyncState({
        ...liveContext,
        content: "known-empty",
        streamStatus: "offline",
      }),
    ).toMatchObject({ content: "error", freshness: "failed" });
  });

  it("reports checkpoint recovery and historical temporal mode separately", () => {
    expect(
      resolveDashboardCardAsyncState({
        ...liveContext,
        content: "populated",
        recoveryPending: true,
        streamStatus: "connecting",
      }).freshness,
    ).toBe("recovering");

    expect(
      resolveDashboardCardAsyncState({
        ...liveContext,
        content: "populated",
        timelineMode: "fixed",
      }).temporal,
    ).toBe("historical");
  });

  it("uses session runtime data as the work totals authority", () => {
    expect(
      dashboardSnapshotHasRecords({
        factory_state: "IDLE",
        runtime: {
          in_flight_dispatch_count: 0,
          session: {
            completed_count: 0,
            dispatched_count: 0,
            failed_count: 0,
            has_data: false,
          },
        },
        tick_count: 0,
        topology: {
          edges: [],
          submit_work_types: [],
          workstation_node_ids: [],
          workstation_nodes_by_id: {},
        },
        uptime_seconds: 0,
      }),
    ).toBe(false);
  });
});
