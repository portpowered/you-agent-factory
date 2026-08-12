import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useCallback, useMemo, useState } from "react";

import {
  approveFactorySession,
  cancelFactorySession,
  type FactorySessionDurableReadModel,
  type FactorySessionLifecycleControlResponse,
  interruptFactorySessionDispatch,
  pauseFactorySession,
  resumeFactorySession,
  retryFactorySessionDispatch,
  terminateFactorySession,
} from "../../../api/factory-sessions";
import { normalizeFactorySessionGetResponse } from "../../../api/factory-sessions/normalize-session-get";
import { useDashboardStreamStore } from "../../dashboard/public/runtime-cache-scope";
import type { FactorySessionLifecycleActionID } from "../lib/factory-session-lifecycle-controls";
import type { LifecycleControlFeedbackState } from "../lib/lifecycle/factory-session-lifecycle-feedback";
import type { FactorySessionDetailData } from "./use-factory-session-detail";
import { factorySessionDetailQueryKey } from "./use-factory-session-detail";

interface LifecycleControlMutationInput {
  actionID: FactorySessionLifecycleActionID;
}

export interface FactorySessionLifecycleControl {
  feedback: LifecycleControlFeedbackState | null;
  pendingActionID: FactorySessionLifecycleActionID | null;
  submitLifecycleAction: (
    actionID: FactorySessionLifecycleActionID,
  ) => Promise<void>;
}

export type UseFactorySessionLifecycleControlResult =
  FactorySessionLifecycleControl;

export function useFactorySessionLifecycleControl({
  selectedDispatchID,
  sessionID,
}: {
  selectedDispatchID: string | null;
  sessionID: string;
}): UseFactorySessionLifecycleControlResult {
  const queryClient = useQueryClient();
  const backendRuntimeCacheScope = useDashboardStreamStore(
    (state) => state.backendRuntimeCacheScope,
  );
  const [feedback, setFeedback] =
    useState<LifecycleControlFeedbackState | null>(null);
  const detailQueryKey = factorySessionDetailQueryKey(
    sessionID,
    backendRuntimeCacheScope,
  );
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
        case "interrupt-dispatch":
          if (selectedDispatchID === null || selectedDispatchID.trim() === "") {
            throw new Error(
              "Interrupt dispatch requires a selected active dispatch.",
            );
          }

          return interruptFactorySessionDispatch(sessionID, {
            dispatchId: selectedDispatchID,
          });
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
    onMutate: ({ actionID }) => {
      setFeedback(null);
      return { actionID };
    },
    onError: (error, { actionID }) => {
      setFeedback({
        actionID,
        kind: "transport-error",
        message:
          error instanceof Error
            ? error.message
            : typeof error === "string"
              ? error
              : "",
      });
    },
    onSuccess: async (response, { actionID }) => {
      setFeedback({
        actionID,
        kind: "resolved",
        response,
      });
      queryClient.setQueryData<FactorySessionDetailData | undefined>(
        detailQueryKey,
        (current) => reconcileLifecycleControlDetailState(current, response),
      );
      await queryClient.invalidateQueries({
        queryKey: detailQueryKey,
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
      feedback,
      pendingActionID: mutation.isPending
        ? (mutation.variables?.actionID ?? null)
        : null,
      submitLifecycleAction,
    }),
    [feedback, mutation.isPending, mutation.variables, submitLifecycleAction],
  );
}

function reconcileLifecycleControlDetailState(
  current: FactorySessionDetailData | undefined,
  response: FactorySessionLifecycleControlResponse,
): FactorySessionDetailData | undefined {
  if (!response.session) {
    return current;
  }

  const normalized = normalizeFactorySessionGetResponse(
    response.session as FactorySessionDurableReadModel,
  );

  if (!current) {
    return {
      durableLifecycleStatus:
        normalized.durableLifecycleStatus ?? response.status,
      partialResult: normalized.partialResult,
      result: normalized.result,
      session: normalized.session,
    };
  }

  return {
    dispatches: current.dispatches,
    durableLifecycleStatus:
      normalized.durableLifecycleStatus ?? response.status,
    partialResult: normalized.partialResult ?? current.partialResult,
    result: normalized.result ?? current.result,
    session: {
      ...current.session,
      ...normalized.session,
      runtime: {
        ...current.session.runtime,
        ...normalized.session.runtime,
        artifacts:
          normalized.session.runtime.artifacts ??
          current.session.runtime.artifacts,
        javascript:
          normalized.session.runtime.javascript ??
          current.session.runtime.javascript,
        petri:
          normalized.session.runtime.petri ?? current.session.runtime.petri,
      },
    },
  };
}
