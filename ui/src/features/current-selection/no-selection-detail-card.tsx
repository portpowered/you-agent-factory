import { DETAIL_COPY_CLASS } from "../../components/dashboard/widget-board";
import { SelectionDetailLayout } from "./current-selection-detail-layout";
import type { NoSelectionDetailCardProps } from "./detail-card-types";

export function NoSelectionDetailCard({
  widgetId = "current-selection",
}: NoSelectionDetailCardProps) {
  return (
    <SelectionDetailLayout widgetId={widgetId}>
      <p className={DETAIL_COPY_CLASS}>
        Select a workstation, work item, or state node to inspect live details.
      </p>
    </SelectionDetailLayout>
  );
}

