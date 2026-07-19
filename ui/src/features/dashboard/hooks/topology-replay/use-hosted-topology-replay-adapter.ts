import type {
  FactoryTimelineScrubberState,
  FactoryTopologyReplayProjection,
  FactoryTopologyReplayState,
} from "@you-agent-factory/factory-visualizers";
import { useCallback, useMemo } from "react";

import type { DashboardStreamState } from "../../../../api/dashboard/types";
import {
  factoryTimelineEntryKey,
  type FactoryTimelineEntryState,
  normalizeStreamDerivedCacheIdentity,
  type StreamDerivedCacheIdentity,
  useFactoryTimelineStore,
} from "../../../timeline/public";
import { useDashboardStreamStore } from "../../state/dashboardStreamStore";

export type HostedTopologyReplayAdapterState =
  | {
      identity: null;
      status: "not-ready";
      streamState: DashboardStreamState;
      timelineState: Extract<
        FactoryTimelineScrubberState,
        { status: "unavailable" }
      >;
      topologyState: Exclude<FactoryTopologyReplayState, { status: "ready" }>;
    }
  | {
      identity: StreamDerivedCacheIdentity;
      projection: FactoryTopologyReplayProjection;
      status: "ready";
      streamState: DashboardStreamState;
      timelineState: Extract<
        FactoryTimelineScrubberState,
        { status: "available" }
      >;
      topologyState: FactoryTopologyReplayState;
    };

export interface HostedTopologyReplayAdapter
  extends HostedTopologyReplayAdapterActions {
  state: HostedTopologyReplayAdapterState;
}

interface HostedTopologyReplayAdapterActions {
  followLatest: () => void;
  selectTick: (tick: number) => void;
}

export function selectHostedTopologyReplayAdapterState(
  identity: StreamDerivedCacheIdentity | null | undefined,
  entry: FactoryTimelineEntryState | undefined,
  streamState: DashboardStreamState,
): HostedTopologyReplayAdapterState {
  const exactIdentity = normalizeStreamDerivedCacheIdentity(identity);
  if (
    !exactIdentity ||
    !entry ||
    !identitiesMatch(exactIdentity, entry.identity)
  ) {
    return {
      identity: null,
      status: "not-ready",
      streamState,
      timelineState: { status: "unavailable" },
      topologyState: {
        status:
          streamState.status === "offline" ||
          streamState.status === "recovery_failed"
            ? "failed"
            : "loading",
      },
    };
  }

  const selectedWorld = entry.worldViewCache[entry.selectedTick];
  if (!selectedWorld) {
    return {
      identity: null,
      status: "not-ready",
      streamState,
      timelineState: { status: "unavailable" },
      topologyState: { status: "loading" },
    };
  }

  const projection = {
    activity: selectedWorld.factoryReplay.activity,
    load: selectedWorld.factoryReplay.load,
    topology: selectedWorld.factoryReplay.topology,
  } satisfies FactoryTopologyReplayProjection;
  const { earliestTick, latestTick } = timelineBounds(entry);
  return {
    identity: exactIdentity,
    projection,
    status: "ready",
    streamState,
    timelineState: {
      earliestTick,
      latestTick,
      mode: entry.mode === "current" ? "current" : "history",
      selectedTick: entry.selectedTick,
      status: "available",
    },
    topologyState:
      projection.topology.nodes.length === 0
        ? { status: "empty" }
        : { projection, status: "ready" },
  };
}

export function useHostedTopologyReplayAdapter(): HostedTopologyReplayAdapter {
  const identity = useDashboardStreamStore(
    (state) => state.resolvedStreamIdentity,
  );
  const streamState = useDashboardStreamStore((state) => state.streamState);
  const exactIdentity = normalizeStreamDerivedCacheIdentity(identity);
  const entryKey = exactIdentity
    ? factoryTimelineEntryKey(exactIdentity)
    : null;
  const entry = useFactoryTimelineStore((state) =>
    entryKey ? state.entriesByKey[entryKey] : undefined,
  );
  const selectTickForEntry = useFactoryTimelineStore(
    (state) => state.selectTickForEntry,
  );
  const setCurrentModeForEntry = useFactoryTimelineStore(
    (state) => state.setCurrentModeForEntry,
  );

  const selectTick = useCallback(
    (tick: number) => {
      if (exactIdentity) {
        selectTickForEntry(exactIdentity, tick);
      }
    },
    [exactIdentity, selectTickForEntry],
  );
  const followLatest = useCallback(() => {
    if (exactIdentity) {
      setCurrentModeForEntry(exactIdentity);
    }
  }, [exactIdentity, setCurrentModeForEntry]);
  const state = useMemo(
    () =>
      selectHostedTopologyReplayAdapterState(exactIdentity, entry, streamState),
    [entry, exactIdentity, streamState],
  );

  return useMemo(
    () => ({ followLatest, selectTick, state }),
    [followLatest, selectTick, state],
  );
}

function identitiesMatch(
  left: StreamDerivedCacheIdentity,
  right: StreamDerivedCacheIdentity,
): boolean {
  return factoryTimelineEntryKey(left) === factoryTimelineEntryKey(right);
}

function timelineBounds(entry: FactoryTimelineEntryState): {
  earliestTick: number;
  latestTick: number;
} {
  const ticks = new Set<number>([entry.latestTick, entry.selectedTick]);
  for (const event of entry.events) {
    ticks.add(event.context.tick);
  }
  for (const cachedTick of Object.keys(entry.worldViewCache)) {
    const tick = Number(cachedTick);
    if (Number.isSafeInteger(tick) && tick >= 0) {
      ticks.add(tick);
    }
  }
  const orderedTicks = [...ticks]
    .filter((tick) => Number.isSafeInteger(tick) && tick >= 0)
    .sort((left, right) => left - right);
  return {
    earliestTick: orderedTicks[0] ?? 0,
    latestTick: Math.max(entry.latestTick, orderedTicks.at(-1) ?? 0),
  };
}
