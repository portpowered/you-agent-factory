import { WorkChartCard } from "./d3-information-card";
import type { WorkChartModel } from "../trends";

export interface WorkOutcomeWidgetProps {
  locale?: string;
  model: WorkChartModel;
  widgetId?: string;
}

export function WorkOutcomeWidget({
  locale,
  model,
  widgetId = "work-outcome-chart",
}: WorkOutcomeWidgetProps) {
  return (
    <WorkChartCard
      className="min-h-72"
      locale={locale}
      model={model}
      widgetId={widgetId}
    />
  );
}
