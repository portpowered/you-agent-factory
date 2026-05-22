import type {
  DashboardWorkItemRef,
} from "../../api/dashboard/types";
export type { TerminalWorkDetail } from "../terminal-work";

export type {
  DashboardSelection,
  DashboardWorkItemSelection,
  DashboardWorkstationRequestSelection,
} from "./state/dashboardSelection";

export interface StatePositionWorkItem extends DashboardWorkItemRef {
  startedAt?: string;
  started_at?: string;
}
