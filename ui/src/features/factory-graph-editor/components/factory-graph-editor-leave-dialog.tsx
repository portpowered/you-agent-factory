import { DashboardMutationDialog } from "../../../components/dashboard";
import { Button } from "../../../components/ui";
import { getFactoryGraphEditorMessages } from "../messages/editor";

export function FactoryGraphEditorLeaveDialog({
  canSave,
  isOpen,
  isSaving,
  locale,
  onCancel,
  onDiscard,
  onSave,
}: {
  canSave: boolean;
  isOpen: boolean;
  isSaving: boolean;
  locale?: string;
  onCancel: () => void;
  onDiscard: () => void;
  onSave: () => void;
}) {
  if (!isOpen) {
    return null;
  }
  const messages = getFactoryGraphEditorMessages(locale);

  return (
    <DashboardMutationDialog
      closeDisabled={isSaving}
      description={messages.leaveDialogDescription}
      onClose={onCancel}
      title={messages.leaveDialogTitle}
      footer={
        <>
          <Button
            disabled={isSaving}
            onClick={onCancel}
            tone="outline"
            type="button"
          >
            {messages.leaveDialogKeepEditing}
          </Button>
          <Button
            disabled={isSaving}
            onClick={onDiscard}
            tone="ghost"
            type="button"
          >
            {messages.draftActionsDiscard}
          </Button>
          <Button
            disabled={!canSave || isSaving}
            onClick={onSave}
            type="button"
          >
            {isSaving ? messages.draftActionsSaving : messages.draftActionsSave}
          </Button>
        </>
      }
    >
      <p className="m-0 text-sm text-af-text-muted">{messages.leaveDialogBody}</p>
    </DashboardMutationDialog>
  );
}
