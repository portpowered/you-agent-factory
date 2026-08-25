import type { DashboardWorkstationNode } from "../../../../../api/dashboard/types";
import { WorkstationType } from "../../../../../api/generated/openapi";
import { workstationRequiresWorkerAssignment } from "../../../../current-factory-definition/lib/workstation-worker-assignment";
import {
  workstationBehaviorSemanticKind,
  workstationGraphPresentation,
} from "../../../../flowchart/lib/workstation-graph-presentation";
import { STANDARD_WORKSTATION_KIND } from "../../../../flowchart/lib/workstation-icon-metadata";
import type { WorkstationDetailCardProps } from "../../lib/keys/detail-card-types";
import type { getWorkstationDetailMessages } from "../../messages/workstation-detail";
import { resolveWorkerBackedWorkstationSummaryRunnerValue } from "./workstation-runner-field";

export function resolveWorkstationSummaryRequiresWorkerAssignment(
  state: WorkstationDetailCardProps["editableConfigurationState"],
  selectedNode: DashboardWorkstationNode,
): boolean {
  if (state?.status === "ready") {
    return workstationRequiresWorkerAssignment({
      type: state.draft.workstationType,
    });
  }

  if (
    selectedNode.workstation_kind?.trim().toUpperCase() ===
    WorkstationType.LOGICAL_MOVE
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

  return messages.localizeWorkstationType(state.draft.workstationType);
}

export function resolveWorkstationSummaryWorkerTypeValue(
  state: WorkstationDetailCardProps["editableConfigurationState"],
  selectedNode: DashboardWorkstationNode,
  messages: ReturnType<typeof getWorkstationDetailMessages>,
): string {
  if (state?.status === "ready") {
    const workerType =
      state.initialValues.workerTypeByName[state.draft.workerName];
    if (workerType) {
      return workerType;
    }
  }

  return selectedNode.worker_type || messages.unknownWorkerTypeValue;
}

export function resolveWorkstationSummaryPresentation(
  editableConfigurationState:
    | WorkstationDetailCardProps["editableConfigurationState"]
    | undefined,
  selectedNode: DashboardWorkstationNode,
  locale?: string | null,
) {
  const behavior =
    editableConfigurationState?.status === "ready"
      ? editableConfigurationState.draft.behavior
      : undefined;
  const presentation = workstationGraphPresentation(
    {
      ...selectedNode,
      workstation_kind:
        behavior !== undefined
          ? workstationBehaviorSemanticKind(behavior)
          : (selectedNode.workstation_kind ?? STANDARD_WORKSTATION_KIND),
    },
    locale,
  );

  return presentation.semanticKind === STANDARD_WORKSTATION_KIND
    ? null
    : presentation;
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
