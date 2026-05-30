import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type FormEvent, useMemo, useState } from "react";

import {
  closeFactorySession,
  type FactorySessionSummary,
  type FactorySessionTarget,
  type FactorySessionsAPIError,
  listFactorySessions,
  openFactorySession,
} from "../../../api/factory-sessions";
import { useDashboardSessionStore } from "../../dashboard/state/dashboardSessionStore";
import {
  classifyFactorySessionFolderValidationError,
  factorySessionTargetOptionValue,
  type FolderValidationState,
  normalizeFactorySessionsError,
} from "../lib/dashboard-session-tabs-utils";
import { selectedFactorySessionTarget } from "../lib/dashboard-session-tabs-utils";

export const FACTORY_SESSIONS_QUERY_KEY = ["factory-sessions"] as const;

export type DashboardSessionTabsState = ReturnType<
  typeof useDashboardSessionTabsState
>;

interface ValidateFolderInput {
  folderPath: string;
  target?: Parameters<typeof openFactorySession>[0]["target"];
}

export function useDashboardSessionTabsState() {
  const queryClient = useQueryClient();
  const sessionsQuery = useQuery({
    queryKey: FACTORY_SESSIONS_QUERY_KEY,
    queryFn: () => listFactorySessions(),
  });
  const closeSessionMutation = useMutation({
    mutationFn: (sessionID: string) => closeFactorySession(sessionID),
  });
  const [closeError, setCloseError] = useState<FactorySessionsAPIError | null>(null);
  const sessions = sessionsQuery.data ?? [];
  const { activeSession, activeSessionID, pausedSessionIDs, setActiveSessionID, setSessionPaused } =
    useActiveDashboardSession(sessions);
  const dialogState = useOpenSessionDialogState({ queryClient, setActiveSessionID });

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
    ...dialogState,
    handleCloseSession,
    isSessionStreamPaused,
    sessions,
    sessionsQuery,
    setActiveSessionID,
    toggleSessionStreamPaused,
  };
}

function useOpenSessionDialogState({
  queryClient,
  setActiveSessionID,
}: {
  queryClient: ReturnType<typeof useQueryClient>;
  setActiveSessionID: (sessionID: string | null) => void;
}) {
  const openSessionMutation = useMutation({
    mutationFn: (input: Parameters<typeof openFactorySession>[0]) =>
      openFactorySession(input),
  });
  const validateFolderMutation = useMutation({
    mutationFn: (input: ValidateFolderInput) =>
      openFactorySession({
        folderPath: input.folderPath,
        target: input.target,
        validateOnly: true,
      }),
  });
  const [dialogOpen, setDialogOpen] = useState(false);
  const [dialogError, setDialogError] = useState<FactorySessionsAPIError | null>(null);
  const [folderPath, setFolderPath] = useState("");
  const [validatedFolderPath, setValidatedFolderPath] = useState<string | null>(null);
  const [discoveredTargets, setDiscoveredTargets] = useState<FactorySessionTarget[]>([]);
  const [selectedTargetValue, setSelectedTargetValue] = useState<string>("");
  const [folderValidation, setFolderValidation] = useState<FolderValidationState>({
    status: "idle",
  });

  async function handleInspectFolder(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setDialogError(null);
    setDiscoveredTargets([]);
    setFolderValidation({ status: "pending" });

    try {
      await inspectFolderCandidate({
        folderPath,
        setDiscoveredTargets,
        setFolderValidation,
        setSelectedTargetValue,
        setValidatedFolderPath,
        validateFolder: validateFolderMutation.mutateAsync,
      });
    } catch (error) {
      const apiError = normalizeFactorySessionsError(error);
      setDialogError(apiError);
      setValidatedFolderPath(null);
      setFolderValidation({
        status: "error",
        reason: classifyFactorySessionFolderValidationError(apiError),
      });
    }
  }

  async function handleOpenTarget(targetValue?: string) {
    setDialogError(null);
    try {
      const response = await openValidatedTarget({
        discoveredTargets,
        folderPath,
        openSession: openSessionMutation.mutateAsync,
        selectedTargetValue: targetValue ?? selectedTargetValue,
        validatedFolderPath,
      });
      if (response.session) {
        await finishOpeningSession(
          queryClient,
          response.session,
          resetDialogState,
          setActiveSessionID,
          setDialogOpen,
        );
      }
    } catch (error) {
      setDialogError(normalizeFactorySessionsError(error));
    }
  }

  async function handleCreateNewFactory() {
    const initFolderPath =
      folderValidation.status === "init_ready"
        ? folderValidation.folderPath
        : validatedFolderPath ?? folderPath;
    setDialogError(null);
    try {
      const response = await openSessionMutation.mutateAsync({
        folderPath: initFolderPath,
        initNewFactory: true,
      });
      if (response.session) {
        await finishOpeningSession(
          queryClient,
          response.session,
          resetDialogState,
          setActiveSessionID,
          setDialogOpen,
        );
      }
    } catch (error) {
      setDialogError(normalizeFactorySessionsError(error));
    }
  }

  function handleCancelInitConfirmation() {
    setDialogError(null);
    setDiscoveredTargets([]);
    setFolderValidation({ status: "idle" });
    setSelectedTargetValue("");
    setValidatedFolderPath(null);
  }

  function resetDialogState() {
    setDialogError(null);
    setDiscoveredTargets([]);
    setFolderValidation({ status: "idle" });
    setFolderPath("");
    setSelectedTargetValue("");
    setValidatedFolderPath(null);
  }

  function handleChangeFolderPath(value: string) {
    setFolderPath(value);
    setDialogError(null);
    setValidatedFolderPath(null);
    setSelectedTargetValue("");
    setDiscoveredTargets([]);
    setFolderValidation({ status: "idle" });
  }

  return {
    dialogError,
    dialogOpen,
    discoveredTargets,
    folderValidation,
    folderPath,
    selectedTargetValue,
    handleCancelInitConfirmation,
    handleChangeFolderPath,
    handleCreateNewFactory,
    handleInspectFolder,
    handleOpenTarget,
    setSelectedTargetValue,
    openSessionMutation,
    resetDialogState,
    setDialogOpen,
    validateFolderMutation,
  };
}

