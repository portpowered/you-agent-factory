import type { ReactNode } from "react";
import type { DashboardPlaceRef } from "../../../../api/dashboard/types";
import { cn } from "../../../../lib/cn";
import {
  DASHBOARD_BODY_CODE_CLASS,
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SECTION_HEADING_CLASS,
  DASHBOARD_SUPPORTING_CODE_CLASS,
  DASHBOARD_SUPPORTING_LABEL_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../../../components/ui/dashboard-typography";
import { DETAIL_COPY_CLASS } from "../../../../components/ui/widget-frame";
import {
  AuthoredBodyText,
  REQUEST_AUTHORED_TEXT_CLASS,
  RequestAuthoredText,
} from "../../../../lib/authored-body-text";
import type {
  InferenceAttemptDetailProps,
  InferenceAttemptTextSectionProps,
  MetadataSectionProps,
} from "./detail-card-types";
import type { CurrentSelectionDetailMessages } from "../messages/current-selection-detail";

export const EXECUTION_PILL_CLASS = cn(
  "inline-flex rounded-full border border-af-info-border bg-af-info-surface px-2 py-0.5 text-af-info",
  DASHBOARD_SUPPORTING_CODE_CLASS,
);
export const PROVIDER_SESSION_CARD_CLASS = "rounded-lg border border-af-border bg-af-surface-subtle p-3.5";
export const CURRENT_SELECTION_FIELD_PANEL_CLASS =
  "grid gap-2 rounded-2xl border border-af-border bg-af-surface-subtle p-3";
export const CURRENT_SELECTION_NOTICE_SUBTLE_CLASS = cn(
  "m-0 text-af-text-muted",
  DASHBOARD_SUPPORTING_TEXT_CLASS,
);
export const CURRENT_SELECTION_ALERT_PANEL_CLASS =
  "grid gap-2 rounded-xl border border-af-danger-border bg-af-danger-surface p-3";
export const CURRENT_SELECTION_WARNING_PANEL_CLASS =
  "grid gap-2 rounded-2xl border border-af-warning-border bg-af-warning-surface p-3";
export const CURRENT_SELECTION_CODE_SUBTLE_CLASS = cn(
  "text-xs text-af-text-muted",
  DASHBOARD_BODY_CODE_CLASS,
);
export const HISTORY_HEADER_CLASS =
  "flex items-center justify-between gap-3 rounded-lg border border-af-border bg-af-surface-subtle px-3 py-2 [&_h4]:m-0";
export const CURRENT_SELECTION_ACCENT_SURFACE_CLASS =
  "border-af-accent-border bg-af-accent-surface text-af-text";
export const CURRENT_SELECTION_BADGE_CLASS = cn(
  "inline-flex rounded-full border px-2 py-0.5",
  CURRENT_SELECTION_ACCENT_SURFACE_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
);
export const HISTORY_TOGGLE_CLASS = cn(
  "shrink-0 cursor-pointer rounded-lg border border-af-border bg-af-surface-raised px-2.5 py-2 text-af-text-muted transition hover:border-af-border-strong hover:bg-af-overlay hover:text-af-text focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-af-accent disabled:cursor-not-allowed disabled:border-af-border disabled:bg-af-surface-subtle disabled:text-af-text-disabled",
  DASHBOARD_SUPPORTING_TEXT_CLASS,
);
export const WORKSTATION_SUMMARY_ITEM_CLASS =
  "grid min-w-0 gap-1 rounded-lg border border-af-border bg-af-surface-subtle px-3 py-2";
export const INFERENCE_ATTEMPT_CARD_CLASS =
  "grid min-w-0 gap-2.5 rounded-lg border border-af-border p-3.5";
export const INFERENCE_ATTEMPT_DETAIL_CLASS = cn(
  "m-0 grid gap-1.5 [&_dd]:m-0 [&_div]:grid [&_div]:min-w-0 [&_div]:grid-cols-[8.5rem_minmax(0,1fr)] [&_div]:gap-2",
  DASHBOARD_BODY_TEXT_CLASS,
);
// tailwind-exception: intrinsic-sizing
export const INFERENCE_ATTEMPT_TEXT_CLASS = cn(
  "min-h-[20rem] md:min-h-[26rem] lg:min-h-[min(70vh,36rem)]",
);
export { AuthoredBodyText, REQUEST_AUTHORED_TEXT_CLASS, RequestAuthoredText };
export const RUNTIME_DETAILS_SECTION_CLASS =
  "mt-4 grid gap-3 border-t border-af-border pt-4 [&_h4]:m-0";
export const RUNTIME_DETAIL_VALUE_CLASS = "min-w-0 [overflow-wrap:anywhere]";
export const RUNTIME_DETAIL_CODE_CLASS = cn(
  DASHBOARD_BODY_CODE_CLASS,
  "[overflow-wrap:anywhere]",
);
export const TRACE_ACTION_LINK_CLASS =
  "inline-flex w-fit rounded-lg border border-af-accent-border bg-af-accent-surface px-3 py-2 text-sm font-bold text-af-text outline-af-accent transition hover:border-af-accent hover:bg-af-accent-surface focus-visible:outline-2 focus-visible:outline-offset-2";
export const REQUEST_SELECTION_STATUS_CLASS = cn(
  "m-0 text-af-text-subtle",
  DASHBOARD_SUPPORTING_TEXT_CLASS,
);
export const PROVIDER_SESSION_SELECTION_BUTTON_CLASS = cn(
  "grid w-full gap-1.5 rounded-lg border border-af-border bg-af-surface-raised px-3 py-2.5 text-left text-af-text-muted outline-af-accent transition hover:border-af-border-strong hover:bg-af-overlay hover:text-af-text focus-visible:outline-2 focus-visible:outline-offset-2 disabled:cursor-not-allowed disabled:border-af-border disabled:bg-af-surface-subtle disabled:text-af-text-disabled",
  DASHBOARD_BODY_TEXT_CLASS,
);
export const WORK_SELECTION_BUTTON_CLASS =
  "inline-flex w-fit rounded-lg border border-af-border bg-af-surface-raised px-2.5 py-2 text-xs font-bold text-af-text-muted outline-af-accent transition hover:border-af-border-strong hover:bg-af-overlay hover:text-af-text focus-visible:outline-2 focus-visible:outline-offset-2 disabled:cursor-not-allowed disabled:border-af-border disabled:bg-af-surface-subtle disabled:text-af-text-disabled";
export const REQUEST_HISTORY_TEXT_CLASS = cn(
  "m-0 whitespace-pre-wrap rounded-lg border border-af-border bg-af-surface-raised p-2 [overflow-wrap:anywhere]",
  DASHBOARD_BODY_CODE_CLASS,
);

export function CurrentSelectionSectionHeader({
  action,
  headingId,
  supportingText,
  title,
}: {
  action?: ReactNode;
  headingId: string;
  supportingText?: ReactNode;
  title: string;
}) {
  return (
    <div className={HISTORY_HEADER_CLASS}>
      <div className="grid min-w-0 gap-1">
        <h4 className={DASHBOARD_SECTION_HEADING_CLASS} id={headingId}>
          {title}
        </h4>
        {supportingText ? (
          <p className={cn("m-0 text-af-text-subtle", DASHBOARD_SUPPORTING_TEXT_CLASS)}>
            {supportingText}
          </p>
        ) : null}
      </div>
      {action}
    </div>
  );
}

export function InferenceAttemptTextSection({
  label,
  value,
}: InferenceAttemptTextSectionProps) {
  return (
    <section aria-label={label} className="grid gap-1">
      <span className={DASHBOARD_SUPPORTING_LABEL_CLASS}>{label}</span>
      <AuthoredBodyText className={INFERENCE_ATTEMPT_TEXT_CLASS} value={value} />
    </section>
  );
}

export function InferenceAttemptDetail({
  code = false,
  label,
  rawValue,
  value,
}: InferenceAttemptDetailProps) {
  if (value === undefined || value === "") {
    return null;
  }

  return (
    <div>
      <dt>{label}</dt>
      <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
        {code ? (
          <code className={RUNTIME_DETAIL_CODE_CLASS}>{value}</code>
        ) : rawValue ? (
          <span title={rawValue}>{value}</span>
        ) : (
          value
        )}
      </dd>
    </div>
  );
}

export function MetadataSection({
  emptyMessage,
  metadata,
  title,
}: MetadataSectionProps) {
  const entries = Object.entries(metadata ?? {}).sort(([left], [right]) =>
    left.localeCompare(right),
  );

  return (
    <section aria-label={title} className={RUNTIME_DETAILS_SECTION_CLASS}>
      <h4 className={DASHBOARD_SECTION_HEADING_CLASS}>{title}</h4>
      {entries.length > 0 ? (
        <dl className={INFERENCE_ATTEMPT_DETAIL_CLASS}>
          {entries.map(([key, value]) => (
            <div key={key}>
              <dt>{key}</dt>
              <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
                <code className={RUNTIME_DETAIL_CODE_CLASS}>{value}</code>
              </dd>
            </div>
          ))}
        </dl>
      ) : (
        <p className={DETAIL_COPY_CLASS}>{emptyMessage}</p>
      )}
    </section>
  );
}

export function isTerminalOrFailedPlace(place: DashboardPlaceRef): boolean {
  return place.state_category === "TERMINAL" || place.state_category === "FAILED";
}

export function emptyStatePlaceMessage(
  messages: Pick<
    CurrentSelectionDetailMessages,
    "noCurrentWorkInPlace" | "noWorkRecordedAtSelectedTick" | "selectedTickWorkUnavailable"
  >,
  usesRetainedWorkItems: boolean,
  tokenCount: number,
): string {
  if (!usesRetainedWorkItems) {
    return messages.noCurrentWorkInPlace;
  }

  if (tokenCount > 0) {
    return messages.selectedTickWorkUnavailable;
  }

  return messages.noWorkRecordedAtSelectedTick;
}

export function normalizeDetailText(value: string | undefined): string | undefined {
  const trimmed = value?.trim();
  return trimmed ? trimmed : undefined;
}
