import type { ReactNode } from "react";

import { DetailCopy } from "../../../components/ui/widget-frame";
import { DashboardWidgetFrame } from "../../bento/public";
import { getFactorySessionWidgetMessages } from "../messages/factory-session-widget";
import { FactorySessionDetailPanel } from "./factory-session-detail-panel";

export interface FactorySessionWidgetProps {
  headerAction?: ReactNode;
  locale?: string;
  sessionID: string | null;
  widgetId?: string;
}

export function FactorySessionWidget({
  headerAction,
  locale,
  sessionID,
  widgetId = "factory-session",
}: FactorySessionWidgetProps) {
  const messages = getFactorySessionWidgetMessages(locale);

  return (
    <DashboardWidgetFrame
      headerAction={headerAction}
      title={messages.title}
      widgetId={widgetId}
    >
      {sessionID ? (
        <FactorySessionDetailPanel locale={locale} sessionID={sessionID} />
      ) : (
        <DetailCopy>{messages.emptyState}</DetailCopy>
      )}
    </DashboardWidgetFrame>
  );
}
