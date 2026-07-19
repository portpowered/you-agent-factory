import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import type { DashboardStreamState } from "../../../../api/dashboard/types";
import type { FactoryEvent } from "../../../../api/events";
import { FACTORY_EVENT_TYPES } from "../../../../api/events";
import type { StreamDerivedCacheIdentity } from "../../../timeline/public";
import { useFactoryTimelineStore } from "../../../timeline/public";
import {
  createDefaultDashboardStreamState,
  useDashboardStreamStore,
} from "../../state/dashboardStreamStore";
import {
  selectHostedTopologyReplayAdapterState,
  useHostedTopologyReplayAdapter,
} from "./use-hosted-topology-replay-adapter";

const liveStreamState: DashboardStreamState = {
  message: "Live",
  status: "live",
};

function streamIdentity(
  factorySessionID: string,
  streamGenerationID = "generation-1",
): StreamDerivedCacheIdentity {
  return {
    backendScopeID: "backend-a",
    factorySessionID,
    logicalSessionKeyID: "shared-logical-session",
    streamGenerationID,
  };
}

function topologyEvent(
  id: string,
  tick: number,
  workTypeName: string,
  initialStateName: string,
): FactoryEvent {
  return {
    context: {
      eventTime: `2026-07-18T20:00:0${tick}.000Z`,
      sequence: tick,
      tick,
    },
    id,
    payload: {
      factory: {
        name: `${workTypeName} factory`,
        workers: [],
        workTypes: [
          {
            name: workTypeName,
            states: [
              { name: initialStateName, type: "INITIAL" },
              { name: "done", type: "TERMINAL" },
            ],
          },
        ],
        workstations: [],
      },
    },
    type: FACTORY_EVENT_TYPES.initialStructureRequest,
  };
}

function workEvent(
  id: string,
  tick: number,
  workTypeName: string,
): FactoryEvent {
  return {
    context: {
      eventTime: `2026-07-18T20:00:0${tick}.000Z`,
      sequence: tick,
      tick,
    },
    id,
    payload: {
      type: "FACTORY_REQUEST_BATCH",
      works: [
        {
          name: `${workTypeName} work`,
          workId: `${workTypeName}-work-1`,
          workTypeName,
        },
      ],
    },
    type: FACTORY_EVENT_TYPES.workRequest,
  };
}

afterEach(() => {
  act(() => {
    useFactoryTimelineStore.getState().reset();
    useDashboardStreamStore.setState({
      backendRuntimeCacheScope: null,
      resolvedStreamIdentity: null,
      streamState: createDefaultDashboardStreamState(),
    });
  });
});

describe("hosted topology replay adapter", () => {
  it("maps only the resolved exact entry and clears projection on identity changes", () => {
    const sessionA = streamIdentity("session-a-uuid");
    const sessionB = streamIdentity("session-b-uuid");
    const missingSession = streamIdentity("session-missing-uuid");

    act(() => {
      const timeline = useFactoryTimelineStore.getState();
      timeline.appendEventsForEntry(sessionA, [
        topologyEvent("topology-a", 1, "story-a", "queued-a"),
        workEvent("work-a", 3, "story-a"),
      ]);
      timeline.appendEventsForEntry(sessionB, [
        topologyEvent("topology-b", 5, "story-b", "queued-b"),
      ]);
      useDashboardStreamStore.setState({
        resolvedStreamIdentity: sessionA,
        streamState: liveStreamState,
      });
    });

    const { result } = renderHook(() => useHostedTopologyReplayAdapter());

    expect(result.current.state).toMatchObject({
      identity: sessionA,
      status: "ready",
      timelineState: {
        earliestTick: 1,
        latestTick: 3,
        mode: "current",
        selectedTick: 3,
        status: "available",
      },
    });
    if (result.current.state.status !== "ready") {
      throw new Error("expected session A adapter state");
    }
    expect(
      result.current.state.projection.topology.nodes.map((node) => node.label),
    ).toContain("queued-a");
    expect(
      result.current.state.projection.load.workStateCounts.find(
        (count) => count.count === 1,
      ),
    ).toBeDefined();

    act(() => {
      useDashboardStreamStore.getState().setResolvedStreamIdentity(sessionB);
    });
    expect(result.current.state).toMatchObject({
      identity: sessionB,
      status: "ready",
      timelineState: { latestTick: 5, selectedTick: 5 },
    });
    if (result.current.state.status !== "ready") {
      throw new Error("expected session B adapter state");
    }
    expect(
      result.current.state.projection.topology.nodes.map((node) => node.label),
    ).toContain("queued-b");
    expect(
      result.current.state.projection.topology.nodes.map((node) => node.label),
    ).not.toContain("queued-a");

    act(() => {
      useDashboardStreamStore
        .getState()
        .setResolvedStreamIdentity(missingSession);
    });
    expect(result.current.state).toEqual({
      identity: null,
      status: "not-ready",
      streamState: liveStreamState,
      timelineState: { status: "unavailable" },
      topologyState: { status: "loading" },
    });
  });
});

describe("hosted topology replay adapter operations", () => {
  it("delegates fixed and current actions to one exact entry", () => {
    const sessionA = streamIdentity("session-a-uuid");
    const sessionB = streamIdentity("session-b-uuid");
    act(() => {
      const timeline = useFactoryTimelineStore.getState();
      timeline.appendEventsForEntry(sessionA, [
        topologyEvent("topology-a", 1, "story-a", "queued-a"),
        workEvent("work-a", 3, "story-a"),
      ]);
      timeline.appendEventsForEntry(sessionB, [
        topologyEvent("topology-b", 2, "story-b", "queued-b"),
        workEvent("work-b", 4, "story-b"),
      ]);
      useDashboardStreamStore.setState({
        resolvedStreamIdentity: sessionA,
        streamState: liveStreamState,
      });
    });
    const { result } = renderHook(() => useHostedTopologyReplayAdapter());

    act(() => result.current.selectTick(1));
    expect(
      useFactoryTimelineStore.getState().entryForIdentity(sessionA),
    ).toMatchObject({ mode: "fixed", selectedTick: 1 });
    expect(
      useFactoryTimelineStore.getState().entryForIdentity(sessionB),
    ).toMatchObject({ mode: "current", selectedTick: 4 });
    expect(result.current.state).toMatchObject({
      timelineState: { mode: "history", selectedTick: 1 },
    });

    act(() => result.current.followLatest());
    expect(
      useFactoryTimelineStore.getState().entryForIdentity(sessionA),
    ).toMatchObject({ mode: "current", selectedTick: 3 });
    expect(
      useFactoryTimelineStore.getState().entryForIdentity(sessionB),
    ).toMatchObject({ mode: "current", selectedTick: 4 });
  });

  it("maps missing identity to a non-ready host transport state", () => {
    const offline: DashboardStreamState = {
      message: "Connection unavailable",
      status: "offline",
    };

    expect(
      selectHostedTopologyReplayAdapterState(null, undefined, offline),
    ).toEqual({
      identity: null,
      status: "not-ready",
      streamState: offline,
      timelineState: { status: "unavailable" },
      topologyState: { status: "failed" },
    });
  });
});
