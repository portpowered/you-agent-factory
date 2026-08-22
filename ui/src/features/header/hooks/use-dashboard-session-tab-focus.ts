import { useLayoutEffect, useRef } from "react";

import type { FactorySessionSummary } from "../../../api/factory-sessions";

export function useDashboardSessionTabFocus({
  activeSession,
  onSelectSession,
  sessions,
}: {
  activeSession: FactorySessionSummary | null;
  onSelectSession: (sessionID: string) => void;
  sessions: FactorySessionSummary[];
}) {
  const sessionButtonRefs = useRef(new Map<string, HTMLButtonElement>());
  const sessionButtonRefCallbacks = useRef<
    Map<string, (element: HTMLButtonElement | null) => void>
  >(new Map());
  const pendingFocusSessionID = useRef<string | null>(null);
  const lastCommittedActiveSessionID = useRef(activeSession?.id ?? null);

  useLayoutEffect(() => {
    const currentActiveSessionID = activeSession?.id ?? null;
    const requestedFocusSessionID = pendingFocusSessionID.current;
    const activeSessionIsLive =
      currentActiveSessionID !== null &&
      sessions.some((session) => session.id === currentActiveSessionID);
    const requestedFocusSessionIsLive =
      requestedFocusSessionID !== null &&
      sessions.some((session) => session.id === requestedFocusSessionID);
    const activeSessionChanged =
      currentActiveSessionID !== lastCommittedActiveSessionID.current;
    const focusSessionID =
      requestedFocusSessionID === currentActiveSessionID
        ? requestedFocusSessionID
        : activeSessionChanged
          ? currentActiveSessionID
          : null;
    const focusButton = focusSessionID
      ? sessionButtonRefs.current.get(focusSessionID)
      : undefined;

    if (focusButton && activeSessionIsLive) {
      focusButton.focus();
    }

    if (!requestedFocusSessionIsLive) {
      pendingFocusSessionID.current = null;
    } else if (
      focusSessionID === requestedFocusSessionID &&
      focusButton &&
      activeSessionIsLive
    ) {
      // Keep the request until the keyed DOM ref is committed and focused.
      pendingFocusSessionID.current = null;
    } else if (
      requestedFocusSessionID !== null &&
      requestedFocusSessionID !== currentActiveSessionID &&
      activeSessionChanged
    ) {
      pendingFocusSessionID.current = null;
    } else if (
      requestedFocusSessionID !== null &&
      requestedFocusSessionIsLive &&
      requestedFocusSessionID !== currentActiveSessionID
    ) {
      onSelectSession(requestedFocusSessionID);
    }

    lastCommittedActiveSessionID.current = currentActiveSessionID;
  }, [activeSession?.id, onSelectSession, sessions]);

  function getSessionButtonRef(sessionID: string) {
    const existingCallback = sessionButtonRefCallbacks.current.get(sessionID);
    if (existingCallback) {
      return existingCallback;
    }

    const callback = (element: HTMLButtonElement | null) => {
      if (element) {
        sessionButtonRefs.current.set(sessionID, element);
      } else {
        sessionButtonRefs.current.delete(sessionID);
      }
    };
    sessionButtonRefCallbacks.current.set(sessionID, callback);
    return callback;
  }

  function selectAndFocusSession(sessionID: string) {
    if (!sessions.some((session) => session.id === sessionID)) {
      return;
    }

    pendingFocusSessionID.current = sessionID;
    onSelectSession(sessionID);

    if (activeSession?.id === sessionID) {
      const button = sessionButtonRefs.current.get(sessionID);
      if (button) {
        button.focus();
        pendingFocusSessionID.current = null;
      }
    }
  }

  function moveSessionFocus(currentSessionID: string, offset: number) {
    const currentIndex = sessions.findIndex(
      (session) => session.id === currentSessionID,
    );
    if (currentIndex < 0) {
      return;
    }
    const nextIndex =
      (currentIndex + offset + sessions.length) % sessions.length;
    const nextSession = sessions[nextIndex];
    if (!nextSession) {
      return;
    }
    selectAndFocusSession(nextSession.id);
  }

  return {
    getSessionButtonRef,
    moveSessionFocus,
    selectAndFocusSession,
  };
}
