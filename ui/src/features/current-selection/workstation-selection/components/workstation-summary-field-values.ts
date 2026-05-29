import type { DashboardWorkstationNode } from "../../../../api/dashboard/types";
import type { WorkstationDetailCardProps } from "../lib/detail-card-types";
import type { getWorkstationDetailMessages } from "../messages/workstation-detail";

function normalizeTopologyWorkstationKind(
  workstationKind: string | undefined,
  defaultKind: string,
): string {
  return (workstationKind?.trim() || defaultKind).toUpperCase();
}

export function resolveWorkstationSummaryKindValue(
  state: WorkstationDetailCardProps["editableConfigurationState"],
  selectedNode: DashboardWorkstationNode,
  messages: ReturnType<typeof getWorkstationDetailMessages>,
): string {
  if (!state) {
    return messages.localizeWorkstationBehavior(
      normalizeTopologyWorkstationKind(
        selectedNode.workstation_kind,
        messages.kindDefaultValue,
      ),
    );
  }

  if (state.status === "loading") {
    return messages.workstationKindLoadingValue;
  }

  if (state.status === "error" || state.status === "empty") {
    return messages.unavailableWorkstationKindValue;
  }

  return messages.localizeWorkstationBehavior(state.draft.behavior);
}

export function resolveWorkstationSummaryTypeValue(
  state: WorkstationDetailCardProps["editableConfigurationState"],
  messages: ReturnType<typeof getWorkstationDetailMessages>,
): string {
  if (!state || state.status === "loading") {
    return messages.workstationTypeLoadingValue;
  }

  if (state.status === "error" || state.status === "empty") {
    return messages.unavailableWorkstationTypeValue;
  }

  return messages.localizeWorkstationType(state.initialValues.workstationType);
}

export { resolveWorkstationSummaryRunnerValue } from "./workstation-runner-field";
