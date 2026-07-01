import { create } from "zustand";

import type { DashboardStreamState } from "../../../api/dashboard/types";
import type { StreamDerivedCacheIdentity } from "../../timeline/public";
import { getDashboardStreamMessages } from "../messages/dashboard-stream";

interface DashboardStreamStoreState {
  resetStreamState: (locale?: string | null) => void;
  resolvedStreamIdentity: StreamDerivedCacheIdentity | null;
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
    resetStreamState: (locale) => {
      set({
        resolvedStreamIdentity: null,
        streamState: createDefaultDashboardStreamState(locale),
      });
    },
    resolvedStreamIdentity: null,
    setResolvedStreamIdentity: (streamIdentity) => {
      set({ resolvedStreamIdentity: streamIdentity });
    },
    setStreamState: (streamState) => {
      set({ streamState });
    },
    streamState: createDefaultDashboardStreamState(),
  }),
);