async function finishOpeningSession(
  queryClient: ReturnType<typeof useQueryClient>,
  session: FactorySessionSummary,
  resetDialogState: () => void,
  setActiveSessionID: (sessionID: string | null) => void,
  setDialogOpen: (open: boolean) => void,
) {
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

async function inspectFolderCandidate({
  folderPath,
  setDiscoveredTargets,
  setFolderValidation,
  setSelectedTargetValue,
  setValidatedFolderPath,
  validateFolder,
}: {
  folderPath: string;
  setDiscoveredTargets: (targets: FactorySessionTarget[]) => void;
  setFolderValidation: (state: FolderValidationState) => void;
  setSelectedTargetValue: (value: string) => void;
  setValidatedFolderPath: (value: string | null) => void;
  validateFolder: (
    input: ValidateFolderInput,
  ) => ReturnType<typeof openFactorySession>;
}) {
  const response = await validateFolder({
    folderPath,
  });
  if (response.initsNewFactory) {
    const resolvedFolderPath = response.folderPath ?? folderPath;
    setDiscoveredTargets([]);
    setSelectedTargetValue("");
    setValidatedFolderPath(resolvedFolderPath);
    setFolderValidation({
      status: "init_ready",
      folderPath: resolvedFolderPath,
    });
    return;
  }

  const targets = response.targets ?? [];
  const resolvedFolderPath = targets[0]?.folderPath ?? folderPath;
  setDiscoveredTargets(targets);
  setSelectedTargetValue(validatedTargetValue(targets));
  setValidatedFolderPath(resolvedFolderPath);
  setFolderValidation({ status: "ready", targets });
}

async function openValidatedTarget({
  discoveredTargets,
  folderPath,
  openSession,
  selectedTargetValue,
  validatedFolderPath,
}: {
  discoveredTargets: FactorySessionTarget[];
  folderPath: string;
  openSession: (
    input: Parameters<typeof openFactorySession>[0],
  ) => ReturnType<typeof openFactorySession>;
  selectedTargetValue: string;
  validatedFolderPath: string | null;
}) {
  const selectedTarget = selectedFactorySessionTarget(
    discoveredTargets,
    selectedTargetValue,
  );
  const launchTarget = selectedTarget?.ref ?? null;
  if (launchTarget == null) {
    return { session: undefined };
  }

  return openSession({
    folderPath: validatedFolderPath ?? selectedTarget?.folderPath ?? folderPath,
    target: launchTarget,
  });
}

function validatedTargetValue(
  targets: FactorySessionTarget[],
): string {
  return targets.length === 1 ? factorySessionTargetOptionValue(targets[0]) : "";
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
