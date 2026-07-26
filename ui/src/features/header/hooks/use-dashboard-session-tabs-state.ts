import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type FormEvent, useMemo, useState } from "react";

import {
  closeFactorySession,
  type FactorySessionSummary,
  type FactorySessionsAPIError,
  type FactorySessionTarget,
  listFactorySessions,
  openFactorySession,
} from "../../../api/factory-sessions";
import { FACTORY_SESSIONS_QUERY_KEY } from "../../../api/factory-sessions/query-keys";
import { useDashboardSessionStore } from "../../dashboard/state/dashboardSessionStore";
import { clearTimelineCheckpointsForSession } from "../../timeline/public/checkpoint-persistence";
import { useFactoryTimelineStore } from "../../timeline/public/store";
import {
  classifyFactorySessionFolderValidationError,
  type FolderValidationState,
  factorySessionTargetOptionValue,
  moveSessionTabOrder,
  normalizeFactorySessionsError,
  orderFactorySessions,
  selectedFactorySessionTarget,
} from "../lib/dashboard-session-tabs-utils";

export type DashboardSessionTabsState = ReturnType<
  typeof useDashboardSessionTabsState
>;

interface ValidateFolderInput {
  folderPath: string;
  target?: Parameters<typeof openFactorySession>[0]["target"];
}

