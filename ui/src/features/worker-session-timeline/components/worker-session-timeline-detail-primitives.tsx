import { Label, Text } from "@you-agent-factory/components/primitives";
import { type ReactNode, useState } from "react";

import { DashboardActionButton } from "../../../components/ui/dashboard-action-button";
import type { WorkerTimelineJSONValue } from "../lib/worker-session-timeline-projection-types";

export const MAX_DISPLAY_TEXT_LENGTH = 4_000;

export function NavigableDetail({
  actionLabel,
  label,
  onNavigate,
  value,
}: {
  actionLabel: string;
  label: string;
  onNavigate?: (workerSessionID: string) => void;
  value: string;
}) {
  return (
    <div className="grid min-w-0 gap-1">
      <Label>{label}</Label>
      {onNavigate ? (
        <DashboardActionButton
          aria-label={actionLabel}
          className="w-fit max-w-full [overflow-wrap:anywhere]"
          onClick={() => onNavigate(value)}
          type="button"
        >
          {value}
        </DashboardActionButton>
      ) : (
        <Text className="m-0 min-w-0 [overflow-wrap:anywhere]">{value}</Text>
      )}
    </div>
  );
}

export function DetailSection({
  children,
  heading,
}: {
  children: ReactNode;
  heading: string;
}) {
  return (
    <section
      aria-label={heading}
      className="grid min-w-0 gap-2 border-t border-outline pt-3"
    >
      <Label>{heading}</Label>
      {children}
    </section>
  );
}

export function DetailList({
  items,
}: {
  items: Array<{ label: string; value: string } | null>;
}) {
  const visibleItems = items.filter(
    (item): item is { label: string; value: string } => item !== null,
  );
  if (visibleItems.length === 0) {
    return null;
  }
  return (
    <dl className="grid min-w-0 gap-2 sm:grid-cols-2">
      {visibleItems.map((item) => (
        <div className="grid min-w-0 gap-1" key={`${item.label}-${item.value}`}>
          <Label as="dt">{item.label}</Label>
          <Text as="dd" className="m-0 min-w-0 [overflow-wrap:anywhere]">
            {item.value}
          </Text>
        </div>
      ))}
    </dl>
  );
}

export function DetailValue({
  label,
  value,
}: {
  label: string;
  value: string;
}) {
  return (
    <div className="grid min-w-0 gap-1">
      <Label>{label}</Label>
      <Text className="m-0 min-w-0 [overflow-wrap:anywhere]">{value}</Text>
    </div>
  );
}

export function BoundedText({
  collapseLabel,
  expandLabel,
  label,
  value,
}: {
  collapseLabel?: string;
  expandLabel?: string;
  label: string;
  value: string;
}) {
  return (
    <BoundedValue
      collapseLabel={collapseLabel}
      expandLabel={expandLabel}
      label={label}
      long={value.length > MAX_DISPLAY_TEXT_LENGTH}
    >
      <Text className="m-0 whitespace-pre-wrap">{boundedText(value)}</Text>
    </BoundedValue>
  );
}

export function BoundedCode({
  collapseLabel,
  expandLabel,
  label,
  value,
}: {
  collapseLabel?: string;
  expandLabel?: string;
  label: string;
  value: WorkerTimelineJSONValue;
}) {
  const formattedValue = formatJSONValue(value);
  return (
    <BoundedValue
      collapseLabel={collapseLabel}
      expandLabel={expandLabel}
      label={label}
      long={formattedValue.length > MAX_DISPLAY_TEXT_LENGTH}
    >
      <pre className="af-body-code m-0 max-h-64 min-w-0 max-w-full overflow-auto whitespace-pre-wrap break-words">
        <code>{boundedText(formattedValue)}</code>
      </pre>
    </BoundedValue>
  );
}

function BoundedValue({
  children,
  collapseLabel,
  expandLabel,
  label,
  long,
}: {
  children: ReactNode;
  collapseLabel?: string;
  expandLabel?: string;
  label: string;
  long: boolean;
}) {
  const [expanded, setExpanded] = useState(false);
  const visibleExpandLabel = expandLabel ?? label;
  const visibleCollapseLabel = collapseLabel ?? label;
  const content = (
    <div className="max-h-64 min-w-0 max-w-full overflow-auto rounded-md border border-outline bg-surface-container-high p-3 [overflow-wrap:anywhere]">
      {children}
    </div>
  );

  return (
    <div className="grid min-w-0 gap-1">
      <Label>{label}</Label>
      {long ? (
        <details
          className="min-w-0 max-w-full rounded-md border border-outline bg-surface-container-high p-2"
          data-worker-session-timeline-bounded-content="true"
          onToggle={(event) => setExpanded(event.currentTarget.open)}
          open={expanded}
        >
          <summary className="cursor-pointer break-words px-1 py-1 text-sm font-semibold text-on-surface focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-af-accent">
            {expanded ? visibleCollapseLabel : visibleExpandLabel}
          </summary>
          {content}
        </details>
      ) : (
        content
      )}
    </div>
  );
}

export function boundedText(value: string): string {
  if (value.length <= MAX_DISPLAY_TEXT_LENGTH) {
    return value;
  }
  return `${value.slice(0, MAX_DISPLAY_TEXT_LENGTH)}…`;
}

function formatJSONValue(value: WorkerTimelineJSONValue): string {
  if (typeof value === "string") {
    return value;
  }
  try {
    return JSON.stringify(value, null, 2) ?? "";
  } catch {
    return "";
  }
}
