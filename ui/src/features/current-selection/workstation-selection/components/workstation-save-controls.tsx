import { Save } from "lucide-react";
import { Button, DashboardActionButton } from "../../../../components/ui";
import { cn } from "../../../../lib/cn";
import { DashboardMutationDialog } from "../../../workflow-activity/components/mutation-dialog";
import { EditableConfigurationDiscardHeaderAction } from "../../base/public";
import { formatEditableOverwriteFieldLabels } from "../editing/editable-workstation-overwrite-fields";
import type {
  EditableWorkstationOverwriteField,
  EditableWorkstationSaveState,
} from "../lib/detail-card-types";
import { getWorkstationDetailMessages } from "../messages/workstation-detail";

export function EditableWorkstationConfigurationHeaderActions({
  canDiscard,
  canSave,
  locale,
  onDiscard,
  onSave,
  saveState,
}: {
  canDiscard: boolean;
  canSave: boolean;
  locale?: string;
  onDiscard: () => void;
  onSave: () => void;
  saveState: EditableWorkstationSaveState;
}) {
  const isSaving = saveState.status === "submitting";

  return (
    <>
      <EditableConfigurationDiscardHeaderAction
        canDiscard={canDiscard}
        isSaving={isSaving}
        locale={locale}
        onClick={onDiscard}
      />
      <EditableWorkstationSaveHeaderAction
        canSave={canSave}
        locale={locale}
        onClick={onSave}
        saveState={saveState}
      />
    </>
  );
}

export function EditableWorkstationSaveHeaderAction({
  canSave,
  locale,
  onClick,
  saveState,
}: {
  canSave: boolean;
  locale?: string;
  onClick: () => void;
  saveState: EditableWorkstationSaveState;
}) {
  const messages = getWorkstationDetailMessages(locale);
  const emphasizeSave = canSave && saveState.status !== "submitting";

  return (
    <DashboardActionButton
      aria-label={
        saveState.status === "submitting"
          ? messages.editableConfigurationSaveBusyAction
          : messages.editableConfigurationSaveAction
      }
      aria-expanded={
        saveState.status === "confirming" || saveState.status === "submitting"
      }
      aria-haspopup="dialog"
      className={
        emphasizeSave
          ? cn(
              "border-af-warning-border bg-af-warning-surface text-af-warning-text",
              "hover:border-af-warning hover:bg-af-warning hover:text-af-on-warning",
            )
          : undefined
      }
      disabled={!canSave}
      executing={saveState.status === "submitting"}
      iconOnly
      onClick={onClick}
      type="button"
    >
      <Save aria-hidden="true" className="size-4" />
    </DashboardActionButton>
  );
}

export function EditableWorkstationSaveDialog({
  locale,
  onCancel,
  onConfirm,
  overwriteFieldNames,
  saveState,
}: {
  locale?: string;
  onCancel: () => void;
  onConfirm: () => void;
  overwriteFieldNames: EditableWorkstationOverwriteField[];
  saveState: EditableWorkstationSaveState;
}) {
  const messages = getWorkstationDetailMessages(locale);

  if (saveState.status !== "confirming" && saveState.status !== "submitting") {
    return null;
  }

  const description =
    overwriteFieldNames.length > 0
      ? messages.editableConfigurationSaveConflictConfirmationDescription(
          formatEditableOverwriteFieldLabels(overwriteFieldNames, messages),
        )
      : messages.editableConfigurationSaveConfirmationDescription;

  return (
    <DashboardMutationDialog
      closeDisabled={saveState.status === "submitting"}
      description={description}
      onClose={onCancel}
      title={messages.editableConfigurationSaveConfirmationTitle}
      footer={
        <>
          <Button
            disabled={saveState.status === "submitting"}
            onClick={onCancel}
            tone="outline"
            type="button"
          >
            {messages.editableConfigurationSaveConfirmationCancelAction}
          </Button>
          <Button
            aria-busy={saveState.status === "submitting" ? "true" : undefined}
            disabled={saveState.status === "submitting"}
            onClick={onConfirm}
            tone="destructive"
            type="button"
          >
            {saveState.status === "submitting"
              ? messages.editableConfigurationSaveBusyAction
              : messages.editableConfigurationSaveConfirmationConfirmAction}
          </Button>
        </>
      }
    >
      <div />
    </DashboardMutationDialog>
  );
}
