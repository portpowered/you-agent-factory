import { create } from "zustand";

import type { FactoryEvent } from "../../../api/events";
import {
  buildFactoryTimelineProjection as buildProjectedTimelineProjection,
  buildFactoryTimelineSnapshot as buildProjectedTimelineSnapshot,
} from "./timeline/buildSnapshot";

export { resolveConfiguredWorkTypeName } from "./timeline/projectTopology";

import {
  advanceWorldStateFromCheckpoint,
  reconstructWorldState,
} from "./timeline/replayWorldState";
import { orderedEvents } from "./timeline/shared";

export type { WorldState } from "./timeline/types";

import {
  appendTimelineEvents,
  emptyTimelineState,
  type FactoryTimelineCheckpoint,
  type FactoryTimelineState,
  replaceTimelineEvents,
  restoreTimelineCheckpoint,
  selectTimelineTick,
  setTimelineCurrentMode,
  type TimelineStoreStateDeps,
} from "./timeline/storeState";
import type { WorldState } from "./timeline/types";

export type {
  FactoryTimelineCheckpoint,
  FactoryTimelineMode,
} from "./timeline/storeState";

export function buildFactoryTimelineSnapshot(
  events: FactoryEvent[],
  selectedTick: number,
): WorldState {
  return buildProjectedTimelineSnapshot(
    events,
    selectedTick,
    reconstructWorldState,
  );
}

const timelineStoreStateDeps: TimelineStoreStateDeps = {
  buildFactoryTimelineProjection: (events, selectedTick, checkpoint) =>
    buildProjectedTimelineProjection(
      events,
      selectedTick,
      checkpoint
        ? (nextEvents, nextSelectedTick) =>
            advanceWorldStateFromCheckpoint(
              checkpoint.replayState,
              nextEvents,
              nextSelectedTick,
            )
        : reconstructWorldState,
    ),
  buildFactoryTimelineSnapshot,
  orderedEvents,
};

export const useFactoryTimelineStore = create<FactoryTimelineState>((set) => ({
  ...emptyTimelineState(),
  appendEvent: (event) => {
    set((current) =>
      appendTimelineEvents(current, [event], timelineStoreStateDeps),
    );
  },
  appendEvents: (events) => {
    set((current) =>
      appendTimelineEvents(current, events, timelineStoreStateDeps),
    );
  },
  replaceEvents: (events) => {
    set(replaceTimelineEvents(events, timelineStoreStateDeps));
  },
  reset: () => {
    set(emptyTimelineState());
  },
  restoreCheckpoint: (checkpoint: FactoryTimelineCheckpoint) => {
    set(restoreTimelineCheckpoint(checkpoint));
  },
  selectTick: (tick) => {
    set((current) => selectTimelineTick(current, tick, timelineStoreStateDeps));
  },
  setCurrentMode: () => {
    set((current) => setTimelineCurrentMode(current, timelineStoreStateDeps));
  },
}));
