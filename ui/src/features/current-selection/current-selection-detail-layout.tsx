import { DETAIL_CARD_WIDE_CLASS } from "../../components/dashboard/widget-board";
import { DashboardWidgetFrame } from "../../components/ui";
import { DASHBOARD_SUPPORTING_TEXT_CLASS } from "../../components/ui/dashboard-typography";
import { cx } from "../../lib/cx";
import { useCurrentSelectionShellMessages } from "./current-selection-locale";
import type { SelectionDetailLayoutProps } from "./detail-card-types";
import { useSelectionHistoryStore } from "./state/selectionHistoryStore";

const SELECTION_HISTORY_ACTIONS_CLASS = "flex items-center gap-2";
const SELECTION_HISTORY_BUTTON_CLASS = cx(
  "inline-flex h-9 items-center justify-center rounded-lg border border-af-overlay/12 bg-af-overlay/6 px-3 text-af-ink/78 transition hover:border-af-overlay/18 hover:bg-af-overlay/10 hover:text-af-ink focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-af-accent disabled:cursor-not-allowed disabled:border-af-overlay/8 disabled:bg-af-overlay/4 disabled:text-af-ink/35",
  DASHBOARD_SUPPORTING_TEXT_CLASS,
);

export function SelectionDetailLayout({
  widgetId = "current-selection",
  children,
  headerAction,
}: SelectionDetailLayoutProps) {
  const messages = useCurrentSelectionShellMessages();
  const canRedo = useSelectionHistoryStore((state) => state.future.length > 0);
  const canUndo = useSelectionHistoryStore((state) => state.past.length > 0);
  const redoSelection = useSelectionHistoryStore((state) => state.redo);
  const undoSelection = useSelectionHistoryStore((state) => state.undo);

  return (
    <DashboardWidgetFrame
      className={DETAIL_CARD_WIDE_CLASS}
      headerAction={
        <div className={SELECTION_HISTORY_ACTIONS_CLASS}>
          {headerAction}
          <button
            aria-label={messages.undoActionLabel}
            className={SELECTION_HISTORY_BUTTON_CLASS}
            disabled={!canUndo}
            onClick={() => undoSelection()}
            type="button"
          >
            {messages.undoAction}
          </button>
          <button
            aria-label={messages.redoActionLabel}
            className={SELECTION_HISTORY_BUTTON_CLASS}
            disabled={!canRedo}
            onClick={() => redoSelection()}
            type="button"
          >
            {messages.redoAction}
          </button>
        </div>
      }
      title={messages.title}
      widgetId={widgetId}
    >
      {children}
    </DashboardWidgetFrame>
  );
}
