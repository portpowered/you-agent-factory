import type { components } from "../../../api/generated/openapi";
import type { DashboardStatusPillTone } from "../../../components/ui/dashboard-status-pill";

import { getFactorySessionDetailMessages } from "./factory-session-detail";

export function resolveFactoryDispatchStatusTone({
  status,
  warningCount = 0,
}: {
  status: string;
  warningCount?: number;
}): DashboardStatusPillTone {
  if (status === "FAILED") {
    return "danger";
  }

  if (status === "COMPLETED" && warningCount > 0) {
    return "warning";
  }

  if (status === "COMPLETED") {
    return "success";
  }

  if (status === "RUNNING") {
    return "active";
  }

  if (status === "QUEUED") {
    return "info";
  }

  return "neutral";
}

export function formatFactorySessionRuntimeStatus(
  runtimeStatus: components["schemas"]["FactorySessionStatus"],
  durableLifecycleStatus?: components["schemas"]["FactorySessionDurableLifecycleStatus"],
  locale?: string | null,
): string {
  const messages = getFactorySessionDetailMessages(locale);

  if (durableLifecycleStatus) {
    return (
      messages.durableLifecycleStatusLabels[durableLifecycleStatus] ??
      durableLifecycleStatus
    );
  }

  return messages.runtimeStatusLabels[runtimeStatus] ?? runtimeStatus;
}

export function formatFactoryOrchestratorKind(
  orchestratorKind: components["schemas"]["FactoryOrchestratorKind"],
  locale?: string | null,
): string {
  const messages = getFactorySessionDetailMessages(locale);
  return messages.orchestratorKindLabels[orchestratorKind] ?? orchestratorKind;
}

export function formatFactorySessionScriptStatus(
  scriptStatus: components["schemas"]["FactorySessionJavaScriptScriptStatus"],
  locale?: string | null,
): string {
  const messages = getFactorySessionDetailMessages(locale);
  return messages.scriptStatusLabels[scriptStatus] ?? scriptStatus;
}
