import { WorkChartCard } from "./d3-information-card";
import type { WorkChartModel } from "./trends";

export interface WorkOutcomeWidgetProps {
  model: WorkChartModel;
  widgetId?: string;
}

export function WorkOutcomeWidget({
  model,
  widgetId = "work-outcome-chart",
}: WorkOutcomeWidgetProps) {
  return (
    <WorkChartCard
      className="min-h-72"
      model={model}
      widgetId={widgetId}
    />
  );
}

