import type { DashboardSelection } from "../base/state/selection-types";
import type { CurrentSelectionState } from "../hooks/useCurrentSelection";
import { EditableResourceSaveHeaderAction } from "../resource-selection/components/resource-save-controls";
import type { useEditableResourceConfigurationState } from "../resource-selection/hooks/use-editable-resource-configuration-state";
import { useSaveEditableResourceConfiguration } from "../resource-selection/hooks/use-save-editable-resource-configuration";
import type { useEditableWorkerConfigurationState } from "../worker-selection/hooks/use-editable-worker-configuration-state";
import { useSaveEditableWorkerConfiguration } from "../worker-selection/hooks/use-save-editable-worker-configuration";
import { EditableWorkerConfigurationHeaderActions } from "../worker-selection/components/worker-save-controls";
import { EditableWorkstationConfigurationHeaderActions } from "../workstation-selection/components/workstation-save-controls";
import type { useEditableWorkstationConfigurationState } from "../workstation-selection/hooks/use-editable-workstation-configuration-state";
import {
  type UseSaveEditableWorkstationConfigurationResult,
  useSaveEditableWorkstationConfiguration,
} from "../workstation-selection/hooks/use-save-editable-workstation-configuration";
import { EditableWorkstationSaveDialog } from "../workstation-selection/public";

export function useCurrentSelectionDetailSave({
  currentSelection,
  editableConfigurationState,
  editableResourceConfigurationState,
  editableWorkerConfigurationState,
  locale,
  selectedNode,
  selectedResourceName,
  selectedWorkerName,
  selection,
}: {
  currentSelection: CurrentSelectionState;
  editableConfigurationState: ReturnType<
    typeof useEditableWorkstationConfigurationState
  >;
  editableResourceConfigurationState: ReturnType<
    typeof useEditableResourceConfigurationState
  >;
  editableWorkerConfigurationState: ReturnType<
    typeof useEditableWorkerConfigurationState
  >;
  locale?: string | null;
  selectedNode: CurrentSelectionState["selectedNode"];
  selectedResourceName: string | null;
  selectedWorkerName: string | null;
  selection: DashboardSelection | null;
}) {
  const workstationSaveScopeKey =
    selection?.kind === "node" && selectedNode
      ? `${selectedNode.node_id}:${selectedNode.transition_id}:${selectedNode.workstation_name}`
      : null;
  const workstationSave = useSaveEditableWorkstationConfiguration({
    editableConfigurationState,
    locale,
    scopeKey: workstationSaveScopeKey,
  });
  const workerSaveScopeKey =
    selection?.kind === "worker" && selectedWorkerName
      ? selectedWorkerName
      : null;
  const workerSave = useSaveEditableWorkerConfiguration({
    editableConfigurationState: editableWorkerConfigurationState,
    locale,
    onWorkerRenamed: currentSelection.selectWorker,
    scopeKey: workerSaveScopeKey,
  });
  const resourceSaveScopeKey =
    selection?.kind === "resource" && selectedResourceName
      ? selectedResourceName
      : null;
  const resourceSave = useSaveEditableResourceConfiguration({
    editableConfigurationState: editableResourceConfigurationState,
    locale,
    onResourceRenamed: currentSelection.selectResource,
    scopeKey: resourceSaveScopeKey,
  });

  const workstationHeaderAction = (
    <EditableWorkstationConfigurationHeaderActions
      canDiscard={
        editableConfigurationState?.status === "ready" &&
        editableConfigurationState.isDirty
      }
      canSave={workstationSave.canSave}
      locale={locale ?? undefined}
      onDiscard={() => {
        if (editableConfigurationState?.status === "ready") {
          editableConfigurationState.onResetToLatest();
        }
      }}
      onSave={workstationSave.beginSaveConfirmation}
      saveState={workstationSave.saveState}
    />
  );
  const workerHeaderAction = (
    <EditableWorkerConfigurationHeaderActions
      canDiscard={
        editableWorkerConfigurationState?.status === "ready" &&
        editableWorkerConfigurationState.isDirty
      }
      canSave={workerSave.canSave}
      locale={locale ?? undefined}
      onDiscard={() => {
        if (editableWorkerConfigurationState?.status === "ready") {
          editableWorkerConfigurationState.onResetToLatest();
        }
      }}
      onSave={() => void workerSave.save()}
      saveState={workerSave.saveState}
    />
  );
  const resourceHeaderAction = (
    <EditableResourceSaveHeaderAction
      canSave={resourceSave.canSave}
      locale={locale ?? undefined}
      onClick={() => void resourceSave.save()}
      saveState={resourceSave.saveState}
    />
  );

  return {
    resourceHeaderAction,
    resourceSaveState: resourceSave.saveState,
    saveWorkerConfiguration: () => void workerSave.save(),
    workstationHeaderAction,
    workstationSave,
    workstationSaveState: workstationSave.saveState,
    workerHeaderAction,
    workerSaveState: workerSave.saveState,
  };
}

export function CurrentSelectionWorkstationSaveDialog({
  editableConfigurationState,
  locale,
  workstationSave,
}: {
  editableConfigurationState: ReturnType<
    typeof useEditableWorkstationConfigurationState
  >;
  locale?: string | null;
  workstationSave: UseSaveEditableWorkstationConfigurationResult;
}) {
  return (
    <EditableWorkstationSaveDialog
      locale={locale ?? undefined}
      onCancel={workstationSave.cancelSaveConfirmation}
      onConfirm={() => void workstationSave.confirmSave()}
      overwriteFieldNames={
        editableConfigurationState?.status === "ready"
          ? editableConfigurationState.overwriteFieldNames
          : []
      }
      saveState={workstationSave.saveState}
    />
  );
}
