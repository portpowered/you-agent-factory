import type { ReactNode } from "react";

import { TraceGridBentoCard } from "./trace-grid-card";
import type { TraceGridState } from "./trace-grid-card";

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
      // tailwind-exception: intrinsic-sizing
      className="h-full min-h-[24rem]"
      headerAction={headerAction}
      locale={locale}
      onSelectWorkID={onSelectWorkID}
      state={state}
      widgetId={widgetId}
    />
  );
}
