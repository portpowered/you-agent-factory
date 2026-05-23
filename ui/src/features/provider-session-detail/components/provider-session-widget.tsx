import { DashboardWidgetFrame, DETAIL_COPY_CLASS } from "../../../components/ui";
import type { LoadableProviderSessionRef } from "../lib/provider-session-ref";
import { ProviderSessionDetailPanel } from "./provider-session-detail-panel";
import { getProviderSessionWidgetMessages } from "../messages/provider-session-widget";

export interface ProviderSessionWidgetProps {
  locale?: string;
  selectedProviderSession: LoadableProviderSessionRef | null;
  widgetId?: string;
}

export function ProviderSessionWidget({
  locale,
  selectedProviderSession,
  widgetId = "provider-session",
}: ProviderSessionWidgetProps) {
  const messages = getProviderSessionWidgetMessages(locale);

  return (
    <DashboardWidgetFrame title={messages.title} widgetId={widgetId}>
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
