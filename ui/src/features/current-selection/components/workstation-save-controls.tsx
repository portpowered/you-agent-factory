import { DashboardMutationDialog } from "../../workflow-activity/components/mutation-dialog";
import { Button, DashboardActionButton } from "../../../components/ui";
import { formatEditableOverwriteFieldLabels } from "../editing/editable-workstation-overwrite-fields";
import { getWorkstationDetailMessages } from "../messages/workstation-detail";
import type {
  EditableWorkstationOverwriteField,
  EditableWorkstationSaveState,
} from "./detail-card-types";

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
      disabled={!canSave}
      executing={saveState.status === "submitting"}
      iconOnly
      onClick={onClick}
      type="button"
    >
      <SaveIcon />
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

function SaveIcon() {
  return (
    <svg aria-hidden="true" className="size-4" fill="none" viewBox="0 0 16 16">
      <path
        d="M3 2.75h8.5L13.25 4.5v8.75H3z"
        stroke="currentColor"
        strokeLinejoin="round"
        strokeWidth="1.5"
      />
      <path
        d="M5 2.75v3h5v-3"
        stroke="currentColor"
        strokeLinejoin="round"
        strokeWidth="1.5"
      />
      <path
        d="M5.25 13.25v-4h5.5v4"
        stroke="currentColor"
        strokeLinejoin="round"
        strokeWidth="1.5"
      />
    </svg>
  );
}
