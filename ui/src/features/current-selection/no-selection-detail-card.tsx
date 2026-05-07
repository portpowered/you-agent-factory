import { DETAIL_COPY_CLASS } from "../../components/dashboard/widget-board";
import { useCurrentSelectionShellMessages } from "./current-selection-locale";
import { SelectionDetailLayout } from "./current-selection-detail-layout";
import type { NoSelectionDetailCardProps } from "./detail-card-types";

export function NoSelectionDetailCard({
  widgetId = "current-selection",
}: NoSelectionDetailCardProps) {
  const messages = useCurrentSelectionShellMessages();

  return (
    <SelectionDetailLayout widgetId={widgetId}>
      <p className={DETAIL_COPY_CLASS}>{messages.emptyStateGuidance}</p>
    </SelectionDetailLayout>
  );
}
