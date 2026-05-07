import type { TerminalWorkDetail } from "../current-selection";
import {
  CompletedFailedWorkstationCard,
  type TerminalWorkItem,
  type TerminalWorkStatus,
} from "./terminal-work-card";

export interface TerminalWorkWidgetProps {
  completedItems: TerminalWorkItem[];
  failedItems: TerminalWorkItem[];
  locale?: string;
  onSelectItem: (status: TerminalWorkStatus, item: TerminalWorkItem) => void;
  selectedItem: TerminalWorkDetail | null;
  widgetId?: string;
}

export function TerminalWorkWidget({
  completedItems,
  failedItems,
  locale,
  onSelectItem,
  selectedItem,
  widgetId = "terminal-work",
}: TerminalWorkWidgetProps) {
  return (
    <CompletedFailedWorkstationCard
      completedItems={completedItems}
      failedItems={failedItems}
      locale={locale}
      selectedItem={selectedItem}
      widgetId={widgetId}
      onSelectItem={onSelectItem}
    />
  );
}
