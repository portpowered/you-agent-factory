import { create } from "zustand";

import type { DashboardStreamState } from "../../../api/dashboard/types";
import { getDashboardStreamMessages } from "../messages/dashboard-stream";

interface DashboardStreamStoreState {
  backendRuntimeCacheScope: string | null;
  resetStreamState: (locale?: string | null) => void;
  setBackendRuntimeCacheScope: (backendRuntimeCacheScope: string | null) => void;
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
        streamState: createDefaultDashboardStreamState(locale),
      });
    },
    setBackendRuntimeCacheScope: (backendRuntimeCacheScope) => {
      set({ backendRuntimeCacheScope });
    },
    setStreamState: (streamState) => {
      set({ streamState });
    },
    streamState: createDefaultDashboardStreamState(),
  }),
);
