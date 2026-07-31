import { ActionRow } from "@you-agent-factory/components/layout";
import { Heading } from "@you-agent-factory/components/primitives";
import { DashboardPanelShell } from "../../../components/ui/dashboard-shell";
import { cn } from "../../../lib/cn";
import type { DashboardSessionTabsState } from "../hooks/use-dashboard-session-tabs-state";
import { getHeaderControlsMessages } from "../messages/header-controls";
import { DashboardBrandLockup } from "./dashboard-brand-lockup";
import { DashboardHeaderColorPaletteControls } from "./dashboard-header-color-palette-controls";
import { DashboardSessionTabs } from "./dashboard-session-tabs";

interface DashboardGeneralHeaderProps {
  locale: string;
  onChangeLocale: (locale: string) => void;
  sessionTabsState: DashboardSessionTabsState;
}

export function DashboardGeneralHeader({
  locale,
  onChangeLocale,
  sessionTabsState,
}: DashboardGeneralHeaderProps) {
  const headerMessages = getHeaderControlsMessages(locale);

  return (
    <DashboardPanelShell
      aria-label={headerMessages.dashboardSummaryLabel}
      className="sticky top-1 z-20 mb-3 min-w-0 px-2 py-2 md:top-2"
    >
      <div className="grid min-w-0 gap-2 md:grid-cols-[auto_minmax(0,1fr)_auto] md:items-center">
        <div
          data-dashboard-header-top-region
          className={cn(
            "flex min-w-0 items-center justify-between gap-2",
            "md:contents",
          )}
        >
          <Heading
            className="m-0 min-w-0 shrink-0 md:col-start-1 md:self-center"
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
            className="shrink-0 justify-end md:col-start-3 md:self-center"
          />
        </div>
        <div
          data-dashboard-header-tab-region
          className="min-w-0 md:col-start-2 md:row-start-1"
        >
          <DashboardSessionTabs locale={locale} state={sessionTabsState} />
        </div>
      </div>
    </DashboardPanelShell>
  );
}
