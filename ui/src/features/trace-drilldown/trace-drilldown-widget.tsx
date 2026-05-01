import { TraceGridBentoCard } from "./trace-grid-card";
import type { TraceGridState } from "./trace-grid-card";

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
      className="h-full min-h-[34rem]"
      onSelectWorkID={onSelectWorkID}
      state={state}
      widgetId={widgetId}
    />
  );
}
