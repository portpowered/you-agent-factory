import { WidgetDetailCopy } from "@you-agent-factory/components/recipes";
import type { ReactNode } from "react";
import { DashboardWidgetFrame } from "../../bento/components/dashboard-widget-frame/dashboard-widget-frame";
import type { LoadableProviderSessionRef } from "../lib/provider-session-ref";
import { getProviderSessionWidgetMessages } from "../messages/provider-session-widget";
import { ProviderSessionDetailPanel } from "./provider-session-detail-panel";

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
        <WidgetDetailCopy>{messages.emptyState}</WidgetDetailCopy>
      )}
    </DashboardWidgetFrame>
  );
}
