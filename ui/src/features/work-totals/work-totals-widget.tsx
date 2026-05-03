import type { DashboardSnapshot } from "../../api/dashboard/types";
import { WorkTotalsCard } from "./work-totals-card";

export interface WorkTotalsWidgetProps {
  snapshot: DashboardSnapshot;
}

export function WorkTotalsWidget({ snapshot }: WorkTotalsWidgetProps) {
  return (
    <WorkTotalsCard
      completedCount={snapshot.runtime.session.completed_count}
      dispatchedCount={snapshot.runtime.session.dispatched_count}
      failedCount={snapshot.runtime.session.failed_count}
      inFlightDispatchCount={snapshot.runtime.in_flight_dispatch_count}
    />
  );
}

