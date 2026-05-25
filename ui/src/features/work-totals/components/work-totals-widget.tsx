import type { ReactNode } from "react";

import type { DashboardSnapshot } from "../../../api/dashboard/types";
import { WorkTotalsCard } from "./work-totals-card";

export interface WorkTotalsWidgetProps {
  headerAction?: ReactNode;
  locale?: string;
  snapshot: DashboardSnapshot;
}

export function WorkTotalsWidget({
  headerAction,
  locale,
  snapshot,
}: WorkTotalsWidgetProps) {
  return (
    <WorkTotalsCard
      completedCount={snapshot.runtime.session.completed_count}
      dispatchedCount={snapshot.runtime.session.dispatched_count}
      failedCount={snapshot.runtime.session.failed_count}
      headerAction={headerAction}
      inFlightDispatchCount={snapshot.runtime.in_flight_dispatch_count}
      locale={locale}
    />
  );
}
