import { useSelectionHistoryStore } from "../../state/selectionHistoryStore";
import { cx } from "../../components/dashboard/classnames";
import { DASHBOARD_SUPPORTING_TEXT_CLASS } from "../../components/dashboard/typography";
import { DETAIL_CARD_WIDE_CLASS } from "../../components/dashboard/widget-board";
import { DashboardWidgetFrame } from "../../components/ui";
import type { SelectionDetailLayoutProps } from "./detail-card-types";

const CURRENT_SELECTION_TITLE = "Current selection";
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
            aria-label="Undo selection"
            className={SELECTION_HISTORY_BUTTON_CLASS}
            disabled={!canUndo}
            onClick={() => undoSelection()}
            type="button"
          >
            Undo
          </button>
          <button
            aria-label="Redo selection"
            className={SELECTION_HISTORY_BUTTON_CLASS}
            disabled={!canRedo}
            onClick={() => redoSelection()}
            type="button"
          >
            Redo
          </button>
        </div>
      }
      title={CURRENT_SELECTION_TITLE}
      widgetId={widgetId}
    >
      {children}
    </DashboardWidgetFrame>
  );
}
