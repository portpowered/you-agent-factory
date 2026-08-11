import { create } from "zustand";

import { DEFAULT_FACTORY_SESSION_ID } from "../../../api/session-routing";

interface DashboardSessionStoreState {
  pausedSessionIDs: string[];
  reconcileSessionList: (
    sessionIDs: readonly string[],
    preferredSessionID?: string | null,
  ) => void;
  sessionTabOrder: string[];
  setSessionPaused: (sessionID: string, paused: boolean) => void;
  setSessionTabOrder: (sessionIDs: string[]) => void;
  resolveSessionIdentity: (
    selectorSessionID: string,
    resolvedSessionID: string,
    discoveredSessionIDs: string[],
  ) => void;
  remapSelectedSessionIdentity: (resolvedSessionID: string) => void;
  selectedSessionID: string | null;
  setSelectedSessionID: (sessionID: string | null) => void;
}

const DASHBOARD_SESSION_STORE_DEFAULTS = {
  pausedSessionIDs: [],
  sessionTabOrder: [],
  selectedSessionID: DEFAULT_FACTORY_SESSION_ID,
} satisfies Pick<
  DashboardSessionStoreState,
  "pausedSessionIDs" | "selectedSessionID" | "sessionTabOrder"
>;

function normalizeSessionList(sessionIDs: readonly string[]): string[] {
  const normalizedSessionIDs: string[] = [];
  for (const sessionID of sessionIDs) {
    const normalizedSessionID = sessionID.trim();
    if (
      normalizedSessionID.length > 0 &&
      !normalizedSessionIDs.includes(normalizedSessionID)
    ) {
      normalizedSessionIDs.push(normalizedSessionID);
    }
  }
  return normalizedSessionIDs;
}

function reconcileSessionListState(
  current: DashboardSessionStoreState,
  sessionIDs: readonly string[],
  preferredSessionID?: string | null,
): Pick<
  DashboardSessionStoreState,
  "pausedSessionIDs" | "selectedSessionID" | "sessionTabOrder"
> {
  const preferred = preferredSessionID?.trim() ?? "";
  const normalizedSessionIDs = normalizeSessionList(sessionIDs).filter(
    (sessionID) => sessionID !== DEFAULT_FACTORY_SESSION_ID || !preferred,
  );
  const availableSessionIDs = new Set(normalizedSessionIDs);
  const currentSelected = current.selectedSessionID?.trim() ?? "";
  const fallbackSessionID = normalizedSessionIDs[0] ?? null;
  const selectedSessionID =
    (preferred && availableSessionIDs.has(preferred)
      ? preferred
      : currentSelected && availableSessionIDs.has(currentSelected)
        ? currentSelected
        : fallbackSessionID) ?? null;

  const nextOrder = normalizedSessionIDs.filter((sessionID) =>
    current.sessionTabOrder.includes(sessionID),
  );
  for (const sessionID of normalizedSessionIDs) {
    if (!nextOrder.includes(sessionID)) {
      nextOrder.push(sessionID);
    }
  }

  return {
    pausedSessionIDs: current.pausedSessionIDs.filter((sessionID) =>
      availableSessionIDs.has(sessionID.trim()),
    ),
    selectedSessionID,
    sessionTabOrder: nextOrder,
  };
}

export const useDashboardSessionStore = create<DashboardSessionStoreState>(
  (set) => ({
    ...DASHBOARD_SESSION_STORE_DEFAULTS,
    reconcileSessionList: (sessionIDs, preferredSessionID) =>
      set((current) =>
        reconcileSessionListState(current, sessionIDs, preferredSessionID),
      ),
    setSessionPaused: (sessionID, paused) => {
      const normalizedSessionID = sessionID.trim();
      if (normalizedSessionID.length === 0) {
        return;
      }

      set((current) => ({
        pausedSessionIDs: paused
          ? current.pausedSessionIDs.includes(normalizedSessionID)
            ? current.pausedSessionIDs
            : [...current.pausedSessionIDs, normalizedSessionID]
          : current.pausedSessionIDs.filter(
              (currentSessionID) => currentSessionID !== normalizedSessionID,
            ),
      }));
    },
    setSessionTabOrder: (sessionIDs) => {
      const normalizedOrder: string[] = [];

      for (const sessionID of sessionIDs) {
        const normalizedSessionID = sessionID.trim();
        if (
          normalizedSessionID.length > 0 &&
          !normalizedOrder.includes(normalizedSessionID)
        ) {
          normalizedOrder.push(normalizedSessionID);
        }
      }

      set({
        sessionTabOrder: normalizedOrder,
      });
    },
    resolveSessionIdentity: (
      selectorSessionID,
      resolvedSessionID,
      discoveredSessionIDs,
    ) => {
      const normalizedSelector = selectorSessionID.trim();
      const normalizedResolved = resolvedSessionID.trim();
      if (normalizedSelector.length === 0 || normalizedResolved.length === 0) {
        return;
      }

      set((current) => {
        const nextOrder: string[] = [];
        for (const sessionID of [
          ...current.sessionTabOrder,
          ...discoveredSessionIDs,
        ]) {
          const normalizedSessionID = sessionID.trim();
          const canonicalSessionID =
            normalizedSessionID === normalizedSelector
              ? normalizedResolved
              : normalizedSessionID;
          if (
            canonicalSessionID.length > 0 &&
            !nextOrder.includes(canonicalSessionID)
          ) {
            nextOrder.push(canonicalSessionID);
          }
        }

        return {
          selectedSessionID:
            current.selectedSessionID == null ||
            current.selectedSessionID.trim().length === 0 ||
            current.selectedSessionID === normalizedSelector
              ? normalizedResolved
              : current.selectedSessionID,
          sessionTabOrder: nextOrder,
        };
      });
    },
    remapSelectedSessionIdentity: (resolvedSessionID) => {
      const normalizedResolved = resolvedSessionID.trim();
      if (normalizedResolved.length === 0) {
        return;
      }

      set((current) => {
        const supersededSessionID = current.selectedSessionID?.trim();
        if (
          !supersededSessionID ||
          supersededSessionID === normalizedResolved
        ) {
          return { selectedSessionID: normalizedResolved };
        }

        const replaceIdentity = (sessionIDs: string[]): string[] => {
          const remapped: string[] = [];
          for (const sessionID of sessionIDs) {
            const candidate =
              sessionID.trim() === supersededSessionID
                ? normalizedResolved
                : sessionID.trim();
            if (candidate.length > 0 && !remapped.includes(candidate)) {
              remapped.push(candidate);
            }
          }
          return remapped;
        };

        return {
          pausedSessionIDs: current.pausedSessionIDs.filter(
            (sessionID) => sessionID.trim() !== supersededSessionID,
          ),
          selectedSessionID: normalizedResolved,
          sessionTabOrder: replaceIdentity(current.sessionTabOrder),
        };
      });
    },
    setSelectedSessionID: (sessionID) => {
      set({
        selectedSessionID:
          sessionID == null
            ? null
            : sessionID.trim().length > 0
              ? sessionID
              : DEFAULT_FACTORY_SESSION_ID,
      });
    },
  }),
);

export function resetDashboardSessionStore(): void {
  useDashboardSessionStore.setState(DASHBOARD_SESSION_STORE_DEFAULTS);
}
