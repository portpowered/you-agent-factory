import type { TraceGridState } from "./trace-grid-card";
import { useDashboardTrace } from "./useTrace";

export interface UseTraceDrilldownResult {
  selectedTrace: ReturnType<typeof useDashboardTrace>["data"];
  traceGridState: TraceGridState;
}

export function useTraceDrilldown(
  selectedWorkID: string | null,
  selectedTraceID?: string | null,
): UseTraceDrilldownResult {
  const traceQuery = useDashboardTrace(selectedWorkID, selectedTraceID);
  const selectedTrace = traceQuery.data;
  const traceUnavailable =
    selectedWorkID !== null &&
    (selectedTrace === undefined || selectedTrace.dispatches.length === 0);
  const traceGridState: TraceGridState =
    selectedWorkID === null
      ? {
          status: "idle",
          message: "Select active, completed, or failed work to inspect retained trace history.",
        }
      : traceQuery.isLoading
        ? { status: "loading", workID: selectedWorkID }
        : traceUnavailable
          ? { status: "empty", workID: selectedWorkID }
          : traceQuery.error instanceof Error
            ? { status: "error", message: traceQuery.error.message }
            : selectedTrace
              ? { status: "ready", trace: selectedTrace }
              : {
                  status: "idle",
                  message: "Select active, completed, or failed work to inspect retained trace history.",
                };

  return { selectedTrace, traceGridState };
}

