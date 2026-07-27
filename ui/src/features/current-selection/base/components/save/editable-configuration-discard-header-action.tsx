import { RotateCcw } from "lucide-react";

import { DashboardActionButton } from "../../../../../components/ui/dashboard-action-button";
import { getEditableConfigurationControlsMessages } from "../../messages/operational/editable-configuration-controls";

export function EditableConfigurationDiscardHeaderAction({
  ariaLabel,
  canDiscard,
  isSaving,
  locale,
  onClick,
}: {
  ariaLabel?: string;
  canDiscard: boolean;
  isSaving: boolean;
  locale?: string;
  onClick: () => void;
}) {
  const messages = getEditableConfigurationControlsMessages(locale);

  return (
    <DashboardActionButton
      aria-label={ariaLabel ?? messages.discardLocalChangesAction}
      disabled={!canDiscard || isSaving}
      iconOnly
      onClick={onClick}
      type="button"
    >
      <RotateCcw aria-hidden="true" className="size-4" />
    </DashboardActionButton>
  );
}
