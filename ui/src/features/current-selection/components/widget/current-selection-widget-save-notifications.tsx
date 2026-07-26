import { factoryBundledDocDisplayLabel } from "../../../workflow-activity/lib/factory-bundled-docs";
import { useFactoryGraphTopologyEditorBridge } from "../../../workflow-activity/state/factory-graph-topology-editor-bridge";
import { CurrentSelectionGraphDraftConflictNotifications } from "../../base/components/save/current-selection-graph-draft-conflict-notifications";
import { buildCurrentSelectionSaveToastMessages } from "../../base/lib/build-current-selection-save-toast-messages";
import { CurrentSelectionSaveNotifications } from "../../base/components/save/current-selection-save-notifications";
import type { DashboardSelection } from "../../base/state/selection-types";
import type { useEditableDocConfigurationState } from "../../doc-selection/hooks/use-editable-doc-configuration-state";
import type { CurrentSelectionState } from "../../hooks/core/useCurrentSelection";
import type { useEditableResourceConfigurationState } from "../../resource-selection/hooks/use-editable-resource-configuration-state";
import type { useEditableWorkStateConfigurationState } from "../../work-state-selection/hooks/use-editable-work-state-configuration-state";
import type { useEditableWorkTypeConfigurationState } from "../../work-type-selection/hooks/use-editable-work-type-configuration-state";
import type { useEditableWorkerConfigurationState } from "../../worker-selection/hooks/use-editable-worker-configuration-state";
import type { useEditableWorkstationConfigurationState } from "../../workstation-selection/hooks/use-editable-workstation-configuration-state";
import type { useCurrentSelectionDetailSave } from "./use-current-selection-detail-save";

function hasEditableDraftChanges(
  editableState: { status: string; isDirty?: boolean } | null | undefined,
): boolean {
  return editableState?.status === "ready" && editableState.isDirty === true;
}

function resolveEditableDisplayName({
  draftName,
  fallbackName,
  editableState,
}: {
  draftName: string;
  fallbackName: string;
  editableState: { status: "ready" } | { status: string } | null | undefined;
}): string {
  if (editableState?.status !== "ready") {
    return fallbackName;
  }
  return draftName.trim() || fallbackName;
}

