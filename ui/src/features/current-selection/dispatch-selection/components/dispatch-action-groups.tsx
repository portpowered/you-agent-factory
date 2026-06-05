import { formatWorkItemLabel } from "../../../../components/ui/formatters";
import { cn } from "../../../../lib/cn";
import { CurrentSelectionSelectableButton } from "../../base/components/current-selection-selectable-button";
import {
  CurrentSelectionLabel,
  CurrentSelectionTraceButton,
} from "../../base/public";
import type { dedupeWorkItems } from "../dispatch-history/selected-work-dispatch-history-helpers";

export function WorkItemActionGroup({
  items,
  label,
  onSelectWorkID,
  selectedWorkID,
  selectWorkItemAccessibleLabel,
}: {
  items: ReturnType<typeof dedupeWorkItems>;
  label: string;
  onSelectWorkID?: (workID: string) => void;
  selectedWorkID: string;
  selectWorkItemAccessibleLabel: (workItemLabel: string) => string;
}) {
  if (items.length === 0) {
    return null;
  }

  return (
    <div className="grid gap-1">
      <CurrentSelectionLabel>{label}</CurrentSelectionLabel>
      <div className="flex flex-wrap gap-2">
        {items.map((workItem) => (
          <CurrentSelectionSelectableButton
            aria-label={selectWorkItemAccessibleLabel(
              formatWorkItemLabel(workItem),
            )}
            className={cn(
              selectedWorkID === workItem.work_id &&
                "border-primary bg-primary-container text-on-surface",
            )}
            key={`${label}-${workItem.work_id}`}
            onClick={() => onSelectWorkID?.(workItem.work_id)}
            selected={selectedWorkID === workItem.work_id}
          >
            {formatWorkItemLabel(workItem)}
          </CurrentSelectionSelectableButton>
        ))}
      </div>
    </div>
  );
}

export function TraceActionGroup({
  activeTraceID,
  label,
  onSelectTraceID,
  selectedTraceSuffix,
  traceIDs,
}: {
  activeTraceID?: string | null;
  label: string;
  onSelectTraceID?: (traceID: string) => void;
  selectedTraceSuffix: string;
  traceIDs: string[];
}) {
  if (traceIDs.length === 0) {
    return null;
  }

  return (
    <div className="grid gap-1">
      <CurrentSelectionLabel>{label}</CurrentSelectionLabel>
      <div className="flex flex-wrap gap-2">
        {traceIDs.map((traceID) => (
          <CurrentSelectionTraceButton
            activeTraceID={activeTraceID}
            key={traceID}
            onSelectTraceID={onSelectTraceID}
            selectedTraceSuffix={selectedTraceSuffix}
            traceID={traceID}
          >
            {traceID}
          </CurrentSelectionTraceButton>
        ))}
      </div>
    </div>
  );
}
