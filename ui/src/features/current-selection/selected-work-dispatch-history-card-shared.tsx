import type { ReactNode } from "react";

import { DETAIL_COPY_CLASS } from "../../components/dashboard/widget-board";
import { DASHBOARD_SUPPORTING_LABEL_CLASS } from "../../components/dashboard/typography";
import { formatWorkItemLabel } from "../../components/ui/formatters";
import {
  INFERENCE_ATTEMPT_DETAIL_CLASS,
  REQUEST_HISTORY_TEXT_CLASS,
  RUNTIME_DETAIL_CODE_CLASS,
  RUNTIME_DETAIL_VALUE_CLASS,
  TRACE_ACTION_LINK_CLASS,
  WORK_SELECTION_BUTTON_CLASS,
} from "./detail-card-shared";
import { dedupeWorkItems } from "./selected-work-dispatch-history-helpers";

export function ScriptArgsSection({ args }: { args: string[] | undefined }) {
  if (!args || args.length === 0) {
    return null;
  }

  return (
    <div className="grid gap-[0.3rem]">
      <span className={DASHBOARD_SUPPORTING_LABEL_CLASS}>Resolved args</span>
      <div className="grid gap-[0.25rem]">
        {args.map((arg) => (
          <code className={RUNTIME_DETAIL_CODE_CLASS} key={arg}>
            {arg}
          </code>
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
    <div className="grid gap-[0.3rem]">
      <span className={DASHBOARD_SUPPORTING_LABEL_CLASS}>{label}</span>
      {value ? (
        <pre className={REQUEST_HISTORY_TEXT_CLASS}>{value}</pre>
      ) : (
        <p className={DETAIL_COPY_CLASS}>{emptyMessage}</p>
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
      className="mt-[0.75rem] grid gap-[0.45rem] border-t border-af-overlay/8 pt-[0.75rem]"
    >
      <span className={DASHBOARD_SUPPORTING_LABEL_CLASS}>{title}</span>
      {children}
    </section>
  );
}

export function DispatchDetailList({
  entries,
}: {
  entries: Array<{ code?: boolean; href?: string; label: string; title?: string; value?: string }>;
}) {
  const populatedEntries = entries.filter((entry) => entry.value);
  if (populatedEntries.length === 0) {
    return null;
  }

  return (
    <dl className={INFERENCE_ATTEMPT_DETAIL_CLASS}>
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
    </dl>
  );
}

export function WorkItemActionGroup({
  items,
  label,
  onSelectWorkID,
  selectedWorkID,
}: {
  items: ReturnType<typeof dedupeWorkItems>;
  label: string;
  onSelectWorkID?: (workID: string) => void;
  selectedWorkID: string;
}) {
  if (items.length === 0) {
    return null;
  }

  return (
    <div className="grid gap-[0.3rem]">
      <span className={DASHBOARD_SUPPORTING_LABEL_CLASS}>{label}</span>
      <div className="flex flex-wrap gap-[0.45rem]">
        {items.map((workItem) => (
          <button
            aria-label={`Select work item ${formatWorkItemLabel(workItem)}`}
            aria-pressed={selectedWorkID === workItem.work_id}
            className={WORK_SELECTION_BUTTON_CLASS}
            key={`${label}-${workItem.work_id}`}
            onClick={() => onSelectWorkID?.(workItem.work_id)}
            type="button"
          >
            {selectedWorkID === workItem.work_id
              ? "Work selected"
              : `Open ${formatWorkItemLabel(workItem)}`}
          </button>
        ))}
      </div>
    </div>
  );
}

export function TraceActionGroup({
  activeTraceID,
  onSelectTraceID,
  traceIDs,
  traceTargetId,
}: {
  activeTraceID?: string | null;
  onSelectTraceID?: (traceID: string) => void;
  traceIDs: string[];
  traceTargetId: string;
}) {
  if (traceIDs.length === 0) {
    return null;
  }

  return (
    <div className="grid gap-[0.3rem]">
      <span className={DASHBOARD_SUPPORTING_LABEL_CLASS}>Trace IDs</span>
      <div className="flex flex-wrap gap-[0.45rem]">
        {traceIDs.map((traceID) => (
          <a
            className={TRACE_ACTION_LINK_CLASS}
            href={`#${traceTargetId}`}
            key={traceID}
            onClick={() => onSelectTraceID?.(traceID)}
          >
            {traceID}
            {activeTraceID === traceID ? " (selected)" : ""}
          </a>
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
      <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
        {href ? (
          <a className={TRACE_ACTION_LINK_CLASS} href={href} title={title}>
            {value}
          </a>
        ) : code ? (
          <code className={RUNTIME_DETAIL_CODE_CLASS}>{value}</code>
        ) : (
          value
        )}
      </dd>
    </div>
  );
}
