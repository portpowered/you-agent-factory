import type { ReactNode } from "react";

import { WorkChartCard } from "./d3-information-card";
import type { WorkChartModel } from "../lib/trends";

export interface WorkOutcomeWidgetProps {
  headerAction?: ReactNode;
  locale?: string;
  model: WorkChartModel;
  widgetId?: string;
}

export function WorkOutcomeWidget({
  headerAction,
  locale,
  model,
  widgetId = "work-outcome-chart",
}: WorkOutcomeWidgetProps) {
  return (
    <WorkChartCard
      className="h-full min-h-0"
      headerAction={headerAction}
      locale={locale}
      model={model}
      widgetId={widgetId}
    />
  );
}
