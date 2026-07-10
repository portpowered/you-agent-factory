import type { components } from "../../../api/generated/openapi";

type FactoryDispatch = components["schemas"]["FactoryDispatch"];
type FactorySessionDurableLifecycleStatus =
  components["schemas"]["FactorySessionDurableLifecycleStatus"];

export type FactorySessionLifecycleActionID =
  | "pause"
  | "resume"
  | "cancel"
  | "terminate"
  | "approve"
  | "interrupt-dispatch"
  | "retry-dispatch";

export interface FactorySessionLifecycleActionAvailability {
  actions: FactorySessionLifecycleActionID[];
  selectedDispatch?: FactoryDispatch;
  showDispatchSelectionHint: boolean;
  showEmptyState: boolean;
}

const SESSION_TERMINAL_STATUSES = new Set<FactorySessionDurableLifecycleStatus>(
  ["CANCELED", "FAILED", "INTERRUPTED", "SUCCEEDED", "TERMINATED", "TIMED_OUT"],
);

const SESSION_CANCELABLE_STATUSES =
  new Set<FactorySessionDurableLifecycleStatus>([
    "AWAITING_APPROVAL",
    "PAUSED",
    "QUEUED",
    "RESUMING",
    "RUNNING",
  ]);

const SESSION_TERMINATABLE_STATUSES =
  new Set<FactorySessionDurableLifecycleStatus>([
    ...SESSION_CANCELABLE_STATUSES,
    "CANCELING",
  ]);

export function resolveFactorySessionLifecycleActionAvailability(input: {
  durableLifecycleStatus?: FactorySessionDurableLifecycleStatus;
  dispatches?: FactoryDispatch[];
  isDurableSession: boolean;
  selectedDispatchID: string | null;
}): FactorySessionLifecycleActionAvailability {
  const actions: FactorySessionLifecycleActionID[] = [];
  if (!input.isDurableSession) {
    return {
      actions,
      showDispatchSelectionHint: false,
      showEmptyState: false,
    };
  }

  const selectedDispatch = input.dispatches?.find(
    (dispatch) => dispatch.id === input.selectedDispatchID,
  );

  switch (input.durableLifecycleStatus) {
    case "RUNNING":
      actions.push("pause");
      break;
    case "PAUSED":
      actions.push("resume");
      break;
    case "AWAITING_APPROVAL":
      actions.push("approve");
      break;
  }

  if (
    input.durableLifecycleStatus &&
    SESSION_CANCELABLE_STATUSES.has(input.durableLifecycleStatus)
  ) {
    actions.push("cancel");
  }

  if (
    input.durableLifecycleStatus &&
    SESSION_TERMINATABLE_STATUSES.has(input.durableLifecycleStatus)
  ) {
    actions.push("terminate");
  }

  if (selectedDispatch?.status === "FAILED") {
    actions.push("retry-dispatch");
  }

  if (selectedDispatch?.status === "RUNNING") {
    actions.push("interrupt-dispatch");
  }

  const hasControllableDispatch = (input.dispatches ?? []).some(
    (dispatch) => dispatch.status === "FAILED" || dispatch.status === "RUNNING",
  );
  const showDispatchSelectionHint =
    hasControllableDispatch &&
    selectedDispatch?.status !== "FAILED" &&
    selectedDispatch?.status !== "RUNNING";

  return {
    actions,
    selectedDispatch,
    showDispatchSelectionHint,
    showEmptyState:
      actions.length === 0 &&
      (!input.durableLifecycleStatus ||
        SESSION_TERMINAL_STATUSES.has(input.durableLifecycleStatus)),
  };
}
