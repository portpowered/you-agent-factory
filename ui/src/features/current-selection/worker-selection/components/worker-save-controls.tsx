import { Save } from "lucide-react";
import { DashboardActionButton } from "../../../../components/ui";
import { cn } from "../../../../lib/cn";
import { EditableConfigurationDiscardHeaderAction } from "../../base/public";
import type { EditableWorkerSaveState } from "../lib/detail-card-types";
import { getWorkerDetailMessages } from "../messages/worker-detail";

export function EditableWorkerConfigurationHeaderActions({
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
  saveState: EditableWorkerSaveState;
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
      <EditableWorkerSaveHeaderAction
        canSave={canSave}
        locale={locale}
        onClick={onSave}
        saveState={saveState}
      />
    </>
  );
}

export function EditableWorkerSaveHeaderAction({
  canSave,
  locale,
  onClick,
  saveState,
}: {
  canSave: boolean;
  locale?: string;
  onClick: () => void;
  saveState: EditableWorkerSaveState;
}) {
  const messages = getWorkerDetailMessages(locale);
  const emphasizeSave = canSave && saveState.status !== "submitting";

  return (
    <DashboardActionButton
      aria-label={
        saveState.status === "submitting"
          ? messages.editableConfigurationSaveBusyAction
          : messages.editableConfigurationSaveAction
      }
      className={
        emphasizeSave
          ? cn(
              "border-af-warning-border bg-warning-container text-on-warning-container",
              "hover:border-af-warning-border hover:bg-warning-container hover:text-on-warning-container",
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
