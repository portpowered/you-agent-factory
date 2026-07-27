import { Save } from "lucide-react";

import { DashboardActionButton } from "../../../../components/ui/dashboard-action-button";
import { EditableConfigurationDiscardHeaderAction } from "../../base/components/save/editable-configuration-discard-header-action";
import type { EditableDocSaveState } from "../lib/detail-card-types";
import { getDocDetailMessages } from "../messages/doc-detail";

export function EditableDocConfigurationHeaderActions({
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
  saveState: EditableDocSaveState;
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
      <EditableDocSaveHeaderAction
        canSave={canSave}
        locale={locale}
        onClick={onSave}
        saveState={saveState}
      />
    </>
  );
}

export function EditableDocSaveHeaderAction({
  canSave,
  locale,
  onClick,
  saveState,
}: {
  canSave: boolean;
  locale?: string;
  onClick: () => void;
  saveState: EditableDocSaveState;
}) {
  const messages = getDocDetailMessages(locale);
  const emphasizeSave = canSave && saveState.status !== "submitting";

  return (
    <DashboardActionButton
      aria-label={
        saveState.status === "submitting"
          ? messages.editableConfigurationSaveBusyAction
          : messages.editableConfigurationSaveAction
      }
      disabled={!canSave}
      executing={saveState.status === "submitting"}
      iconOnly
      onClick={onClick}
      tone={emphasizeSave ? "warning" : "outline"}
      type="button"
    >
      <Save aria-hidden="true" className="size-4" />
    </DashboardActionButton>
  );
}
