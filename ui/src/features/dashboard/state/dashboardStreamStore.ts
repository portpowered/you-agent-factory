import { create } from "zustand";

import type { DashboardStreamState } from "../../../api/dashboard/types";
import type { StreamDerivedCacheIdentity } from "../../timeline/public/stream-identity";
import { getDashboardStreamMessages } from "../messages/dashboard-stream";

interface DashboardStreamStoreState {
  backendRuntimeCacheScope: string | null;
  resetStreamState: (locale?: string | null) => void;
  resolvedStreamIdentity: StreamDerivedCacheIdentity | null;
  setBackendRuntimeCacheScope: (
    backendRuntimeCacheScope: string | null,
  ) => void;
  setResolvedStreamIdentity: (
    streamIdentity: StreamDerivedCacheIdentity | null,
  ) => void;
  setStreamState: (streamState: DashboardStreamState) => void;
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

export const useDashboardStreamStore = create<DashboardStreamStoreState>(
  (set) => ({
    backendRuntimeCacheScope: null,
    resetStreamState: (locale) => {
      set({
        backendRuntimeCacheScope: null,
        resolvedStreamIdentity: null,
        streamState: createDefaultDashboardStreamState(locale),
      });
    },
    resolvedStreamIdentity: null,
    setBackendRuntimeCacheScope: (backendRuntimeCacheScope) => {
      set({ backendRuntimeCacheScope });
    },
    setResolvedStreamIdentity: (streamIdentity) => {
      set({ resolvedStreamIdentity: streamIdentity });
    },
    setStreamState: (streamState) => {
      set({ streamState });
    },
    streamState: createDefaultDashboardStreamState(),
  }),
);