export function useDashboardSessionTabsState() {
  const queryClient = useQueryClient();
  const removeTimelineEntriesForSession = useFactoryTimelineStore(
    (state) => state.removeEntriesForSession,
  );
  const sessionsQuery = useQuery({
    queryKey: FACTORY_SESSIONS_QUERY_KEY,
    queryFn: () => listFactorySessions(),
  });
  const closeSessionMutation = useMutation({
    mutationFn: (sessionID: string) => closeFactorySession(sessionID),
  });
  const [closeError, setCloseError] = useState<FactorySessionsAPIError | null>(
    null,
  );
  const sessions = sessionsQuery.data ?? [];
  const {
    activeSession,
    activeSessionID,
    moveSessionTab,
    pausedSessionIDs,
    sessions: orderedSessions,
    setActiveSessionID,
    setSessionPaused,
  } = useActiveDashboardSession(sessions);
  const dialogState = useOpenSessionDialogState({
    queryClient,
    setActiveSessionID,
  });

  async function handleCloseSession(sessionID: string) {
    setCloseError(null);
    try {
      await closeSessionMutation.mutateAsync(sessionID);
      removeTimelineEntriesForSession(sessionID);
      await clearTimelineCheckpointsForSession(
        typeof window === "undefined" ? undefined : window.indexedDB,
        sessionID,
      );
      setSessionPaused(sessionID, false);
      queryClient.setQueryData(
        FACTORY_SESSIONS_QUERY_KEY,
        (current: FactorySessionSummary[] | undefined) =>
          current?.filter((session) => session.id !== sessionID) ?? [],
      );
      const nextSessionID =
        sessionID === activeSessionID
          ? (sessions.find((session) => session.id !== sessionID)?.id ?? null)
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
    moveSessionTab,
    sessions: orderedSessions,
    sessionsQuery,
    setActiveSessionID,
    toggleSessionStreamPaused,
  };
}

function useOpenSessionDialogFormState() {
  const [dialogOpen, setDialogOpen] = useState(false);
  const [dialogError, setDialogError] =
    useState<FactorySessionsAPIError | null>(null);
  const [folderPath, setFolderPath] = useState("");
  const [validatedFolderPath, setValidatedFolderPath] = useState<string | null>(
    null,
  );
  const [discoveredTargets, setDiscoveredTargets] = useState<
    FactorySessionTarget[]
  >([]);
  const [selectedTargetValue, setSelectedTargetValue] = useState<string>("");
  const [folderValidation, setFolderValidation] =
    useState<FolderValidationState>({
      status: "idle",
    });

  function clearFolderInspection() {
    setDialogError(null);
    setDiscoveredTargets([]);
    setFolderValidation({ status: "idle" });
    setSelectedTargetValue("");
    setValidatedFolderPath(null);
  }

  function resetDialogState() {
    clearFolderInspection();
    setFolderPath("");
  }

  function handleChangeFolderPath(value: string) {
    setFolderPath(value);
    clearFolderInspection();
  }

  return {
    clearFolderInspection,
    dialogError,
    dialogOpen,
    discoveredTargets,
    folderPath,
    folderValidation,
    resetDialogState,
    selectedTargetValue,
    setDialogError,
    setDialogOpen,
    setDiscoveredTargets,
    setFolderPath,
    setFolderValidation,
    setSelectedTargetValue,
    setValidatedFolderPath,
    validatedFolderPath,
    handleChangeFolderPath,
  };
}

function useOpenSessionDialogState({
  queryClient,
  setActiveSessionID,
}: {
  queryClient: ReturnType<typeof useQueryClient>;
  setActiveSessionID: (sessionID: string | null) => void;
}) {
  const form = useOpenSessionDialogFormState();
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

  async function openSessionAndFinish(
    input: Parameters<typeof openFactorySession>[0],
  ) {
    form.setDialogError(null);
    try {
      const response = await openSessionMutation.mutateAsync(input);
      if (response.session) {
        await finishOpeningSession(
          queryClient,
          response.session,
          form.resetDialogState,
          setActiveSessionID,
          form.setDialogOpen,
        );
      }
    } catch (error) {
      form.setDialogError(normalizeFactorySessionsError(error));
    }
  }

  async function handleInspectFolder(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    form.setDialogError(null);
    form.setDiscoveredTargets([]);
    form.setFolderValidation({ status: "pending" });

    try {
      await inspectFolderCandidate({
        folderPath: form.folderPath,
        setDiscoveredTargets: form.setDiscoveredTargets,
        setFolderValidation: form.setFolderValidation,
        setSelectedTargetValue: form.setSelectedTargetValue,
        setValidatedFolderPath: form.setValidatedFolderPath,
        validateFolder: validateFolderMutation.mutateAsync,
      });
    } catch (error) {
      const apiError = normalizeFactorySessionsError(error);
      form.setDialogError(apiError);
      form.setValidatedFolderPath(null);
      form.setFolderValidation({
        status: "error",
        reason: classifyFactorySessionFolderValidationError(apiError),
      });
    }
  }

  async function handleOpenTarget(targetValue?: string) {
    form.setDialogError(null);
    try {
      const response = await openValidatedTarget({
        discoveredTargets: form.discoveredTargets,
        folderPath: form.folderPath,
        openSession: openSessionMutation.mutateAsync,
        selectedTargetValue: targetValue ?? form.selectedTargetValue,
        validatedFolderPath: form.validatedFolderPath,
      });
      if (response.session) {
        await finishOpeningSession(
          queryClient,
          response.session,
          form.resetDialogState,
          setActiveSessionID,
          form.setDialogOpen,
        );
      }
    } catch (error) {
      form.setDialogError(normalizeFactorySessionsError(error));
    }
  }

  async function handleCreateNewFactory() {
    const initFolderPath = resolveInitNewFactoryFolderPath(
      form.folderValidation,
      form.validatedFolderPath,
      form.folderPath,
    );
    await openSessionAndFinish({
      folderPath: initFolderPath,
      initNewFactory: true,
    });
  }

  return {
    dialogError: form.dialogError,
    dialogOpen: form.dialogOpen,
    discoveredTargets: form.discoveredTargets,
    folderValidation: form.folderValidation,
    folderPath: form.folderPath,
    selectedTargetValue: form.selectedTargetValue,
    handleCancelInitConfirmation: form.clearFolderInspection,
    handleChangeFolderPath: form.handleChangeFolderPath,
    handleCreateNewFactory,
    handleInspectFolder,
    handleOpenTarget,
    setSelectedTargetValue: form.setSelectedTargetValue,
    openSessionMutation,
    resetDialogState: form.resetDialogState,
    setDialogOpen: form.setDialogOpen,
    validateFolderMutation,
  };
}

function resolveInitNewFactoryFolderPath(
  folderValidation: FolderValidationState,
  validatedFolderPath: string | null,
  folderPath: string,
): string {
  if (folderValidation.status === "init_ready") {
    return folderValidation.folderPath;
  }
  return validatedFolderPath ?? folderPath;
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

function validatedTargetValue(targets: FactorySessionTarget[]): string {
  return targets.length === 1
    ? factorySessionTargetOptionValue(targets[0])
    : "";
}

function useActiveDashboardSession(sessions: FactorySessionSummary[]) {
  const activeSessionID = useDashboardSessionStore(
    (state) => state.selectedSessionID,
  );
  const pausedSessionIDs = useDashboardSessionStore(
    (state) => state.pausedSessionIDs,
  );
  const orderedSessionIDs = useDashboardSessionStore(
    (state) => state.sessionTabOrder,
  );
  const setActiveSessionID = useDashboardSessionStore(
    (state) => state.setSelectedSessionID,
  );
  const setSessionPaused = useDashboardSessionStore(
    (state) => state.setSessionPaused,
  );
  const setSessionTabOrder = useDashboardSessionStore(
    (state) => state.setSessionTabOrder,
  );
  const orderedSessions = useMemo(
    () => orderFactorySessions(sessions, orderedSessionIDs),
    [orderedSessionIDs, sessions],
  );
  const activeSession = useMemo(
    () =>
      orderedSessions.find((session) => session.id === activeSessionID) ??
      orderedSessions[0] ??
      null,
    [activeSessionID, orderedSessions],
  );

  function moveSessionTab(sessionID: string, targetIndex: number) {
    const nextOrder = moveSessionTabOrder(
      orderedSessions.map((session) => session.id),
      sessionID,
      targetIndex,
    );
    setSessionTabOrder(nextOrder);
  }

  return {
    activeSession,
    activeSessionID,
    moveSessionTab,
    pausedSessionIDs,
    sessions: orderedSessions,
    setActiveSessionID,
    setSessionPaused,
  };
}
