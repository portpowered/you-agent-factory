import { TraceGridBentoCard } from "./trace-grid-card";
import type { TraceGridState } from "./trace-grid-card";

// tailwind-exception: intrinsic-sizing
const TRACE_DRILLDOWN_WIDGET_CLASS = "h-full min-h-[34rem]";

export interface TraceDrilldownWidgetProps {
  onSelectWorkID?: (workID: string) => void;
  state: TraceGridState;
  widgetId?: string;
}

export function TraceDrilldownWidget({
  onSelectWorkID,
  state,
  widgetId = "trace",
}: TraceDrilldownWidgetProps) {
  return (
    <TraceGridBentoCard
      className={TRACE_DRILLDOWN_WIDGET_CLASS}
      onSelectWorkID={onSelectWorkID}
      state={state}
      widgetId={widgetId}
    />
  );
}
