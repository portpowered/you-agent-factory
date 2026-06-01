import type { DashboardWorkstationNode } from "../../../../api/dashboard/types";
import { workstationRequiresWorkerAssignment } from "../../../current-factory-definition/lib/workstation-worker-assignment";
import type { WorkstationDetailCardProps } from "../lib/detail-card-types";
import type { getWorkstationDetailMessages } from "../messages/workstation-detail";
import { resolveWorkerBackedWorkstationSummaryRunnerValue } from "./workstation-runner-field";

const LOGICAL_MOVE_TOPOLOGY_KIND = "LOGICAL_MOVE";

export function resolveWorkstationSummaryRequiresWorkerAssignment(
  state: WorkstationDetailCardProps["editableConfigurationState"],
  selectedNode: DashboardWorkstationNode,
): boolean {
  if (state?.status === "ready") {
    return workstationRequiresWorkerAssignment({
      type: state.initialValues.workstationType,
    });
  }

  if (
    selectedNode.workstation_kind?.trim().toUpperCase() ===
    LOGICAL_MOVE_TOPOLOGY_KIND
  ) {
    return false;
  }

  return true;
}

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
): string | null {
  if (!resolveWorkstationSummaryRequiresWorkerAssignment(state, selectedNode)) {
    return null;
  }

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

export function resolveWorkstationSummaryRunnerValue(
  state: WorkstationDetailCardProps["editableConfigurationState"],
  messages: ReturnType<typeof getWorkstationDetailMessages>,
  selectedNode: DashboardWorkstationNode,
): string | null {
  if (!resolveWorkstationSummaryRequiresWorkerAssignment(state, selectedNode)) {
    return null;
  }

  return resolveWorkerBackedWorkstationSummaryRunnerValue(state, messages);
}