export type CurrentSelectionWidgetSaveNotificationsProps = {
  docSave: ReturnType<typeof useCurrentSelectionDetailSave>["docSave"];
  docSaveState: ReturnType<
    typeof useCurrentSelectionDetailSave
  >["docSaveState"];
  editableConfigurationState: ReturnType<
    typeof useEditableWorkstationConfigurationState
  >;
  editableDocConfigurationState: ReturnType<
    typeof useEditableDocConfigurationState
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
  locale?: string;
  resourceSave: ReturnType<
    typeof useCurrentSelectionDetailSave
  >["resourceSave"];
  resourceSaveState: ReturnType<
    typeof useCurrentSelectionDetailSave
  >["resourceSaveState"];
  selectedDocTargetPath: string | null;
  selectedNode: CurrentSelectionState["selectedNode"];
  selectedResourceName: string | null;
  selectedWorkerName: string | null;
  selectedWorkTypeName: string | null;
  selection: DashboardSelection | null;
  workStatePlaceId: string | null;
  workStateSave: ReturnType<
    typeof useCurrentSelectionDetailSave
  >["workStateSave"];
  workerSave: ReturnType<typeof useCurrentSelectionDetailSave>["workerSave"];
  workerSaveState: ReturnType<
    typeof useCurrentSelectionDetailSave
  >["workerSaveState"];
  workstationSave: ReturnType<
    typeof useCurrentSelectionDetailSave
  >["workstationSave"];
  workstationSaveState: ReturnType<
    typeof useCurrentSelectionDetailSave
  >["workstationSaveState"];
  workTypeSave: ReturnType<
    typeof useCurrentSelectionDetailSave
  >["workTypeSave"];
};

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: one mount surface wires all five editable entity save toasts.
export function CurrentSelectionWidgetSaveNotifications({
  docSave,
  docSaveState,
  editableConfigurationState,
  editableDocConfigurationState,
  editableResourceConfigurationState,
  editableWorkStateConfigurationState,
  editableWorkerConfigurationState,
  editableWorkTypeConfigurationState,
  locale,
  resourceSave,
  resourceSaveState,
  selectedDocTargetPath,
  selectedNode,
  selectedResourceName,
  selectedWorkerName,
  selectedWorkTypeName,
  selection,
  workStatePlaceId,
  workStateSave,
  workerSave,
  workerSaveState,
  workstationSave,
  workstationSaveState,
  workTypeSave,
}: CurrentSelectionWidgetSaveNotificationsProps) {
  const graphDraftHasPendingChanges = useFactoryGraphTopologyEditorBridge(
    (state) => state.graphDraftHasPendingChanges,
  );

  const workerDisplayName = resolveEditableDisplayName({
    draftName:
      editableWorkerConfigurationState?.status === "ready"
        ? editableWorkerConfigurationState.draft.name
        : "",
    editableState: editableWorkerConfigurationState,
    fallbackName: selectedWorkerName ?? "",
  });
  const resourceDisplayName = resolveEditableDisplayName({
    draftName:
      editableResourceConfigurationState?.status === "ready"
        ? editableResourceConfigurationState.draft.name
        : "",
    editableState: editableResourceConfigurationState,
    fallbackName: selectedResourceName ?? "",
  });
  const workTypeDisplayName = resolveEditableDisplayName({
    draftName:
      editableWorkTypeConfigurationState?.status === "ready"
        ? editableWorkTypeConfigurationState.draft.name
        : "",
    editableState: editableWorkTypeConfigurationState,
    fallbackName: selectedWorkTypeName ?? "",
  });
  const workStateDisplayName =
    editableWorkStateConfigurationState?.status === "ready"
      ? editableWorkStateConfigurationState.draft.name.trim() ||
        editableWorkStateConfigurationState.originalStateName
      : (workStatePlaceId ?? "");
  const workstationDisplayName = resolveEditableDisplayName({
    draftName:
      editableConfigurationState?.status === "ready"
        ? editableConfigurationState.draft.name
        : "",
    editableState: editableConfigurationState,
    fallbackName: selectedNode?.workstation_name ?? "",
  });
  const docDisplayName =
    editableDocConfigurationState?.status === "ready"
      ? factoryBundledDocDisplayLabel(
          editableDocConfigurationState.pendingTargetPath,
        )
      : factoryBundledDocDisplayLabel(selectedDocTargetPath ?? "");

  return (
    <>
      {selectedNode ? (
        <>
          <CurrentSelectionSaveNotifications
            documentSave={workstationSaveState}
            entityKind="workstation"
            hasDraftChanges={hasEditableDraftChanges(
              editableConfigurationState,
            )}
            locale={locale}
            messages={buildCurrentSelectionSaveToastMessages({
              entityDisplayName: workstationDisplayName,
              entityKind: "workstation",
              locale,
            })}
            saveAttemptRevision={workstationSave.saveAttemptRevision}
            saveMutationError={workstationSave.saveMutationError}
          />
          <CurrentSelectionGraphDraftConflictNotifications
            documentSave={workstationSaveState}
            graphDraftHasPendingChanges={graphDraftHasPendingChanges}
            isTopologyAffectingSave={
              workstationSave.lastSuccessfulSaveWasTopologyAffecting
            }
            locale={locale}
            saveAttemptRevision={workstationSave.saveAttemptRevision}
          />
        </>
      ) : null}
      {selection?.kind === "doc" && selectedDocTargetPath ? (
        <CurrentSelectionSaveNotifications
          documentSave={docSaveState}
          entityKind="doc"
          hasDraftChanges={hasEditableDraftChanges(
            editableDocConfigurationState,
          )}
          locale={locale}
          messages={buildCurrentSelectionSaveToastMessages({
            entityDisplayName: docDisplayName,
            entityKind: "doc",
            locale,
          })}
          saveAttemptRevision={docSave.saveAttemptRevision}
          saveMutationError={docSave.saveMutationError}
        />
      ) : null}
      {selection?.kind === "worker" && selectedWorkerName ? (
        <>
          <CurrentSelectionSaveNotifications
            documentSave={workerSaveState}
            entityKind="worker"
            hasDraftChanges={hasEditableDraftChanges(
              editableWorkerConfigurationState,
            )}
            locale={locale}
            messages={buildCurrentSelectionSaveToastMessages({
              entityDisplayName: workerDisplayName,
              entityKind: "worker",
              locale,
            })}
            saveAttemptRevision={workerSave.saveAttemptRevision}
            saveMutationError={workerSave.saveMutationError}
          />
          <CurrentSelectionGraphDraftConflictNotifications
            documentSave={workerSaveState}
            graphDraftHasPendingChanges={graphDraftHasPendingChanges}
            isTopologyAffectingSave={
              workerSave.lastSuccessfulSaveWasTopologyAffecting
            }
            locale={locale}
            saveAttemptRevision={workerSave.saveAttemptRevision}
          />
        </>
      ) : null}
      {selection?.kind === "resource" && selectedResourceName ? (
        <>
          <CurrentSelectionSaveNotifications
            documentSave={resourceSaveState}
            entityKind="resource"
            hasDraftChanges={hasEditableDraftChanges(
              editableResourceConfigurationState,
            )}
            locale={locale}
            messages={buildCurrentSelectionSaveToastMessages({
              entityDisplayName: resourceDisplayName,
              entityKind: "resource",
              locale,
            })}
            saveAttemptRevision={resourceSave.saveAttemptRevision}
            saveMutationError={resourceSave.saveMutationError}
          />
          <CurrentSelectionGraphDraftConflictNotifications
            documentSave={resourceSaveState}
            graphDraftHasPendingChanges={graphDraftHasPendingChanges}
            isTopologyAffectingSave={
              resourceSave.lastSuccessfulSaveWasTopologyAffecting
            }
            locale={locale}
            saveAttemptRevision={resourceSave.saveAttemptRevision}
          />
        </>
      ) : null}
      {selection?.kind === "work-type" && selectedWorkTypeName ? (
        <>
          <CurrentSelectionSaveNotifications
            documentSave={workTypeSave.saveState}
            entityKind="work-type"
            hasDraftChanges={hasEditableDraftChanges(
              editableWorkTypeConfigurationState,
            )}
            locale={locale}
            messages={buildCurrentSelectionSaveToastMessages({
              entityDisplayName: workTypeDisplayName,
              entityKind: "work-type",
              locale,
            })}
            saveAttemptRevision={workTypeSave.saveAttemptRevision}
            saveMutationError={workTypeSave.saveMutationError}
          />
          <CurrentSelectionGraphDraftConflictNotifications
            documentSave={workTypeSave.saveState}
            graphDraftHasPendingChanges={graphDraftHasPendingChanges}
            isTopologyAffectingSave={
              workTypeSave.lastSuccessfulSaveWasTopologyAffecting
            }
            locale={locale}
            saveAttemptRevision={workTypeSave.saveAttemptRevision}
          />
        </>
      ) : null}
      {selection?.kind === "state-node" && workStatePlaceId ? (
        <>
          <CurrentSelectionSaveNotifications
            documentSave={workStateSave.saveState}
            entityKind="work-state"
            hasDraftChanges={hasEditableDraftChanges(
              editableWorkStateConfigurationState,
            )}
            locale={locale}
            messages={buildCurrentSelectionSaveToastMessages({
              entityDisplayName: workStateDisplayName,
              entityKind: "work-state",
              locale,
            })}
            saveAttemptRevision={workStateSave.saveAttemptRevision}
            saveMutationError={workStateSave.saveMutationError}
          />
          <CurrentSelectionGraphDraftConflictNotifications
            documentSave={workStateSave.saveState}
            graphDraftHasPendingChanges={graphDraftHasPendingChanges}
            isTopologyAffectingSave={
              workStateSave.lastSuccessfulSaveWasTopologyAffecting
            }
            locale={locale}
            saveAttemptRevision={workStateSave.saveAttemptRevision}
          />
        </>
      ) : null}
    </>
  );
}
