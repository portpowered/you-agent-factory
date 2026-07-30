import type { ReactNode } from "react";

import { useExportDialogStore } from "../../export/state/exportDialogStore";
import { DashboardSessionControls } from "../../header/components/dashboard-session-controls";
import { TickSliderControl } from "../../header/components/tick-slider-control";
import { useDashboardSessionTabsState } from "../../header/hooks/use-dashboard-session-tabs-state";
import { getHeaderControlsMessages } from "../../header/messages/header-controls";
import { DashboardWidgetFrame } from "./dashboard-widget-frame/dashboard-widget-frame";

export interface SessionControlsWidgetProps {
  headerAction?: ReactNode;
  locale?: string;
  widgetId?: string;
}

export function SessionControlsWidget({
  headerAction,
  locale,
  widgetId = "session-controls",
}: SessionControlsWidgetProps) {
  const sessionTabsState = useDashboardSessionTabsState();
  const isExportDialogOpen = useExportDialogStore(
    (state) => state.isExportDialogOpen,
  );
  const openExportDialog = useExportDialogStore(
    (state) => state.openExportDialog,
  );
  const messages = getHeaderControlsMessages(locale);

  return (
    <DashboardWidgetFrame
      bodyClassName="flex min-w-0 items-center"
      bodyScroll={false}
      headerAction={headerAction}
      title={messages.sessionControlsLabel}
      widgetId={widgetId}
    >
      <div className="flex min-w-0 w-full items-center gap-1.5">
        <TickSliderControl locale={locale} />
        <DashboardSessionControls
          isExportDialogOpen={isExportDialogOpen}
          locale={locale ?? "en"}
          onOpenExportDialog={openExportDialog}
          sessionTabsState={sessionTabsState}
        />
      </div>
    </DashboardWidgetFrame>
  );
}
