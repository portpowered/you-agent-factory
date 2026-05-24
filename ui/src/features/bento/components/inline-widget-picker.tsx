import { Button, PopoverContent } from "../../../components/ui";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SECTION_HEADING_CLASS,
  DASHBOARD_SUPPORTING_LABELS_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../../components/ui/dashboard-typography";
import { cn } from "../../../lib/cn";
import type { DashboardWidgetPickerAvailability } from "../lib/dashboard-widget-picker";
import { getInlineWidgetPickerMessages, getInlineWidgetPickerOptions, getInlineWidgetPickerStatus } from "../messages/inline-widget-picker";

const PICKER_CONTENT_CLASS =
  "grid w-72 max-w-full gap-4 rounded-3xl border-af-overlay/16 bg-af-surface/98 p-4 sm:w-80";
const PICKER_HEADER_CLASS = "grid gap-2 pr-10";
const PICKER_LABEL_CLASS = cn(
  "text-xs font-semibold uppercase tracking-[0.12em] text-af-accent",
  DASHBOARD_SUPPORTING_LABELS_CLASS,
);
const PICKER_TITLE_CLASS = cn("m-0", DASHBOARD_SECTION_HEADING_CLASS);
const PICKER_DESCRIPTION_CLASS = cn("m-0 text-af-ink/76", DASHBOARD_BODY_TEXT_CLASS);
const PICKER_LIST_CLASS = "m-0 grid list-none gap-2 p-0";
const PICKER_ITEM_CLASS =
  "grid gap-2 rounded-2xl border border-af-overlay/10 bg-af-overlay/4 p-1.5";
const PICKER_ITEM_BUTTON_CLASS =
  "grid gap-2 rounded-2xl p-3 text-left outline-none transition-colors hover:bg-af-overlay/8 focus-visible:ring-2 focus-visible:ring-af-accent/25 disabled:cursor-not-allowed disabled:opacity-72 disabled:hover:bg-transparent";
const PICKER_ITEM_HEADER_CLASS = "flex items-start justify-between gap-3";
const PICKER_ITEM_TITLE_CLASS = cn("m-0 text-sm font-semibold text-af-ink");
const PICKER_ITEM_DESCRIPTION_CLASS = cn(
  "m-0 text-sm text-af-ink/72",
  DASHBOARD_SUPPORTING_TEXT_CLASS,
);
const PICKER_ITEM_BADGE_CLASS = cn(
  "inline-flex shrink-0 rounded-full border border-af-overlay/12 bg-af-base/88 px-2 py-1 text-[11px] font-medium uppercase tracking-[0.08em] text-af-ink/62",
  DASHBOARD_SUPPORTING_LABELS_CLASS,
);
const PICKER_HINT_CLASS = cn(
  "m-0 rounded-2xl border border-af-overlay/10 bg-af-overlay/4 p-3 text-sm text-af-ink/68",
  DASHBOARD_SUPPORTING_TEXT_CLASS,
);
const PICKER_CLOSE_ROW_CLASS = "flex justify-end";

export interface InlineWidgetPickerProps {
  availability: DashboardWidgetPickerAvailability[];
  locale?: string;
  onDismiss: () => void;
  onSelectWidget: (
    widgetType: DashboardWidgetPickerAvailability["widgetType"],
  ) => void;
}

export function InlineWidgetPicker({
  availability,
  locale,
  onDismiss,
  onSelectWidget,
}: InlineWidgetPickerProps) {
  const messages = getInlineWidgetPickerMessages(locale);
  const options = getInlineWidgetPickerOptions(locale);
  const availabilityByType = new Map(
    availability.map((item) => [item.widgetType, item]),
  );

  return (
    <PopoverContent
      align="start"
      aria-label={messages.title}
      className={PICKER_CONTENT_CLASS}
      role="dialog"
    >
      <div className={PICKER_HEADER_CLASS}>
        <p className={PICKER_LABEL_CLASS}>{messages.openAction}</p>
        <div className="grid gap-1.5">
          <h3 className={PICKER_TITLE_CLASS}>{messages.title}</h3>
          <p className={PICKER_DESCRIPTION_CLASS}>{messages.description}</p>
        </div>
      </div>

      <ul className={PICKER_LIST_CLASS}>
        {options.map((option) => (
          <li className={PICKER_ITEM_CLASS} key={option.widgetType}>
            <button
              aria-label={`${messages.openAction}: ${option.title}`}
              className={PICKER_ITEM_BUTTON_CLASS}
              disabled={!availabilityByType.get(option.widgetType)?.enabled}
              onClick={() => {
                onSelectWidget(option.widgetType);
              }}
              type="button"
            >
              <div className={PICKER_ITEM_HEADER_CLASS}>
                <p className={PICKER_ITEM_TITLE_CLASS}>{option.title}</p>
                <span className={PICKER_ITEM_BADGE_CLASS}>
                  {getInlineWidgetPickerStatus(
                    availabilityByType.get(option.widgetType) ?? {
                      duplicateCapable: false,
                      enabled: false,
                      widgetType: option.widgetType,
                    },
                    locale,
                  )}
                </span>
              </div>
              <p className={PICKER_ITEM_DESCRIPTION_CLASS}>{option.description}</p>
            </button>
          </li>
        ))}
      </ul>

      <p className={PICKER_HINT_CLASS}>{messages.selectionHint}</p>

      <div className={PICKER_CLOSE_ROW_CLASS}>
        <Button
          aria-label={messages.closeLabel}
          onClick={onDismiss}
          size="sm"
          tone="outline"
          type="button"
        >
          {messages.dismissAction}
        </Button>
      </div>
    </PopoverContent>
  );
}
