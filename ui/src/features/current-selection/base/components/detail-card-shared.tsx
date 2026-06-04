import type { DashboardPlaceRef } from "../../../../api/dashboard/types";
import {
  DASHBOARD_BODY_CODE_CLASS,
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../../../components/ui/dashboard-typography";
import { cn } from "../../../../lib/cn";
import type { CurrentSelectionDetailMessages } from "../messages/current-selection-detail";

/** Outer shell for expanded current-selection configuration and history panels. */
export const CURRENT_SELECTION_EXPANDABLE_SECTION_BODY_CLASS =
  "grid gap-3 rounded-2xl border border-outline bg-surface-container-high p-3";
/** Field stack inside an expandable section body (no per-field outline). */
export const CURRENT_SELECTION_FORM_FIELD_CLASS = "grid gap-2";
/** Standalone nested panel when a field group is not inside an expandable section body. */
export const CURRENT_SELECTION_FIELD_PANEL_CLASS =
  "grid gap-2 rounded-2xl border border-outline bg-surface-container-high p-3";
/** Editable configuration field groups must use this stack instead of multi-column grid wrappers. */
export const CURRENT_SELECTION_VERTICAL_FORM_FIELDS_CLASS =
  "grid grid-cols-1 gap-3";
export const CURRENT_SELECTION_NOTICE_SUBTLE_CLASS = cn(
  "m-0 text-on-surface-variant",
  DASHBOARD_SUPPORTING_TEXT_CLASS,
);
export const CURRENT_SELECTION_ALERT_PANEL_CLASS =
  "grid gap-2 rounded-xl border border-af-danger-border bg-error-container p-3";
export const CURRENT_SELECTION_WARNING_PANEL_CLASS =
  "grid gap-2 rounded-2xl border border-af-warning-border bg-warning-container p-3";
export const CURRENT_SELECTION_CODE_SUBTLE_CLASS = cn(
  "text-xs text-on-surface-variant",
  DASHBOARD_BODY_CODE_CLASS,
);
export const HISTORY_HEADER_CLASS =
  "flex items-center justify-between gap-3 rounded-lg border border-outline bg-surface-container-high px-3 py-2 [&_h4]:m-0";
export const CURRENT_SELECTION_ACCENT_SURFACE_CLASS =
  "border-primary text-on-surface";
export const WORKSTATION_SUMMARY_ITEM_CLASS =
  "grid min-w-0 gap-1 rounded-lg border border-outline bg-surface-container-high px-3 py-2";
export const INFERENCE_ATTEMPT_CARD_CLASS =
  "grid min-w-0 gap-2.5 rounded-lg border border-outline p-3.5";
export const INFERENCE_ATTEMPT_DETAIL_CLASS = cn(
  "m-0 grid gap-1.5 [&_dd]:m-0 [&_div]:grid [&_div]:min-w-0 [&_div]:grid-cols-[8.5rem_minmax(0,1fr)] [&_div]:gap-2",
  DASHBOARD_BODY_TEXT_CLASS,
);
// tailwind-exception: intrinsic-sizing
export const INFERENCE_ATTEMPT_TEXT_CLASS = cn(
  "min-h-[20rem] md:min-h-[26rem] lg:min-h-[min(70vh,36rem)]",
);
export const RUNTIME_DETAILS_SECTION_CLASS =
  "mt-4 grid gap-3 border-t border-outline pt-4 [&_h4]:m-0";
export const RUNTIME_DETAIL_VALUE_CLASS = "min-w-0 [overflow-wrap:anywhere]";
export const RUNTIME_DETAIL_CODE_CLASS = cn(
  DASHBOARD_BODY_CODE_CLASS,
  "[overflow-wrap:anywhere]",
);
export const TRACE_ACTION_LINK_CLASS =
  "inline-flex w-fit rounded-lg px-3 py-2 text-sm font-bold text-on-surface outline-af-accent transition hover:border-primary hover:bg-primary-container focus-visible:outline-2 focus-visible:outline-offset-2";
export const REQUEST_SELECTION_STATUS_CLASS = cn(
  "m-0 text-on-surface-subtle",
  DASHBOARD_SUPPORTING_TEXT_CLASS,
);
export const REQUEST_HISTORY_TEXT_CLASS = cn(
  "m-0 whitespace-pre-wrap rounded-lg border border-outline bg-surface-container-high p-2 [overflow-wrap:anywhere]",
  DASHBOARD_BODY_CODE_CLASS,
);

export function isTerminalOrFailedPlace(place: DashboardPlaceRef): boolean {
  return (
    place.state_category === "TERMINAL" || place.state_category === "FAILED"
  );
}

export function emptyStatePlaceMessage(
  messages: Pick<
    CurrentSelectionDetailMessages,
    | "noCurrentWorkInPlace"
    | "noWorkRecordedAtSelectedTick"
    | "selectedTickWorkUnavailable"
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

export function normalizeDetailText(
  value: string | undefined,
): string | undefined {
  const trimmed = value?.trim();
  return trimmed ? trimmed : undefined;
}
