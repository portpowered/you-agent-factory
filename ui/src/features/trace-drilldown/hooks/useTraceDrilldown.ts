import type { DashboardTrace } from "../../../api/dashboard/types";
import type { StreamDerivedCacheIdentity } from "../../timeline/public/stream-identity";
import type { TraceGridState } from "../components/trace-grid-card";
import { getTraceDrilldownMessages } from "../messages/trace-drilldown";
import { useDashboardTrace } from "./useTrace";

export interface UseTraceDrilldownResult {
  selectedTrace: ReturnType<typeof useDashboardTrace>["data"];
  traceGridState: TraceGridState;
}

export interface ResolveTraceGridStateOptions {
  error: unknown;
  idleMessage: string;
  isLoading: boolean;
  selectedTrace: DashboardTrace | undefined;
  selectedWorkID: string | null;
}

export function resolveTraceGridState({
  error,
  idleMessage,
  isLoading,
  selectedTrace,
  selectedWorkID,
}: ResolveTraceGridStateOptions): TraceGridState {
  if (selectedWorkID === null) {
    return { message: idleMessage, status: "idle" };
  }

  if (error) {
    return {
      message: error instanceof Error ? error.message : String(error),
      status: "error",
    };
  }

  if (isLoading) {
    return { status: "loading", workID: selectedWorkID };
  }

  const hasTraceContent =
    (selectedTrace?.dispatches.length ?? 0) > 0 ||
    (selectedTrace?.relations?.length ?? 0) > 0;

  if (selectedTrace && hasTraceContent) {
    return { status: "ready", trace: selectedTrace };
  }

  return { status: "empty", workID: selectedWorkID };
}

export function useTraceDrilldown(
  selectedWorkID: string | null,
  selectedTraceID?: string | null,
  locale?: string | null,
  streamIdentity?: StreamDerivedCacheIdentity | null,
): UseTraceDrilldownResult {
  const messages = getTraceDrilldownMessages(locale);
  const traceQuery = useDashboardTrace(
    selectedWorkID,
    selectedTraceID,
    streamIdentity,
  );
  const selectedTrace = traceQuery.data;
  const traceGridState = resolveTraceGridState({
    error: traceQuery.error,
    idleMessage: messages.idleMessage,
    isLoading: traceQuery.isLoading,
    selectedTrace,
    selectedWorkID,
  });

  return { selectedTrace, traceGridState };
}
