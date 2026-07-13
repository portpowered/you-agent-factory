import type { StreamDerivedCacheIdentity } from "../../lib/stream-derived-cache-identity";
import {
  emptyTimelineState,
  type FactoryTimelineEntryState,
} from "../timeline/storeState";

export type FactoryTimelineEntryKey = string;

/**
 * Logical session identity is remap metadata. It is intentionally absent from
 * this key so aliases cannot create or select a different resolved stream.
 */
export function factoryTimelineEntryKey(
  identity: StreamDerivedCacheIdentity,
): FactoryTimelineEntryKey {
  return JSON.stringify([
    identity.backendScopeID,
    identity.factorySessionID,
    identity.streamGenerationID,
  ]);
}

export function createFactoryTimelineEntry(
  identity: StreamDerivedCacheIdentity,
): FactoryTimelineEntryState {
  return {
    ...emptyTimelineState(),
    identity: { ...identity },
  };
}

export function withEntryTimelineState(
  entry: FactoryTimelineEntryState,
  timeline: Omit<
    ReturnType<typeof emptyTimelineState>,
    "materializedWorkOutcomeState"
  > &
    Partial<Pick<FactoryTimelineEntryState, "materializedWorkOutcomeState">>,
): FactoryTimelineEntryState {
  if (timeline === entry) {
    return entry;
  }
  return {
    ...entry,
    ...timeline,
  };
}
