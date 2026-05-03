import {
  CompletedFailedWorkstationCard,
  type TerminalWorkItem,
  type TerminalWorkStatus,
} from "./terminal-work-card";
import type { TerminalWorkDetail } from "../current-selection";

export interface TerminalWorkWidgetProps {
  completedItems: TerminalWorkItem[];
  failedItems: TerminalWorkItem[];
  onSelectItem: (status: TerminalWorkStatus, item: TerminalWorkItem) => void;
  selectedItem: TerminalWorkDetail | null;
  widgetId?: string;
}

export function TerminalWorkWidget({
  completedItems,
  failedItems,
  onSelectItem,
  selectedItem,
  widgetId = "terminal-work",
}: TerminalWorkWidgetProps) {
  return (
    <CompletedFailedWorkstationCard
      completedItems={completedItems}
      failedItems={failedItems}
      selectedItem={selectedItem}
      widgetId={widgetId}
      onSelectItem={onSelectItem}
    />
  );
}

