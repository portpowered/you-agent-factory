import { resolveSupportedLocale } from "../../../i18n";
import {
  CompletedFailedWorkstationCard,
} from "./terminal-work-card";
import type {
  TerminalWorkDetail,
  TerminalWorkItem,
  TerminalWorkStatus,
} from "../lib/types";

export interface TerminalWorkWidgetProps {
  completedItems: TerminalWorkItem[];
  failedItems: TerminalWorkItem[];
  locale?: string;
  onSelectItem: (status: TerminalWorkStatus, item: TerminalWorkItem) => void;
  selectedItem: TerminalWorkDetail | null;
  widgetId?: string;
}

export function TerminalWorkWidget({
  completedItems,
  failedItems,
  locale,
  onSelectItem,
  selectedItem,
  widgetId = "terminal-work",
}: TerminalWorkWidgetProps) {
  const resolvedLocale = resolveTerminalWorkLocale(locale);

  return (
    <CompletedFailedWorkstationCard
      completedItems={completedItems}
      failedItems={failedItems}
      locale={resolvedLocale}
      selectedItem={selectedItem}
      widgetId={widgetId}
      onSelectItem={onSelectItem}
    />
  );
}

function resolveTerminalWorkLocale(locale?: string): string | undefined {
  const localeCandidate = locale ?? getBrowserLocaleCandidate();
  if (!localeCandidate) {
    return undefined;
  }

  return resolveSupportedLocale(localeCandidate);
}

function getBrowserLocaleCandidate(): string | undefined {
  if (typeof document !== "undefined" && document.documentElement.lang) {
    return document.documentElement.lang;
  }

  if (typeof navigator !== "undefined") {
    return navigator.languages[0] ?? navigator.language;
  }

  return undefined;
}
