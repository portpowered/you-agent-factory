import { create, type StateCreator } from "zustand";

import type { FactoryEvent } from "../../../api/events";
import { canonicalizeFactoryEvents } from "../../../../../packages/factory-replay/src/index.js";
import {
  correlationTokenForIdentityScope,
  recordSessionPersistenceDiagnostic,
  sessionPersistenceDiagnostic,
} from "../../dashboard/public/session-persistence-diagnostics";
import {
  normalizeStreamDerivedCacheIdentity,
  type StreamDerivedCacheIdentity,
} from "../lib/stream-derived-cache-identity";
import {
  buildFactoryTimelineProjection as buildProjectedTimelineProjection,
  buildFactoryTimelineSnapshot as buildProjectedTimelineSnapshot,
} from "./timeline/buildSnapshot";

export { resolveConfiguredWorkTypeName } from "./timeline/projectTopology";


export type { WorldState } from "./timeline/types";

import {
  appendTimelineEvents,
  emptyTimelineState,
  type FactoryTimelineEntryState,
  type FactoryTimelineCheckpoint,
  type FactoryTimelineState,
  replaceTimelineEvents,
  restoreTimelineCheckpoint,
  selectTimelineTick,
  setTimelineCurrentMode,
  type TimelineStoreStateDeps,
} from "./timeline/storeState";
import type { WorldState } from "./timeline/types";
import {
  createFactoryTimelineEntry,
  factoryTimelineEntryKey,
  withEntryTimelineState,
} from "./entries/factoryTimelineEntry";

export type {
  FactoryTimelineCheckpoint,
  FactoryTimelineEntryState,
  FactoryTimelineMode,
  FactoryTimelineSyncIdentity,
} from "./timeline/storeState";

export function buildFactoryTimelineSnapshot(
  events: FactoryEvent[],
  selectedTick: number,
): WorldState {
  return buildProjectedTimelineSnapshot(
    events,
    selectedTick,
  );
}

const timelineStoreStateDeps: TimelineStoreStateDeps = {
  buildFactoryTimelineProjection: (events, selectedTick, checkpoint) =>
    buildProjectedTimelineProjection(events, selectedTick, checkpoint),
  buildFactoryTimelineSnapshot,
  canonicalizeEvents: canonicalizeFactoryEvents,
};

function exactIdentity(
  identity: StreamDerivedCacheIdentity,
): StreamDerivedCacheIdentity {
  const normalized = normalizeStreamDerivedCacheIdentity(identity);
  if (!normalized) {
    throw new Error(
      "Timeline entries require backend scope, resolved Factory Session UUID, logical session metadata, and stream generation.",
    );
  }
  return normalized;
}

function entryStateForMutation(
  state: FactoryTimelineState,
  identity: StreamDerivedCacheIdentity,
): {
  entry: FactoryTimelineEntryState;
  key: string;
} {
  const normalized = exactIdentity(identity);
  const key = factoryTimelineEntryKey(normalized);
  const existing = state.entriesByKey[key];
  const identityIsUnchanged =
    existing?.identity.backendScopeID === normalized.backendScopeID &&
    existing.identity.factorySessionID === normalized.factorySessionID &&
    existing.identity.logicalSessionKeyID === normalized.logicalSessionKeyID &&
    existing.identity.streamGenerationID === normalized.streamGenerationID;
  return {
    entry: existing
      ? identityIsUnchanged
        ? existing
        : { ...existing, identity: normalized }
      : createFactoryTimelineEntry(normalized),
    key,
  };
}

function bindUnownedActiveState(
  state: FactoryTimelineState,
  entry: FactoryTimelineEntryState,
): FactoryTimelineEntryState {
  if (
    state.activeEntryKey !== null ||
    Object.keys(state.entriesByKey).length > 0
  ) {
    return entry;
  }
  return {
    currentReplayCheckpoint: state.currentReplayCheckpoint,
    events: state.events,
    identity: entry.identity,
    latestTick: state.latestTick,
    materializedWorkOutcomeState: state.materializedWorkOutcomeState,
    mode: state.mode,
    receivedEventIDs: state.receivedEventIDs,
    selectedTick: state.selectedTick,
    worldViewCache: state.worldViewCache,
  };
}

function entryMutation(
  state: FactoryTimelineState,
  identity: StreamDerivedCacheIdentity,
  mutate: (entry: FactoryTimelineEntryState) => FactoryTimelineEntryState,
): Partial<FactoryTimelineState> {
  const { entry, key } = entryStateForMutation(state, identity);
  const nextEntry = mutate(entry);
  const registry = {
    ...state.entriesByKey,
    [key]: nextEntry,
  };
  return state.activeEntryKey === key
    ? { ...nextEntry, entriesByKey: registry }
    : { entriesByKey: registry };
}

function activeEntryMutation(
  state: FactoryTimelineState,
  mutate: (entry: FactoryTimelineEntryState) => FactoryTimelineEntryState,
  mutateLegacy: () => Partial<FactoryTimelineState>,
): Partial<FactoryTimelineState> {
  const key = state.activeEntryKey;
  const entry = key ? state.entriesByKey[key] : undefined;
  if (!key || !entry) {
    return mutateLegacy();
  }
  const nextEntry = mutate(entry);
  return {
    ...nextEntry,
    entriesByKey: {
      ...state.entriesByKey,
      [key]: nextEntry,
    },
  };
}

const initialTimelineState = emptyTimelineState();

