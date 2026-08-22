import type {
  DashboardProviderSessionAttempt,
  DashboardWorkItemRef,
} from "../../../api/dashboard/types";

export const TERMINAL_WORK_STATUSES = [
  "completed",
  "failed",
  "canceled",
  "terminated",
  "unknown",
] as const;

export type TerminalWorkStatus = (typeof TERMINAL_WORK_STATUSES)[number];

export interface TerminalWorkItem {
  attempts?: DashboardProviderSessionAttempt[];
  dispatchID?: string;
  failureMessage?: string;
  failureReason?: string;
  label: string;
  traceWorkID: string;
  workItem?: DashboardWorkItemRef;
  workstationName?: string;
}

export interface TerminalWorkDetail {
  attempts?: DashboardProviderSessionAttempt[];
  dispatchID?: string;
  failureMessage?: string;
  failureReason?: string;
  label: string;
  status: TerminalWorkStatus;
  traceWorkID: string;
  workItem?: DashboardWorkItemRef;
}

export type TerminalWorkSelection = Pick<
  TerminalWorkDetail,
  "dispatchID" | "status" | "traceWorkID" | "workItem"
>;

/** Return the canonical Work ID represented by a terminal row. */
export function terminalWorkID(
  item: Pick<TerminalWorkItem, "traceWorkID" | "workItem">,
): string {
  return item.workItem?.work_id ?? item.traceWorkID;
}

/** Qualify a Work ID with dispatch identity when the row has it. */
export function terminalWorkIdentity(
  item: Pick<TerminalWorkItem, "dispatchID" | "traceWorkID" | "workItem">,
): string {
  const workID = terminalWorkID(item);
  return item.dispatchID ? `${item.dispatchID}:${workID}` : workID;
}

export function terminalWorkSelectionMatches(
  item: TerminalWorkItem,
  selection: TerminalWorkSelection | null | undefined,
  status: TerminalWorkStatus,
): boolean {
  return (
    selection?.status === status &&
    terminalWorkIdentity(item) === terminalWorkIdentity(selection)
  );
}

export function terminalWorkStatusFromOutcome(
  outcome: string | undefined,
): TerminalWorkStatus | undefined {
  const normalizedOutcome = outcome?.trim().toUpperCase();
  if (!normalizedOutcome) {
    return undefined;
  }

  switch (normalizedOutcome) {
    case "ACCEPTED":
    case "COMPLETED":
    case "SUCCEEDED":
    case "SUCCESS":
    case "TERMINAL":
      return "completed";
    case "FAILED":
      return "failed";
    case "CANCELED":
    case "CANCELLED":
    case "INVOCATION_CANCELED":
      return "canceled";
    case "TERMINATED":
      return "terminated";
    default:
      return "unknown";
  }
}
