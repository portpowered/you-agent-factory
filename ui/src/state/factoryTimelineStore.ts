import { create } from "zustand";

import type { FactoryEvent } from "../api/events";
import { buildFactoryTimelineSnapshot as buildProjectedTimelineSnapshot } from "./timeline/buildSnapshot";
export { resolveConfiguredWorkTypeName } from "./timeline/projectTopology";
import { reconstructWorldState } from "./timeline/replayWorldState";
import { orderedEvents } from "./timeline/shared";
export type { FactoryTimelineSnapshot } from "./timeline/snapshotTypes";
import type { FactoryTimelineSnapshot } from "./timeline/snapshotTypes";
import {
  appendTimelineEvents,
  emptyTimelineState,
  replaceTimelineEvents,
  selectTimelineTick,
  setTimelineCurrentMode,
  type FactoryTimelineState,
  type TimelineStoreStateDeps,
} from "./timeline/storeState";
export type { FactoryTimelineMode } from "./timeline/storeState";

export function buildFactoryTimelineSnapshot(
  events: FactoryEvent[],
  selectedTick: number,
): FactoryTimelineSnapshot {
  return buildProjectedTimelineSnapshot(events, selectedTick, reconstructWorldState);
}

const timelineStoreStateDeps: TimelineStoreStateDeps = {
  buildFactoryTimelineSnapshot,
  orderedEvents,
};

export const useFactoryTimelineStore = create<FactoryTimelineState>((set) => ({
  ...emptyTimelineState(),
  appendEvent: (event) => {
    set((current) => appendTimelineEvents(current, [event], timelineStoreStateDeps));
  },
  appendEvents: (events) => {
    set((current) => appendTimelineEvents(current, events, timelineStoreStateDeps));
  },
  replaceEvents: (events) => {
    set(replaceTimelineEvents(events, timelineStoreStateDeps));
  },
  reset: () => {
    set(emptyTimelineState());
  },
  selectTick: (tick) => {
    set((current) => selectTimelineTick(current, tick, timelineStoreStateDeps));
  },
  setCurrentMode: () => {
    set((current) => setTimelineCurrentMode(current, timelineStoreStateDeps));
  },
}));
