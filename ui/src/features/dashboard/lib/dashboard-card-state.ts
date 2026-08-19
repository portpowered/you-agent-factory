import type { DashboardStreamState } from "../../../api/dashboard/types";
import type { FactoryTimelineMode } from "../../timeline/state/timeline/storeState";

export interface DashboardCardStateContext {
  hasAuthoritativeSnapshot: boolean;
  recoveryPending: boolean;
  streamStatus: DashboardStreamState["status"];
  timelineMode: FactoryTimelineMode;
}

export interface DashboardCardStateSnapshot {
  value: unknown;
  widgetType: string;
}
