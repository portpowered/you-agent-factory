import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@you-agent-factory/components/overlays";
import { Button } from "../../../../components/ui/button";
import { getFactoryGraphEditorMessages } from "../../messages/editor";

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
  const messages = getFactoryGraphEditorMessages(locale);

  const handleOpenChange = (open: boolean) => {
    if (!open && !isSaving) {
      onCancel();
    }
  };

  return (
    <Dialog onOpenChange={handleOpenChange} open={isOpen}>
      <DialogContent
        closeDisabled={isSaving}
        closeLabel="Close dialog"
        onEscapeKeyDown={(event) => {
          if (isSaving) {
            event.preventDefault();
          }
        }}
        onInteractOutside={(event) => {
          if (isSaving) {
            event.preventDefault();
          }
        }}
      >
        <DialogHeader>
          <DialogTitle>{messages.leaveDialogTitle}</DialogTitle>
          <DialogDescription>
            {messages.leaveDialogDescription}
          </DialogDescription>
        </DialogHeader>
        <p className="m-0 text-sm text-on-surface-variant">
          {messages.leaveDialogBody}
        </p>
        <DialogFooter>
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
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
