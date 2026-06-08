import type { DashboardSelection } from "../../base/state/selection-types";
import type { CurrentSelectionState } from "../../hooks/core/useCurrentSelection";
import { EditableResourceConfigurationHeaderActions } from "../../resource-selection/components/resource-save-controls";
import type { useEditableResourceConfigurationState } from "../../resource-selection/hooks/use-editable-resource-configuration-state";
import { useSaveEditableResourceConfiguration } from "../../resource-selection/hooks/use-save-editable-resource-configuration";
import { EditableWorkStateConfigurationHeaderActions } from "../../work-state-selection/components/work-state-save-controls";
import type { useEditableWorkStateConfigurationState } from "../../work-state-selection/hooks/use-editable-work-state-configuration-state";
import { useSaveEditableWorkStateConfiguration } from "../../work-state-selection/hooks/use-save-editable-work-state-configuration";
import {
  EditableWorkTypeConfigurationHeaderActions,
  EditableWorkTypeSaveDialog,
} from "../../work-type-selection/components/work-type-save-controls";
import type { useEditableWorkTypeConfigurationState } from "../../work-type-selection/hooks/use-editable-work-type-configuration-state";
import {
  type UseSaveEditableWorkTypeConfigurationResult,
  useSaveEditableWorkTypeConfiguration,
} from "../../work-type-selection/hooks/use-save-editable-work-type-configuration";
import { EditableWorkerConfigurationHeaderActions } from "../../worker-selection/components/worker-save-controls";
import type { useEditableWorkerConfigurationState } from "../../worker-selection/hooks/use-editable-worker-configuration-state";
import { useSaveEditableWorkerConfiguration } from "../../worker-selection/hooks/use-save-editable-worker-configuration";
import { EditableWorkstationConfigurationHeaderActions } from "../../workstation-selection/components/editable/workstation-save-controls";
import type { useEditableWorkstationConfigurationState } from "../../workstation-selection/hooks/use-editable-workstation-configuration-state";
import { useSaveEditableWorkstationConfiguration } from "../../workstation-selection/hooks/use-save-editable-workstation-configuration";
import { buildWorkstationSaveScopeKey } from "../../workstation-selection/lib/keys/workstation-save-scope-key";

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: one hook wires save scopes and paired header actions for every editable selection kind.
export function useCurrentSelectionDetailSave({
  currentSelection,
  editableConfigurationState,
  editableResourceConfigurationState,
  editableWorkStateConfigurationState,
  editableWorkerConfigurationState,
  editableWorkTypeConfigurationState,
  locale,
  selectedNode,
  selectedResourceName,
  selectedWorkerName,
  selectedWorkTypeName,
  selection,
  workStatePlaceId,
}: {
  currentSelection: CurrentSelectionState;
  editableConfigurationState: ReturnType<
    typeof useEditableWorkstationConfigurationState
  >;
  editableResourceConfigurationState: ReturnType<
    typeof useEditableResourceConfigurationState
  >;
  editableWorkStateConfigurationState: ReturnType<
    typeof useEditableWorkStateConfigurationState
  >;
  editableWorkerConfigurationState: ReturnType<
    typeof useEditableWorkerConfigurationState
  >;
  editableWorkTypeConfigurationState: ReturnType<
    typeof useEditableWorkTypeConfigurationState
  >;
  locale?: string | null;
  selectedNode: CurrentSelectionState["selectedNode"];
  selectedResourceName: string | null;
  selectedWorkerName: string | null;
  selectedWorkTypeName: string | null;
  selection: DashboardSelection | null;
  workStatePlaceId: string | null;
}) {
  const workstationSaveScopeKey =
    selection?.kind === "node" && selectedNode
      ? buildWorkstationSaveScopeKey({
          nodeId: selectedNode.node_id,
          transitionId: selectedNode.transition_id,
          workstationName: selectedNode.workstation_name,
        })
      : null;
  const workstationSave = useSaveEditableWorkstationConfiguration({
    editableConfigurationState,
    locale,
    onWorkstationRenamed: currentSelection.selectWorkstation,
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
  const workStateSave = useSaveEditableWorkStateConfiguration({
    editableConfigurationState: editableWorkStateConfigurationState,
    locale,
    onWorkStateRenamed: currentSelection.selectStateNode,
    scopeKey: workStatePlaceId,
  });
  const workTypeSaveScopeKey =
    selection?.kind === "work-type" && selectedWorkTypeName
      ? selectedWorkTypeName
      : null;
  const workTypeSave = useSaveEditableWorkTypeConfiguration({
    editableConfigurationState: editableWorkTypeConfigurationState,
    locale,
    onWorkTypeRenamed: currentSelection.selectWorkType,
    scopeKey: workTypeSaveScopeKey,
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
      onSave={() => void workstationSave.save()}
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
    <EditableResourceConfigurationHeaderActions
      canDiscard={
        editableResourceConfigurationState?.status === "ready" &&
        editableResourceConfigurationState.isDirty
      }
      canSave={resourceSave.canSave}
      locale={locale ?? undefined}
      onDiscard={() => {
        if (editableResourceConfigurationState?.status === "ready") {
          editableResourceConfigurationState.onResetToLatest();
        }
      }}
      onSave={() => void resourceSave.save()}
      saveState={resourceSave.saveState}
    />
  );
  const workStateHeaderAction = (
    <EditableWorkStateConfigurationHeaderActions
      canDiscard={
        editableWorkStateConfigurationState?.status === "ready" &&
        editableWorkStateConfigurationState.isDirty
      }
      canSave={workStateSave.canSave}
      locale={locale ?? undefined}
      onDiscard={() => {
        if (editableWorkStateConfigurationState?.status === "ready") {
          editableWorkStateConfigurationState.onResetToLatest();
        }
      }}
      onSave={() => void workStateSave.save()}
      saveState={workStateSave.saveState}
    />
  );
  const workTypeHeaderAction = (
    <EditableWorkTypeConfigurationHeaderActions
      canDiscard={
        editableWorkTypeConfigurationState?.status === "ready" &&
        editableWorkTypeConfigurationState.isDirty
      }
      canSave={workTypeSave.canSave}
      locale={locale ?? undefined}
      onDiscard={() => {
        if (editableWorkTypeConfigurationState?.status === "ready") {
          editableWorkTypeConfigurationState.onResetToLatest();
        }
      }}
      onSave={workTypeSave.beginSaveConfirmation}
      saveState={workTypeSave.saveState}
    />
  );

  return {
    resourceHeaderAction,
    resourceSave,
    resourceSaveState: resourceSave.saveState,
    saveWorkerConfiguration: () => void workerSave.save(),
    saveWorkStateConfiguration: () => void workStateSave.save(),
    workstationHeaderAction,
    workstationSave,
    workstationSaveState: workstationSave.saveState,
    workerHeaderAction,
    workerSave,
    workerSaveState: workerSave.saveState,
    workStateHeaderAction,
    workStateSave,
    workStateSaveState: workStateSave.saveState,
    workTypeHeaderAction,
    workTypeSave,
    workTypeSaveState: workTypeSave.saveState,
  };
}

export function CurrentSelectionWorkTypeSaveDialog({
  locale,
  workTypeSave,
}: {
  locale?: string | null;
  workTypeSave: UseSaveEditableWorkTypeConfigurationResult;
}) {
  return (
    <EditableWorkTypeSaveDialog
      locale={locale ?? undefined}
      onCancel={workTypeSave.cancelSaveConfirmation}
      onConfirm={() => void workTypeSave.confirmSave()}
      saveState={workTypeSave.saveState}
    />
  );
}
