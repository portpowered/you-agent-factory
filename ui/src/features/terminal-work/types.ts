import type {
  DashboardProviderSessionAttempt,
  DashboardWorkItemRef,
} from "../../api/dashboard/types";

export type TerminalWorkStatus = "completed" | "failed";

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
