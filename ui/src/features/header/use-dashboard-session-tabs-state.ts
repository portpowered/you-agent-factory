import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type FormEvent, useMemo, useState } from "react";

import {
  closeFactorySession,
  type FactorySessionSummary,
  type FactorySessionTarget,
  FactorySessionsAPIError,
  listFactorySessions,
  openFactorySession,
} from "../../api/factory-sessions";
import { useDashboardSessionStore } from "../dashboard/state/dashboardSessionStore";

export const FACTORY_SESSIONS_QUERY_KEY = ["factory-sessions"] as const;

export type DashboardSessionTabsState = ReturnType<
  typeof useDashboardSessionTabsState
>;

export function useDashboardSessionTabsState() {
  const queryClient = useQueryClient();
  const sessionsQuery = useQuery({
    queryKey: FACTORY_SESSIONS_QUERY_KEY,
    queryFn: () => listFactorySessions(),
  });
  const openSessionMutation = useMutation({
    mutationFn: (input: Parameters<typeof openFactorySession>[0]) =>
      openFactorySession(input),
  });
  const closeSessionMutation = useMutation({
    mutationFn: (sessionID: string) => closeFactorySession(sessionID),
  });
  const [dialogOpen, setDialogOpen] = useState(false);
  const [dialogError, setDialogError] = useState<FactorySessionsAPIError | null>(null);
  const [closeError, setCloseError] = useState<FactorySessionsAPIError | null>(null);
  const [folderPath, setFolderPath] = useState("");
  const [discoveredTargets, setDiscoveredTargets] = useState<FactorySessionTarget[]>([]);

  const sessions = sessionsQuery.data ?? [];
  const { activeSession, activeSessionID, pausedSessionIDs, setActiveSessionID, setSessionPaused } =
    useActiveDashboardSession(sessions);

  async function handleInspectFolder(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setDialogError(null);
    setDiscoveredTargets([]);

    try {
      const response = await openSessionMutation.mutateAsync({
        folderPath,
      });
      if (response.session) {
        await finishOpeningSession(response.session);
        return;
      }
      setDiscoveredTargets(response.targets ?? []);
    } catch (error) {
      setDialogError(normalizeFactorySessionsError(error));
    }
  }

  async function handleOpenTarget(target: FactorySessionTarget) {
    setDialogError(null);
    try {
      const response = await openSessionMutation.mutateAsync({
        folderPath,
        target: target.ref,
      });
      if (response.session) {
        await finishOpeningSession(response.session);
      }
    } catch (error) {
      setDialogError(normalizeFactorySessionsError(error));
    }
  }

  async function handleCloseSession(sessionID: string) {
    setCloseError(null);
    try {
      await closeSessionMutation.mutateAsync(sessionID);
      setSessionPaused(sessionID, false);
      queryClient.setQueryData(
        FACTORY_SESSIONS_QUERY_KEY,
        (current: FactorySessionSummary[] | undefined) =>
          current?.filter((session) => session.id !== sessionID) ?? [],
      );
      const nextSessionID =
        sessionID === activeSessionID
          ? sessions.find((session) => session.id !== sessionID)?.id ??
            null
          : activeSessionID;
      setActiveSessionID(nextSessionID);
      await queryClient.invalidateQueries({
        queryKey: FACTORY_SESSIONS_QUERY_KEY,
      });
    } catch (error) {
      setCloseError(normalizeFactorySessionsError(error));
    }
  }

  async function finishOpeningSession(session: FactorySessionSummary) {
    queryClient.setQueryData(
      FACTORY_SESSIONS_QUERY_KEY,
      (current: FactorySessionSummary[] | undefined) => {
        const next = current ?? [];
        if (next.some((existingSession) => existingSession.id === session.id)) {
          return next;
        }
        return [...next, session];
      },
    );
    await queryClient.invalidateQueries({
      queryKey: FACTORY_SESSIONS_QUERY_KEY,
    });
    setActiveSessionID(session.id);
    resetDialogState();
    setDialogOpen(false);
  }

  function resetDialogState() {
    setDialogError(null);
    setDiscoveredTargets([]);
    setFolderPath("");
  }

  function isSessionStreamPaused(sessionID: string): boolean {
    return pausedSessionIDs.includes(sessionID);
  }

  function toggleSessionStreamPaused(sessionID: string) {
    setSessionPaused(sessionID, !isSessionStreamPaused(sessionID));
  }

  return {
    activeSession,
    activeSessionID,
    closeError,
    closeSessionMutation,
    dialogError,
    dialogOpen,
    discoveredTargets,
    folderPath,
    handleCloseSession,
    handleInspectFolder,
    handleOpenTarget,
    isSessionStreamPaused,
    openSessionMutation,
    resetDialogState,
    sessions,
    sessionsQuery,
    setActiveSessionID,
    setDialogOpen,
    setFolderPath,
    toggleSessionStreamPaused,
  };
}

function useActiveDashboardSession(sessions: FactorySessionSummary[]) {
  const activeSessionID = useDashboardSessionStore(
    (state) => state.selectedSessionID,
  );
  const pausedSessionIDs = useDashboardSessionStore(
    (state) => state.pausedSessionIDs,
  );
  const setActiveSessionID = useDashboardSessionStore(
    (state) => state.setSelectedSessionID,
  );
  const setSessionPaused = useDashboardSessionStore(
    (state) => state.setSessionPaused,
  );
  const activeSession = useMemo(
    () =>
      sessions.find((session) => session.id === activeSessionID) ??
      sessions[0] ??
      null,
    [activeSessionID, sessions],
  );

  return {
    activeSession,
    activeSessionID,
    pausedSessionIDs,
    setActiveSessionID,
    setSessionPaused,
  };
}

function normalizeFactorySessionsError(error: unknown): FactorySessionsAPIError {
  if (error instanceof FactorySessionsAPIError) {
    return error;
  }
  return new FactorySessionsAPIError(
    "The dashboard could not complete the factory session request.",
    {
      code: "INTERNAL_ERROR",
      responseBody: error,
    },
  );
}
