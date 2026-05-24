import { Button, PopoverContent } from "../../../components/ui";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SECTION_HEADING_CLASS,
  DASHBOARD_SUPPORTING_LABELS_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../../components/ui/dashboard-typography";
import { cn } from "../../../lib/cn";
import { getInlineWidgetPickerMessages, getInlineWidgetPickerOptions } from "../messages/inline-widget-picker";

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
  "grid gap-1 rounded-2xl border border-af-overlay/10 bg-af-overlay/4 p-3";
const PICKER_ITEM_TITLE_CLASS = cn("m-0 text-sm font-semibold text-af-ink");
const PICKER_ITEM_DESCRIPTION_CLASS = cn(
  "m-0 text-sm text-af-ink/72",
  DASHBOARD_SUPPORTING_TEXT_CLASS,
);
const PICKER_HINT_CLASS = cn(
  "m-0 rounded-2xl border border-af-overlay/10 bg-af-overlay/4 p-3 text-sm text-af-ink/68",
  DASHBOARD_SUPPORTING_TEXT_CLASS,
);
const PICKER_CLOSE_ROW_CLASS = "flex justify-end";

export interface InlineWidgetPickerProps {
  locale?: string;
  onDismiss: () => void;
}

export function InlineWidgetPicker({
  locale,
  onDismiss,
}: InlineWidgetPickerProps) {
  const messages = getInlineWidgetPickerMessages(locale);
  const options = getInlineWidgetPickerOptions(locale);

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
            <p className={PICKER_ITEM_TITLE_CLASS}>{option.title}</p>
            <p className={PICKER_ITEM_DESCRIPTION_CLASS}>{option.description}</p>
          </li>
        ))}
      </ul>

      <p className={PICKER_HINT_CLASS}>{messages.phaseHint}</p>

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
