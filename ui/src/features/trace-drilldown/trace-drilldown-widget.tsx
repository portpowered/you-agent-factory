import { TraceGridBentoCard } from "./trace-grid-card";
import type { TraceGridState } from "./trace-grid-card";

// tailwind-exception: intrinsic-sizing
const TRACE_DRILLDOWN_WIDGET_CLASS = "h-full min-h-[34rem]";

export interface TraceDrilldownWidgetProps {
  locale?: string;
  onSelectWorkID?: (workID: string) => void;
  state: TraceGridState;
  widgetId?: string;
}

export function TraceDrilldownWidget({
  locale,
  onSelectWorkID,
  state,
  widgetId = "trace",
}: TraceDrilldownWidgetProps) {
  return (
    <TraceGridBentoCard
      className={TRACE_DRILLDOWN_WIDGET_CLASS}
      locale={locale}
      onSelectWorkID={onSelectWorkID}
      state={state}
      widgetId={widgetId}
    />
  );
}
