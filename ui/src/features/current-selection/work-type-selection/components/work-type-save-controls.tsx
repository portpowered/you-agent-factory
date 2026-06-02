import { Save } from "lucide-react";
import { Button, DashboardActionButton } from "../../../../components/ui";
import { cn } from "../../../../lib/cn";
import { DashboardMutationDialog } from "../../../workflow-activity/components/mutation-dialog";
import { EditableConfigurationDiscardHeaderAction } from "../../base/public";
import type { EditableWorkTypeSaveState } from "../lib/detail-card-types";
import { getWorkTypeDetailMessages } from "../messages/work-type-detail";

export function EditableWorkTypeConfigurationHeaderActions({
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
  saveState: EditableWorkTypeSaveState;
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
      <EditableWorkTypeSaveHeaderAction
        canSave={canSave}
        locale={locale}
        onClick={onSave}
        saveState={saveState}
      />
    </>
  );
}

export function EditableWorkTypeSaveHeaderAction({
  canSave,
  locale,
  onClick,
  saveState,
}: {
  canSave: boolean;
  locale?: string;
  onClick: () => void;
  saveState: EditableWorkTypeSaveState;
}) {
  const messages = getWorkTypeDetailMessages(locale);
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
              "hover:border-af-warning-border hover:bg-af-warning-surface hover:text-af-warning-text",
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

export function EditableWorkTypeSaveDialog({
  locale,
  onCancel,
  onConfirm,
  saveState,
}: {
  locale?: string;
  onCancel: () => void;
  onConfirm: () => void;
  saveState: EditableWorkTypeSaveState;
}) {
  const messages = getWorkTypeDetailMessages(locale);

  if (saveState.status !== "confirming" && saveState.status !== "submitting") {
    return null;
  }

  return (
    <DashboardMutationDialog
      closeDisabled={saveState.status === "submitting"}
      description={messages.editableConfigurationSaveConfirmationDescription}
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
