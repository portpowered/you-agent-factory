import {
  FactorySessionDurableLifecycleStatus,
  FactorySessionResultStatus,
  type components,
} from "../../../api/generated/openapi";
import type { FactorySessionDetailData } from "../hooks/use-factory-session-detail";

export type DurableInspectionPresentation = "partial" | "terminal";

const TERMINAL_DURABLE_LIFECYCLE_STATUSES = new Set<string>([
  FactorySessionDurableLifecycleStatus.FactorySessionDurableLifecycleStatusSucceeded,
  FactorySessionDurableLifecycleStatus.FactorySessionDurableLifecycleStatusFailed,
  FactorySessionDurableLifecycleStatus.FactorySessionDurableLifecycleStatusCanceled,
  FactorySessionDurableLifecycleStatus.FactorySessionDurableLifecycleStatusTimedOut,
  FactorySessionDurableLifecycleStatus.FactorySessionDurableLifecycleStatusInterrupted,
  FactorySessionDurableLifecycleStatus.FactorySessionDurableLifecycleStatusTerminated,
]);

const TERMINAL_DURABLE_RESULT_STATUSES = new Set<string>([
  FactorySessionResultStatus.FactorySessionResultStatusFinal,
  FactorySessionResultStatus.FactorySessionResultStatusFailedWithPartial,
]);

export function isDurableTerminalLifecycleStatus(
  status: components["schemas"]["FactorySessionDurableLifecycleStatus"],
): boolean {
  return TERMINAL_DURABLE_LIFECYCLE_STATUSES.has(status);
}

export function resolveDurableResultStatus(
  data: Extract<FactorySessionDetailData, { kind: "durable" }>,
): components["schemas"]["FactorySessionResultStatus"] | undefined {
  return (
    data.durableResult?.resultStatus ??
    data.durablePartialResult?.resultStatus ??
    data.session.resultSummary?.resultStatus
  );
}

export function resolveDurableInspectionPresentation(
  data: Extract<FactorySessionDetailData, { kind: "durable" }>,
): DurableInspectionPresentation {
  const resultStatus = resolveDurableResultStatus(data);

  if (resultStatus && TERMINAL_DURABLE_RESULT_STATUSES.has(resultStatus)) {
    return "terminal";
  }

  if (isDurableTerminalLifecycleStatus(data.session.status)) {
    return "terminal";
  }

  return "partial";
}
