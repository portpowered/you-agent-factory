import { create } from "zustand";

import type { DashboardStreamState } from "../../../api/dashboard/types";
import {
  normalizeStreamDerivedCacheIdentity,
  type StreamDerivedCacheIdentity,
} from "../../timeline/public/stream-identity";
import { getDashboardStreamMessages } from "../messages/dashboard-stream";

export interface DashboardSessionStreamProjection {
  streamIdentity: StreamDerivedCacheIdentity | null;
  streamState: DashboardStreamState;
}

export type DashboardSessionStreamProjectionMap = Readonly<
  Record<string, DashboardSessionStreamProjection>
>;

interface DashboardStreamStoreState {
  backendRuntimeCacheScope: string | null;
  resetSessionStreamState: (
    sessionID: string | null,
    locale?: string | null,
  ) => void;
  resetStreamState: (locale?: string | null) => void;
  resolvedStreamIdentity: StreamDerivedCacheIdentity | null;
  setBackendRuntimeCacheScope: (
    backendRuntimeCacheScope: string | null,
  ) => void;
  setSessionStreamState: (
    sessionID: string,
    streamIdentity: StreamDerivedCacheIdentity | null,
    streamState: DashboardStreamState,
    sessionAliases?: readonly string[],
  ) => void;
  setResolvedStreamIdentity: (
    streamIdentity: StreamDerivedCacheIdentity | null,
  ) => void;
  setStreamState: (streamState: DashboardStreamState) => void;
  sessionStreamStateKeysBySessionID: Readonly<Record<string, string>>;
  sessionStreamStates: DashboardSessionStreamProjectionMap;
  streamState: DashboardStreamState;
}

export function createDefaultDashboardStreamState(
  locale?: string | null,
): DashboardStreamState {
  return {
    status: "connecting",
    message: getDashboardStreamMessages(locale).loadingFactoryEvents,
  };
}

export function dashboardStreamStateKey(
  sessionID: string,
  streamIdentity?: StreamDerivedCacheIdentity | null,
): string | null {
  const normalizedSessionID = sessionID.trim();
  const normalizedIdentity =
    normalizeStreamDerivedCacheIdentity(streamIdentity);
  const canonicalSessionID =
    normalizedIdentity?.factorySessionID ?? normalizedSessionID;
  if (canonicalSessionID.length === 0) {
    return null;
  }
  if (!normalizedIdentity) {
    return canonicalSessionID;
  }
  return `${canonicalSessionID}::${normalizedIdentity.streamGenerationID}`;
}

export function getDashboardStreamStateForSession(
  sessionID: string,
  sessionStreamStates: DashboardSessionStreamProjectionMap,
  sessionStreamStateKeysBySessionID: Readonly<Record<string, string>>,
  locale?: string | null,
): DashboardStreamState {
  const normalizedSessionID = sessionID.trim();
  const key =
    sessionStreamStateKeysBySessionID[normalizedSessionID] ??
    dashboardStreamStateKey(normalizedSessionID);
  return (
    (key ? sessionStreamStates[key]?.streamState : undefined) ??
    createDefaultDashboardStreamState(locale)
  );
}

export const useDashboardStreamStore = create<DashboardStreamStoreState>(
  (set) => ({
    backendRuntimeCacheScope: null,
    resetSessionStreamState: (sessionID, locale) => {
      const normalizedSessionID = sessionID?.trim() ?? "";
      set((current) => {
        if (normalizedSessionID.length === 0) {
          return {
            backendRuntimeCacheScope: null,
            resolvedStreamIdentity: null,
            streamState: createDefaultDashboardStreamState(locale),
          };
        }

        const stateKey =
          current.sessionStreamStateKeysBySessionID[normalizedSessionID];
        if (!stateKey) {
          return {
            backendRuntimeCacheScope: null,
            resolvedStreamIdentity: null,
            streamState: createDefaultDashboardStreamState(locale),
          };
        }

        const sessionStreamStates = { ...current.sessionStreamStates };
        delete sessionStreamStates[stateKey];
        const sessionStreamStateKeysBySessionID = {
          ...current.sessionStreamStateKeysBySessionID,
        };
        for (const [alias, key] of Object.entries(
          sessionStreamStateKeysBySessionID,
        )) {
          if (key === stateKey) {
            delete sessionStreamStateKeysBySessionID[alias];
          }
        }

        return {
          backendRuntimeCacheScope: null,
          resolvedStreamIdentity: null,
          sessionStreamStateKeysBySessionID,
          sessionStreamStates,
          streamState: createDefaultDashboardStreamState(locale),
        };
      });
    },
    resetStreamState: (locale) => {
      set({
        backendRuntimeCacheScope: null,
        resolvedStreamIdentity: null,
        sessionStreamStateKeysBySessionID: {},
        sessionStreamStates: {},
        streamState: createDefaultDashboardStreamState(locale),
      });
    },
    resolvedStreamIdentity: null,
    setBackendRuntimeCacheScope: (backendRuntimeCacheScope) => {
      set({ backendRuntimeCacheScope });
    },
    setSessionStreamState: (
      sessionID,
      streamIdentity,
      streamState,
      sessionAliases = [],
    ) => {
      const normalizedSessionID = sessionID.trim();
      const normalizedIdentity =
        normalizeStreamDerivedCacheIdentity(streamIdentity);
      const stateKey = dashboardStreamStateKey(
        normalizedSessionID,
        normalizedIdentity,
      );
      if (!stateKey) {
        return;
      }

      const aliases = new Set(
        [
          normalizedSessionID,
          normalizedIdentity?.factorySessionID,
          ...sessionAliases.map((alias) => alias.trim()),
        ].filter((alias): alias is string =>
          Boolean(alias && alias.length > 0),
        ),
      );
      set((current) => {
        const sessionStreamStates = {
          ...current.sessionStreamStates,
          [stateKey]: {
            streamIdentity: normalizedIdentity,
            streamState,
          },
        };
        const sessionStreamStateKeysBySessionID = {
          ...current.sessionStreamStateKeysBySessionID,
        };
        for (const alias of aliases) {
          sessionStreamStateKeysBySessionID[alias] = stateKey;
        }

        return {
          sessionStreamStateKeysBySessionID,
          sessionStreamStates,
          streamState,
        };
      });
    },
    setResolvedStreamIdentity: (streamIdentity) => {
      set({ resolvedStreamIdentity: streamIdentity });
    },
    setStreamState: (streamState) => {
      set({ streamState });
    },
    sessionStreamStateKeysBySessionID: {},
    sessionStreamStates: {},
    streamState: createDefaultDashboardStreamState(),
  }),
);
