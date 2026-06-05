import { DashboardActionRow, DashboardHeading } from "../../../components/ui";
import { DashboardPanelShell } from "../../../components/ui/dashboard-shell";
import { cn } from "../../../lib/cn";
import { getHeaderControlsMessages } from "../messages/header-controls";
import type { DashboardSessionTabsState } from "../hooks/use-dashboard-session-tabs-state";
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
      <div className="flex min-w-0 flex-col gap-0">
        <div
          className={cn(
            "flex min-w-0 items-stretch gap-2 px-2",
            "max-md:flex-col",
          )}
        >
          <DashboardHeading
            className="m-0 min-w-0 shrink-0 self-end pb-2"
            level="page"
          >
            <DashboardBrandLockup locale={locale} wordmarkClassName="truncate" />
          </DashboardHeading>
          <div className="flex min-w-0 w-full flex-1">
            <div className="flex h-full min-w-0 w-full items-stretch overflow-x-auto px-4 pt-1">
              <DashboardSessionTabs locale={locale} state={sessionTabsState} />
            </div>
          </div>
          <DashboardActionRow
            actions={
              <DashboardHeaderColorPaletteControls
                locale={locale}
                onChangeLocale={onChangeLocale}
              />
            }
            actionsClassName="max-md:w-full max-md:justify-end"
            className="justify-end max-md:w-full"
          />
        </div>
        <div className="flex min-w-0">
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
