import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useCallback, useMemo } from "react";

import {
  approveFactorySession,
  cancelFactorySession,
  pauseFactorySession,
  resumeFactorySession,
  retryFactorySessionDispatch,
  terminateFactorySession,
  type FactorySessionLifecycleControlResponse,
} from "../../../api/factory-sessions";
import type { FactorySessionLifecycleActionID } from "../lib/factory-session-lifecycle-controls";
import { FACTORY_SESSION_DETAIL_QUERY_KEY } from "./use-factory-session-detail";

interface LifecycleControlMutationInput {
  actionID: FactorySessionLifecycleActionID;
}

export interface UseFactorySessionLifecycleControlResult {
  pendingActionID: FactorySessionLifecycleActionID | null;
  submitLifecycleAction: (
    actionID: FactorySessionLifecycleActionID,
  ) => Promise<void>;
}

export function useFactorySessionLifecycleControl({
  selectedDispatchID,
  sessionID,
}: {
  selectedDispatchID: string | null;
  sessionID: string;
}): UseFactorySessionLifecycleControlResult {
  const queryClient = useQueryClient();
  const mutation = useMutation({
    mutationFn: async ({
      actionID,
    }: LifecycleControlMutationInput): Promise<FactorySessionLifecycleControlResponse> => {
      switch (actionID) {
        case "approve":
          return approveFactorySession(sessionID);
        case "cancel":
          return cancelFactorySession(sessionID);
        case "pause":
          return pauseFactorySession(sessionID);
        case "resume":
          return resumeFactorySession(sessionID);
        case "terminate":
          return terminateFactorySession(sessionID);
        case "retry-dispatch":
          if (selectedDispatchID === null || selectedDispatchID.trim() === "") {
            throw new Error(
              "Retry dispatch requires a selected failed dispatch.",
            );
          }

          return retryFactorySessionDispatch(sessionID, {
            dispatchId: selectedDispatchID,
            forceNewAttempt: false,
            resetAttemptCount: false,
          });
      }
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: [...FACTORY_SESSION_DETAIL_QUERY_KEY, sessionID],
      });
    },
  });

  const submitLifecycleAction = useCallback(
    async (actionID: FactorySessionLifecycleActionID) => {
      try {
        await mutation.mutateAsync({ actionID });
      } catch {
        return;
      }
    },
    [mutation],
  );

  return useMemo(
    () => ({
      pendingActionID: mutation.isPending
        ? (mutation.variables?.actionID ?? null)
        : null,
      submitLifecycleAction,
    }),
    [mutation.isPending, mutation.variables, submitLifecycleAction],
  );
}
