import type { TerminalWorkDetail } from "../current-selection";
import { isSupportedLocale } from "../../i18n";
import {
  CompletedFailedWorkstationCard,
  type TerminalWorkItem,
  type TerminalWorkStatus,
} from "./terminal-work-card";

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

  const normalizedLocale = localeCandidate.trim().replaceAll("_", "-").toLowerCase();
  if (isSupportedLocale(normalizedLocale)) {
    return normalizedLocale;
  }

  const primaryLanguage = normalizedLocale.split("-")[0];
  return primaryLanguage && isSupportedLocale(primaryLanguage)
    ? primaryLanguage
    : undefined;
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
