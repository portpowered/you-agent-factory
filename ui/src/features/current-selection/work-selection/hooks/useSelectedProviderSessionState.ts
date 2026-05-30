import { useEffect, useMemo, useState } from "react";

import { requestInferenceAttempts } from "../../dispatch-selection/dispatch-history/selected-work-dispatch-history-helpers";
import {
  getLoadableProviderSessionRef,
  providerSessionSelectionKey,
  type LoadableProviderSessionRef,
} from "../../../provider-session-detail/lib/provider-session-ref";
import type { CurrentSelectionState } from "../../hooks/useCurrentSelection";

type CurrentSelectionVisibleProviderSessions = Pick<
  CurrentSelectionState,
  | "selectedNode"
  | "selectedNodeProviderSessions"
  | "selectedWorkDispatchAttempts"
  | "selectedWorkRequestHistory"
  | "selection"
>;

export interface SelectedProviderSessionState {
  selectedProviderSession: LoadableProviderSessionRef | null;
  selectedProviderSessionKey: string | null;
  setSelectedProviderSession: (session: LoadableProviderSessionRef) => void;
}

export function useSelectedProviderSessionState({
  selectedNode,
  selectedNodeProviderSessions,
  selectedWorkDispatchAttempts,
  selectedWorkRequestHistory,
  selection,
}: CurrentSelectionVisibleProviderSessions): SelectedProviderSessionState {
  const [selectedProviderSession, setSelectedProviderSession] =
    useState<LoadableProviderSessionRef | null>(null);
  const visibleProviderSessionKeys = useMemo(
    () =>
      new Set(
        (selection?.kind === "work-item"
          ? [
              ...selectedWorkDispatchAttempts,
              ...selectedWorkRequestHistory.flatMap((request) =>
                requestInferenceAttempts(request),
              ),
            ]
          : selectedNode
            ? selectedNodeProviderSessions
            : []
        )
          .map((attempt) => getLoadableProviderSessionRef(attempt))
          .filter(
            (session): session is LoadableProviderSessionRef =>
              session !== null,
          )
          .map((session) => providerSessionSelectionKey(session)),
      ),
    [
      selectedNode,
      selectedNodeProviderSessions,
      selectedWorkDispatchAttempts,
      selectedWorkRequestHistory,
      selection,
    ],
  );
  const selectedProviderSessionKey = selectedProviderSession
    ? providerSessionSelectionKey(selectedProviderSession)
    : null;

  useEffect(() => {
    if (!selectedProviderSession) {
      return;
    }

    if (
      !visibleProviderSessionKeys.has(
        providerSessionSelectionKey(selectedProviderSession),
      )
    ) {
      setSelectedProviderSession(null);
    }
  }, [selectedProviderSession, visibleProviderSessionKeys]);

  return {
    selectedProviderSession,
    selectedProviderSessionKey,
    setSelectedProviderSession,
  };
}
