import type { ReactNode } from "react";

import { WorkChartCard } from "./d3-information-card";
import type { WorkChartModel } from "../lib/trends";
import type { WorkChartState } from "./work-chart";

export interface WorkOutcomeWidgetProps {
  chartState?: WorkChartState;
  headerAction?: ReactNode;
  locale?: string;
  model: WorkChartModel;
  widgetId?: string;
}

export function WorkOutcomeWidget({
  chartState,
  headerAction,
  locale,
  model,
  widgetId = "work-outcome-chart",
}: WorkOutcomeWidgetProps) {
  return (
    <WorkChartCard
      chartState={chartState}
      className="h-full min-h-0"
      headerAction={headerAction}
      locale={locale}
      model={model}
      widgetId={widgetId}
    />
  );
}
