import { ActionRow, Heading } from "../../../components/ui";
import { DashboardPanelShell } from "../../../components/ui/dashboard-shell";
import { cn } from "../../../lib/cn";
import type { DashboardSessionTabsState } from "../hooks/use-dashboard-session-tabs-state";
import { getHeaderControlsMessages } from "../messages/header-controls";
import { DashboardBrandLockup } from "./dashboard-brand-lockup";
import { DashboardHeaderColorPaletteControls } from "./dashboard-header-color-palette-controls";
import { DashboardHeaderSessionControls } from "./dashboard-header-session-controls";
import { DashboardSessionTabs } from "./dashboard-session-tabs";
import { TickSliderControl } from "./tick-slider-control";

interface DashboardGeneralHeaderProps {
  isExportDialogOpen: boolean;
  locale: string;
  onChangeLocale: (locale: string) => void;
  onOpenExportDialog: () => void;
  sessionTabsState: DashboardSessionTabsState;
}

export function DashboardGeneralHeader({
  isExportDialogOpen,
  locale,
  onChangeLocale,
  onOpenExportDialog,
  sessionTabsState,
}: DashboardGeneralHeaderProps) {
  const headerMessages = getHeaderControlsMessages(locale);

  return (
    <DashboardPanelShell
      aria-label={headerMessages.dashboardSummaryLabel}
      className="mb-3 grid gap-2"
    >
      <div className="grid min-w-0 gap-2 md:grid-cols-[auto_minmax(0,1fr)_auto] md:items-stretch">
        <div
          data-dashboard-header-top-region
          className={cn(
            "flex min-w-0 items-center justify-between gap-2 px-2",
            "md:contents",
          )}
        >
          <Heading
            className="m-0 min-w-0 shrink-0 md:col-start-1 md:self-end"
            level="page"
          >
            <DashboardBrandLockup
              locale={locale}
              wordmarkClassName="truncate"
            />
          </Heading>
          <ActionRow
            actions={
              <DashboardHeaderColorPaletteControls
                locale={locale}
                onChangeLocale={onChangeLocale}
              />
            }
            actionsClassName="justify-end"
            className="shrink-0 justify-end md:col-start-3 md:self-end"
          />
        </div>
        <div
          data-dashboard-header-tab-region
          className="min-w-0 md:col-start-2 md:row-start-1"
        >
          <DashboardSessionTabs locale={locale} state={sessionTabsState} />
        </div>
        <div data-dashboard-header-control-region className="flex min-w-0">
          <div className="relative flex min-w-0 w-full items-center gap-1.5 rounded-sm rounded-t-2xl bg-surface-container-low px-2 pb-2 pt-1">
            <TickSliderControl locale={locale} />
            <DashboardHeaderSessionControls
              isExportDialogOpen={isExportDialogOpen}
              locale={locale}
              onOpenExportDialog={onOpenExportDialog}
              sessionTabsState={sessionTabsState}
            />
          </div>
        </div>
      </div>
    </DashboardPanelShell>
  );
}
