import type { ReactNode } from "react";

import { DashboardWidgetFrame, DETAIL_COPY_CLASS } from "../../../components/ui";
import type { LoadableProviderSessionRef } from "../lib/provider-session-ref";
import { ProviderSessionDetailPanel } from "./provider-session-detail-panel";
import { getProviderSessionWidgetMessages } from "../messages/provider-session-widget";

export interface ProviderSessionWidgetProps {
  headerAction?: ReactNode;
  locale?: string;
  selectedProviderSession: LoadableProviderSessionRef | null;
  widgetId?: string;
}

export function ProviderSessionWidget({
  headerAction,
  locale,
  selectedProviderSession,
  widgetId = "provider-session",
}: ProviderSessionWidgetProps) {
  const messages = getProviderSessionWidgetMessages(locale);

  return (
    <DashboardWidgetFrame
      headerAction={headerAction}
      title={messages.title}
      widgetId={widgetId}
    >
      {selectedProviderSession ? (
        <ProviderSessionDetailPanel
          locale={locale}
          selectedProviderSession={selectedProviderSession}
        />
      ) : (
        <p className={DETAIL_COPY_CLASS}>{messages.emptyState}</p>
      )}
    </DashboardWidgetFrame>
  );
}
