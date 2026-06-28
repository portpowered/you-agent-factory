import type { ReactNode } from "react";

import { DetailCopy } from "../../../components/ui/widget-frame";
import { DashboardWidgetFrame } from "../../bento/public";
import { readFactorySessionIDSearchParam } from "../lib/search-param/factory-session-search-param";
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
  const selectedSessionID =
    typeof window === "undefined"
      ? sessionID
      : readFactorySessionIDSearchParam(window.location.search) ?? sessionID;

  return (
    <DashboardWidgetFrame
      headerAction={headerAction}
      title={messages.title}
      widgetId={widgetId}
    >
      {selectedSessionID ? (
        <FactorySessionDetailPanel
          locale={locale}
          sessionID={selectedSessionID}
        />
      ) : (
        <DetailCopy>{messages.emptyState}</DetailCopy>
      )}
    </DashboardWidgetFrame>
  );
}
