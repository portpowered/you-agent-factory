import type { ReactNode } from "react";

import { TraceGridBentoCard } from "./trace-grid-card";
import type { TraceGridState } from "./trace-grid-card";

// tailwind-exception: intrinsic-sizing
const TRACE_DRILLDOWN_WIDGET_CLASS = "h-full min-h-[24rem]";

export interface TraceDrilldownWidgetProps {
  headerAction?: ReactNode;
  locale?: string;
  onSelectWorkID?: (workID: string) => void;
  state: TraceGridState;
  widgetId?: string;
}

export function TraceDrilldownWidget({
  headerAction,
  locale,
  onSelectWorkID,
  state,
  widgetId = "trace",
}: TraceDrilldownWidgetProps) {
  return (
    <TraceGridBentoCard
      className={TRACE_DRILLDOWN_WIDGET_CLASS}
      headerAction={headerAction}
      locale={locale}
      onSelectWorkID={onSelectWorkID}
      state={state}
      widgetId={widgetId}
    />
  );
}
