import type { ReactElement } from "react";

import { Popover, PopoverTrigger } from "../../../components/ui";
import { DashboardPanelShell } from "../../../components/ui/dashboard-shell";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SECTION_HEADING_CLASS,
  DASHBOARD_SUPPORTING_LABELS_CLASS,
} from "../../../components/ui/dashboard-typography";
import { cn } from "../../../lib/cn";
import { AgentBentoDragHandle } from "./agent-bento";
import { InlineWidgetPicker } from "./inline-widget-picker";
import type { DashboardWidgetPickerAvailability } from "../lib/dashboard-widget-picker";
import { getInlineAddWidgetMessages } from "../messages/inline-add-widget";

const INLINE_ADD_WIDGET_CARD_CLASS =
  "grid h-full min-h-0 min-w-0 place-items-stretch overflow-hidden";
const INLINE_ADD_WIDGET_SURFACE_CLASS =
  "grid h-full min-h-0 gap-2 rounded-2xl border border-dashed border-af-border-strong bg-linear-to-br from-af-surface-subtle via-af-surface-raised to-af-overlay p-3 text-left";
const INLINE_ADD_WIDGET_HEADER_CLASS =
  "flex flex-wrap items-start justify-between gap-2";
const INLINE_ADD_WIDGET_HEADER_COPY_CLASS = "grid min-w-0 flex-1 content-start gap-2";
const INLINE_ADD_WIDGET_DRAG_HANDLE_WRAP_CLASS = "shrink-0";
const INLINE_ADD_WIDGET_ACTION_CLASS =
  "grid min-h-0 min-w-0 flex-1 content-start gap-2 rounded-2xl p-2 text-left outline-none transition-colors hover:bg-af-overlay focus-visible:bg-af-overlay focus-visible:ring-2 focus-visible:ring-af-focus-ring";
const INLINE_ADD_WIDGET_COPY_CLASS = "grid min-w-0 content-start gap-1.5";
const INLINE_ADD_WIDGET_BADGE_CLASS = cn(
  "inline-flex w-fit items-center rounded-full border border-af-border bg-af-surface-subtle px-2 py-1 text-xs font-medium uppercase text-af-text-subtle",
  DASHBOARD_SUPPORTING_LABELS_CLASS,
);
const INLINE_ADD_WIDGET_TITLE_CLASS = cn(
  "m-0",
  DASHBOARD_SECTION_HEADING_CLASS,
);
const INLINE_ADD_WIDGET_BODY_CLASS = cn(
  "m-0",
  DASHBOARD_BODY_TEXT_CLASS,
);
const INLINE_ADD_WIDGET_HINT_CLASS = cn(
  "m-0 text-af-text-muted",
  DASHBOARD_SUPPORTING_LABELS_CLASS,
);
const INLINE_ADD_WIDGET_ACTION_ROW_CLASS =
  "flex flex-wrap items-center gap-2";
const INLINE_ADD_WIDGET_ACTION_LABEL_CLASS = cn(
  "inline-flex items-center rounded-full bg-af-accent px-3 py-1.5 text-sm font-semibold text-af-accent-foreground",
  DASHBOARD_SUPPORTING_LABELS_CLASS,
);
const INLINE_ADD_WIDGET_ICON_CLASS =
  "grid size-9 place-items-center rounded-xl border border-af-border bg-af-surface text-af-text-muted shadow-sm";

export interface InlineAddWidgetCardProps {
  pickerAvailability?: DashboardWidgetPickerAvailability[];
  onPickerOpenChange?: (open: boolean) => void;
  onSelectWidget?: (
    widgetType: DashboardWidgetPickerAvailability["widgetType"],
  ) => void;
  pickerOpen?: boolean;
  locale?: string;
}

export function InlineAddWidgetCard({
  locale,
  onSelectWidget,
  onPickerOpenChange,
  pickerAvailability = [],
  pickerOpen = false,
}: InlineAddWidgetCardProps): ReactElement {
  const messages = getInlineAddWidgetMessages(locale);
  const hasEnabledWidgets = pickerAvailability.some((item) => item.enabled);
  const statusMessage = pickerOpen
    ? messages.pickerOpenState
    : hasEnabledWidgets
      ? messages.readyState
      : messages.unavailableState;
  const supportingHint = hasEnabledWidgets ? messages.body : messages.unavailableHint;
  const actionLabel = hasEnabledWidgets
    ? messages.actionLabel
    : messages.actionUnavailableLabel;

  return (
    <Popover onOpenChange={onPickerOpenChange} open={pickerOpen}>
      <DashboardPanelShell
        aria-label={messages.title}
        as="article"
        className={INLINE_ADD_WIDGET_CARD_CLASS}
        shellKind="grid-card"
      >
        <div className={INLINE_ADD_WIDGET_SURFACE_CLASS}>
          <div className={INLINE_ADD_WIDGET_HEADER_CLASS}>
            <div className={INLINE_ADD_WIDGET_HEADER_COPY_CLASS}>
              <span className={INLINE_ADD_WIDGET_BADGE_CLASS}>{messages.badge}</span>
              <PopoverTrigger asChild>
                <button
                  aria-label={messages.title}
                  aria-expanded={pickerOpen}
                  aria-haspopup="dialog"
                  className={INLINE_ADD_WIDGET_ACTION_CLASS}
                  disabled={!hasEnabledWidgets}
                  type="button"
                >
                  <div className={INLINE_ADD_WIDGET_COPY_CLASS}>
                    <span aria-hidden="true" className={INLINE_ADD_WIDGET_ICON_CLASS}>
                      <svg
                        aria-hidden="true"
                        fill="none"
                        height="22"
                        viewBox="0 0 22 22"
                        width="22"
                        xmlns="http://www.w3.org/2000/svg"
                      >
                        <title>{messages.iconTitle}</title>
                        <path
                          d="M11 4.125v13.75M4.125 11h13.75"
                          stroke="currentColor"
                          strokeLinecap="round"
                          strokeLinejoin="round"
                          strokeWidth="1.9"
                        />
                      </svg>
                    </span>
                    <h3 className={INLINE_ADD_WIDGET_TITLE_CLASS}>{messages.title}</h3>
                    <p className={INLINE_ADD_WIDGET_BODY_CLASS}>{statusMessage}</p>
                    <p className={INLINE_ADD_WIDGET_HINT_CLASS}>{supportingHint}</p>
                    <div className={INLINE_ADD_WIDGET_ACTION_ROW_CLASS}>
                      <span className={INLINE_ADD_WIDGET_ACTION_LABEL_CLASS}>
                        {actionLabel}
                      </span>
                    </div>
                  </div>
                </button>
              </PopoverTrigger>
            </div>
            <div className={INLINE_ADD_WIDGET_DRAG_HANDLE_WRAP_CLASS}>
              <AgentBentoDragHandle title={messages.title} />
            </div>
          </div>
        </div>
      </DashboardPanelShell>
      {pickerOpen ? (
        <InlineWidgetPicker
          availability={pickerAvailability}
          locale={locale}
          onDismiss={() => {
            onPickerOpenChange?.(false);
          }}
          onSelectWidget={(widgetType) => {
            onSelectWidget?.(widgetType);
          }}
        />
      ) : null}
    </Popover>
  );
}
