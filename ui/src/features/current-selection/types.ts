import type {
  DashboardProviderSessionAttempt,
  DashboardWorkItemRef,
} from "../../api/dashboard/types";

export type {
  DashboardSelection,
  DashboardWorkItemSelection,
  DashboardWorkstationRequestSelection,
} from "./state/dashboardSelection";

export interface TerminalWorkDetail {
  attempts?: DashboardProviderSessionAttempt[];
  dispatchID?: string;
  failureMessage?: string;
  failureReason?: string;
  label: string;
  preferWorkstationRequest?: boolean;
  status: "completed" | "failed";
  traceWorkID: string;
  workItem?: DashboardWorkItemRef;
}
