import type { ReactNode } from "react";
import type { TraceSelectionIdentity } from "../lib/trace-selection";
import type { TraceGridState } from "./trace-grid-card";
import { TraceGridBentoCard } from "./trace-grid-card";

export interface TraceDrilldownWidgetProps {
  headerAction?: ReactNode;
  locale?: string;
  onSelectWorkID?: (workID: string) => void;
  onSelectTraceSelection?: (selection: TraceSelectionIdentity) => void;
  selectedTraceSelection?: TraceSelectionIdentity | null;
  state: TraceGridState;
  widgetId?: string;
}

export function TraceDrilldownWidget({
  headerAction,
  locale,
  onSelectWorkID,
  onSelectTraceSelection,
  selectedTraceSelection,
  state,
  widgetId = "trace",
}: TraceDrilldownWidgetProps) {
  return (
    <TraceGridBentoCard
      // tailwind-exception: intrinsic-sizing
      className="min-h-[24rem]"
      headerAction={headerAction}
      locale={locale}
      onSelectWorkID={onSelectWorkID}
      onSelectTraceSelection={onSelectTraceSelection}
      selectedTraceSelection={selectedTraceSelection}
      state={state}
      widgetId={widgetId}
    />
  );
}
