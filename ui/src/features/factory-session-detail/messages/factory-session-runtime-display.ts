import type { components } from "../../../api/generated/openapi";

import { getFactorySessionDetailMessages } from "./factory-session-detail";

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
  return (
    messages.orchestratorKindLabels[orchestratorKind] ?? orchestratorKind
  );
}

export function formatFactorySessionScriptStatus(
  scriptStatus: components["schemas"]["FactorySessionJavaScriptScriptStatus"],
  locale?: string | null,
): string {
  const messages = getFactorySessionDetailMessages(locale);
  return messages.scriptStatusLabels[scriptStatus] ?? scriptStatus;
}
