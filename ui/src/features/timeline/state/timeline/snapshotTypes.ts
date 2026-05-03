import type {
  DashboardSnapshot,
  DashboardTrace,
  DashboardWorkstationRequest,
} from "../../../../api/dashboard";
import type { FactoryRelation } from "../../../../api/events";
import type { TimelineWorkRequestPayload } from "./types";

export interface FactoryTimelineSnapshot {
  dashboard: DashboardSnapshot;
  relationsByWorkID: Record<string, FactoryRelation[]>;
  tracesByWorkID: Record<string, DashboardTrace>;
  workstationRequestsByDispatchID: Record<string, DashboardWorkstationRequest>;
  workRequestsByID: Record<string, TimelineWorkRequestPayload>;
}


