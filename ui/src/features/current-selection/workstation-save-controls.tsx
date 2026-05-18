import { DashboardMutationDialog } from "../../components/dashboard";
import { Button } from "../../components/ui";
import type {
  EditableWorkstationOverwriteField,
  EditableWorkstationSaveState,
} from "./detail-card-types";
import { formatEditableOverwriteFieldLabels } from "./editable-workstation-overwrite-fields";
import { getWorkstationDetailMessages } from "./messages";

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

  return (
    <Button
      aria-expanded={
        saveState.status === "confirming" || saveState.status === "submitting"
      }
      aria-haspopup="dialog"
      disabled={!canSave}
      onClick={onClick}
      size="sm"
      type="button"
    >
      {saveState.status === "submitting"
        ? messages.editableConfigurationSaveBusyAction
        : messages.editableConfigurationSaveAction}
    </Button>
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
          <Button onClick={onConfirm} tone="destructive" type="button">
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
