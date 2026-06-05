import type { ReactNode } from "react";
import { Button } from "../../../../components/ui";
import { CodePanel } from "../../../../components/ui/code-panel";
import { formatWorkItemLabel } from "../../../../components/ui/formatters";
import { DetailCopy } from "../../../../components/ui/widget-frame";
import { cn } from "../../../../lib/cn";
import { CurrentSelectionSelectableButton } from "../../base/components/current-selection-selectable-button";
import {
  CurrentSelectionDescriptionList,
  CurrentSelectionDetailCode,
  CurrentSelectionDetailValue,
  CurrentSelectionLabel,
} from "../../base/public";
import type { dedupeWorkItems } from "../dispatch-history/selected-work-dispatch-history-helpers";

export function ScriptArgsSection({
  args,
  label,
}: {
  args: string[] | undefined;
  label: string;
}) {
  if (!args || args.length === 0) {
    return null;
  }

  return (
    <div className="grid gap-1">
      <CurrentSelectionLabel>{label}</CurrentSelectionLabel>
      <div className="grid gap-1">
        {args.map((arg) => (
          <CurrentSelectionDetailCode key={arg}>
            {arg}
          </CurrentSelectionDetailCode>
        ))}
      </div>
    </div>
  );
}

export function ScriptOutputSection({
  emptyMessage,
  label,
  value,
}: {
  emptyMessage: string;
  label: string;
  value: string | undefined;
}) {
  return (
    <div className="grid gap-1">
      <CurrentSelectionLabel>{label}</CurrentSelectionLabel>
      {value ? (
        <CodePanel>{value}</CodePanel>
      ) : (
        <DetailCopy>{emptyMessage}</DetailCopy>
      )}
    </div>
  );
}

export function DispatchDetailSection({
  children,
  title,
}: {
  children: ReactNode;
  title: string;
}) {
  return (
    <section
      aria-label={title}
      className="mt-3 grid gap-2 border-t border-outline pt-3"
    >
      <CurrentSelectionLabel>{title}</CurrentSelectionLabel>
      {children}
    </section>
  );
}

export function DispatchDetailList({
  entries,
}: {
  entries: Array<{
    code?: boolean;
    href?: string;
    label: string;
    title?: string;
    value?: string;
  }>;
}) {
  const populatedEntries = entries.filter((entry) => entry.value);
  if (populatedEntries.length === 0) {
    return null;
  }

  return (
    <CurrentSelectionDescriptionList>
      {populatedEntries.map((entry) => (
        <InferenceAttemptDetailLink
          code={entry.code}
          href={entry.href}
          key={entry.label}
          label={entry.label}
          title={entry.title}
          value={entry.value}
        />
      ))}
    </CurrentSelectionDescriptionList>
  );
}

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
  traceTargetId,
}: {
  activeTraceID?: string | null;
  label: string;
  onSelectTraceID?: (traceID: string) => void;
  selectedTraceSuffix: string;
  traceIDs: string[];
  traceTargetId: string;
}) {
  if (traceIDs.length === 0) {
    return null;
  }

  return (
    <div className="grid gap-1">
      <CurrentSelectionLabel>{label}</CurrentSelectionLabel>
      <div className="flex flex-wrap gap-2">
        {traceIDs.map((traceID) => (
          <Button
            asChild
            className="w-fit rounded-lg"
            key={traceID}
            size="sm"
            tone="outline"
          >
            <a
              href={`#${traceTargetId}`}
              onClick={() => onSelectTraceID?.(traceID)}
            >
              {traceID}
              {activeTraceID === traceID ? selectedTraceSuffix : ""}
            </a>
          </Button>
        ))}
      </div>
    </div>
  );
}

function InferenceAttemptDetailLink({
  code = false,
  href,
  label,
  title,
  value,
}: {
  code?: boolean;
  href?: string;
  label: string;
  title?: string;
  value?: string;
}) {
  if (!value) {
    return null;
  }

  return (
    <div>
      <dt>{label}</dt>
      <CurrentSelectionDetailValue>
        {href ? (
          <Button asChild className="w-fit rounded-lg" size="sm" tone="outline">
            <a href={href} title={title}>
              {value}
            </a>
          </Button>
        ) : code ? (
          <CurrentSelectionDetailCode>{value}</CurrentSelectionDetailCode>
        ) : (
          value
        )}
      </CurrentSelectionDetailValue>
    </div>
  );
}
