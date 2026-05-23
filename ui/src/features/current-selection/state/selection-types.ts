import type {
  DashboardWorkItemRef,
} from "../../../api/dashboard/types";
export type { TerminalWorkDetail } from "../../terminal-work/public";

export type {
  DashboardSelection,
  DashboardWorkItemSelection,
  DashboardWorkstationRequestSelection,
} from "./dashboardSelection";

export interface StatePositionWorkItem extends DashboardWorkItemRef {
  startedAt?: string;
  started_at?: string;
}
