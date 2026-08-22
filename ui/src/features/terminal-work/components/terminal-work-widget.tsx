import type { ReactNode } from "react";

import { resolveSupportedLocale } from "../../../i18n";
import type {
  TerminalWorkDetail,
  TerminalWorkItem,
  TerminalWorkStatus,
} from "../lib/types";
import { CompletedFailedWorkstationCard } from "./terminal-work-card";

export interface TerminalWorkWidgetProps {
  canceledItems?: TerminalWorkItem[];
  completedItems: TerminalWorkItem[];
  failedItems: TerminalWorkItem[];
  headerAction?: ReactNode;
  locale?: string;
  onSelectItem: (status: TerminalWorkStatus, item: TerminalWorkItem) => void;
  selectedItem: TerminalWorkDetail | null;
  terminatedItems?: TerminalWorkItem[];
  unknownItems?: TerminalWorkItem[];
  widgetId?: string;
}

export function TerminalWorkWidget({
  canceledItems = [],
  completedItems,
  failedItems,
  headerAction,
  locale,
  onSelectItem,
  selectedItem,
  terminatedItems = [],
  unknownItems = [],
  widgetId = "terminal-work",
}: TerminalWorkWidgetProps) {
  const resolvedLocale = resolveTerminalWorkLocale(locale);

  return (
    <CompletedFailedWorkstationCard
      canceledItems={canceledItems}
      completedItems={completedItems}
      failedItems={failedItems}
      headerAction={headerAction}
      locale={resolvedLocale}
      selectedItem={selectedItem}
      terminatedItems={terminatedItems}
      unknownItems={unknownItems}
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
