import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@you-agent-factory/components/overlays";
import { Button } from "@you-agent-factory/components/primitives";
import type { ReactElement } from "react";
import {
  type DashboardWidgetRemovalCandidate,
  type DashboardWidgetUndoState,
  dashboardWidgetRemovalStatusMessage,
} from "../../hooks/removal/useDashboardWidgetRemoval";
import { getDashboardWidgetRemovalMessages } from "../../messages/dashboard-widget-removal";

export interface DashboardWidgetRemovalFeedbackProps {
  locale?: string | null;
  onCancelRemoval: () => void;
  onConfirmRemoval: () => void;
  onDismissUndo: () => void;
  onDialogCloseAutoFocus: (event: Event) => void;
  onDialogOpenChange: (open: boolean) => void;
  onUndoRemoval: () => void;
  pendingRemoval: DashboardWidgetRemovalCandidate | null;
  undoState: DashboardWidgetUndoState | null;
}

export function DashboardWidgetRemovalFeedback({
  locale,
  onCancelRemoval,
  onConfirmRemoval,
  onDismissUndo,
  onDialogCloseAutoFocus,
  onDialogOpenChange,
  onUndoRemoval,
  pendingRemoval,
  undoState,
}: DashboardWidgetRemovalFeedbackProps): ReactElement {
  const messages = getDashboardWidgetRemovalMessages(locale);

  return (
    <>
      {undoState ? (
        <div
          aria-live={
            undoState.status === "failed-to-persist" ||
            undoState.storageWriteFailed
              ? "assertive"
              : "polite"
          }
          className="mb-3 flex flex-wrap items-center gap-2 rounded-lg border border-outline bg-surface-container-low px-3 py-2 text-sm text-on-surface"
          data-testid="dashboard-widget-removal-status"
          role={
            undoState.status === "failed-to-persist" ||
            undoState.storageWriteFailed
              ? "alert"
              : "status"
          }
        >
          <span>{dashboardWidgetRemovalStatusMessage(undoState, locale)}</span>
          {undoState.status === "available" ? (
            <>
              <Button
                aria-label={messages.undoAction(undoState.widgetTitle)}
                onClick={onUndoRemoval}
                size="sm"
                type="button"
              >
                {messages.undoLabel}
              </Button>
              <Button
                onClick={onDismissUndo}
                size="sm"
                tone="outline"
                type="button"
              >
                {messages.dismissAction}
              </Button>
            </>
          ) : null}
        </div>
      ) : null}

      <Dialog onOpenChange={onDialogOpenChange} open={pendingRemoval !== null}>
        {pendingRemoval ? (
          <DialogContent
            closeLabel={messages.closeDialog}
            onCloseAutoFocus={onDialogCloseAutoFocus}
          >
            <DialogHeader>
              <DialogTitle>
                {messages.confirmTitle(pendingRemoval.widgetTitle)}
              </DialogTitle>
              <DialogDescription>
                {messages.confirmDescription(pendingRemoval.widgetTitle)}
              </DialogDescription>
            </DialogHeader>
            <DialogFooter>
              <Button onClick={onCancelRemoval} tone="outline" type="button">
                {messages.cancelAction}
              </Button>
              <Button onClick={onConfirmRemoval} type="button">
                {messages.confirmAction}
              </Button>
            </DialogFooter>
          </DialogContent>
        ) : null}
      </Dialog>
    </>
  );
}