type TimelineStoreInitializer = StateCreator<FactoryTimelineState>;
type TimelineStoreSet = Parameters<TimelineStoreInitializer>[0];
type TimelineStoreGet = Parameters<TimelineStoreInitializer>[1];

function exactEntryActions(set: TimelineStoreSet, get: TimelineStoreGet) {
  return {
    activateEntry: (identity) => {
      set((current) => {
        const resolved = entryStateForMutation(current, identity);
        const entry = bindUnownedActiveState(current, resolved.entry);
        const { key } = resolved;
        return {
          ...entry,
          activeEntryKey: key,
          entriesByKey: {
            ...current.entriesByKey,
            [key]: entry,
          },
        };
      });
    },
    appendEventForEntry: (identity, event) => {
      set((current) =>
        entryMutation(current, identity, (entry) =>
          withEntryTimelineState(
            entry,
            appendTimelineEvents(entry, [event], timelineStoreStateDeps),
          ),
        ),
      );
    },
    appendEventsForEntry: (identity, events) => {
      set((current) =>
        entryMutation(current, identity, (entry) =>
          withEntryTimelineState(
            entry,
            appendTimelineEvents(entry, events, timelineStoreStateDeps),
          ),
        ),
      );
    },
    entryForIdentity: (identity) => {
      const normalized = exactIdentity(identity);
      return get().entriesByKey[factoryTimelineEntryKey(normalized)];
    },
    replaceEventsForEntry: (identity, events) => {
      set((current) =>
        entryMutation(current, identity, (entry) =>
          withEntryTimelineState(
            entry,
            replaceTimelineEvents(events, timelineStoreStateDeps),
          ),
        ),
      );
    },
    resetEntry: (identity) => {
      set((current) =>
        entryMutation(current, identity, (entry) =>
          createFactoryTimelineEntry(entry.identity),
        ),
      );
    },
    restoreCheckpointForEntry: (identity, checkpoint) => {
      set((current) =>
        entryMutation(current, identity, (entry) =>
          withEntryTimelineState(entry, restoreTimelineCheckpoint(checkpoint)),
        ),
      );
      try {
        recordSessionPersistenceDiagnostic(
          sessionPersistenceDiagnostic(
            "restore_succeeded",
            correlationTokenForIdentityScope(identity),
          ),
        );
      } catch {
        // Diagnostics are best effort and cannot affect timeline restoration.
      }
    },
    selectTickForEntry: (identity, tick) => {
      set((current) =>
        entryMutation(current, identity, (entry) => ({
          ...entry,
          ...selectTimelineTick(entry, tick, timelineStoreStateDeps),
        })),
      );
    },
    setCurrentModeForEntry: (identity) => {
      set((current) =>
        entryMutation(current, identity, (entry) => ({
          ...entry,
          ...setTimelineCurrentMode(entry, timelineStoreStateDeps),
        })),
      );
    },
  } satisfies Partial<FactoryTimelineState>;
}

function activeEntryActions(set: TimelineStoreSet) {
  return {
    appendEvent: (event: FactoryEvent) => {
      set((current) =>
        activeEntryMutation(
          current,
          (entry) =>
            withEntryTimelineState(
              entry,
              appendTimelineEvents(entry, [event], timelineStoreStateDeps),
            ),
          () => appendTimelineEvents(current, [event], timelineStoreStateDeps),
        ),
      );
    },
    appendEvents: (events: FactoryEvent[]) => {
      set((current) =>
        activeEntryMutation(
          current,
          (entry) =>
            withEntryTimelineState(
              entry,
              appendTimelineEvents(entry, events, timelineStoreStateDeps),
            ),
          () => appendTimelineEvents(current, events, timelineStoreStateDeps),
        ),
      );
    },
    replaceEvents: (events: FactoryEvent[]) => {
      set((current) =>
        activeEntryMutation(
          current,
          (entry) =>
            withEntryTimelineState(
              entry,
              replaceTimelineEvents(events, timelineStoreStateDeps),
            ),
          () => replaceTimelineEvents(events, timelineStoreStateDeps),
        ),
      );
    },
    reset: () => {
      set({
        activeEntryKey: null,
        ...emptyTimelineState(),
        entriesByKey: {},
      });
    },
    restoreCheckpoint: (checkpoint: FactoryTimelineCheckpoint) => {
      set((current) =>
        activeEntryMutation(
          current,
          (entry) =>
            withEntryTimelineState(
              entry,
              restoreTimelineCheckpoint(checkpoint),
            ),
          () => restoreTimelineCheckpoint(checkpoint),
        ),
      );
    },
    selectTick: (tick: number) => {
      set((current) =>
        activeEntryMutation(
          current,
          (entry) => ({
            ...entry,
            ...selectTimelineTick(entry, tick, timelineStoreStateDeps),
          }),
          () => selectTimelineTick(current, tick, timelineStoreStateDeps),
        ),
      );
    },
    setCurrentMode: () => {
      set((current) =>
        activeEntryMutation(
          current,
          (entry) => ({
            ...entry,
            ...setTimelineCurrentMode(entry, timelineStoreStateDeps),
          }),
          () => setTimelineCurrentMode(current, timelineStoreStateDeps),
        ),
      );
    },
  } satisfies Partial<FactoryTimelineState>;
}

export const useFactoryTimelineStore = create<FactoryTimelineState>(
  (set, get) => ({
    activeEntryKey: null,
    ...initialTimelineState,
    entriesByKey: {},
    ...activeEntryActions(set),
    ...exactEntryActions(set, get),
  }),
);
