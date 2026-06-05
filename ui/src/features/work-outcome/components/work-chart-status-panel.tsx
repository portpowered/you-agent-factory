import { Skeleton } from "../../../components/ui/skeleton";
import {
  DashboardEmptyState,
  DashboardEmptyStateText,
  DashboardEmptyStateTitle,
} from "../../../components/ui/widget-frame";
import { cn } from "../../../lib/cn";
import type { WorkChartPresentation } from "./work-chart-presentation";

// tailwind-exception: intrinsic-sizing
const WORK_CHART_STATUS_PANEL_CLASS =
  "flex h-full min-h-[14rem] min-w-0 w-full flex-1 flex-col justify-center";
// tailwind-exception: intrinsic-sizing
const WORK_CHART_EMBEDDED_STATUS_PANEL_CLASS =
  "grid min-h-[14rem] min-w-0 w-full flex-1 flex-col justify-center items-start gap-1.5 p-0 [&_h3]:m-0";

export interface WorkChartStatusPanelProps {
  ariaBusy?: boolean;
  loading?: boolean;
  message: string;
  presentation: WorkChartPresentation;
  role: "alert" | "status";
  title: string;
}

export function WorkChartStatusPanel({
  ariaBusy = false,
  loading = false,
  message,
  presentation,
  role,
  title,
}: WorkChartStatusPanelProps) {
  const embedded = presentation === "embedded";
  const content = (
    <>
      {loading ? (
        <div aria-hidden="true" className="grid w-full gap-3">
          <Skeleton className="h-4 w-32" />
          <Skeleton className="h-28 w-full" />
        </div>
      ) : null}
      <DashboardEmptyStateTitle>{title}</DashboardEmptyStateTitle>
      <DashboardEmptyStateText>{message}</DashboardEmptyStateText>
    </>
  );

  if (!embedded) {
    return (
      <DashboardEmptyState
        aria-busy={ariaBusy || undefined}
        aria-live={role === "alert" ? "assertive" : "polite"}
        className={WORK_CHART_STATUS_PANEL_CLASS}
        compact
        data-work-chart-presentation={presentation}
        role={role}
      >
        {content}
      </DashboardEmptyState>
    );
  }

  return (
    <div
      aria-busy={ariaBusy || undefined}
      aria-live={role === "alert" ? "assertive" : "polite"}
      className={cn(
        WORK_CHART_EMBEDDED_STATUS_PANEL_CLASS,
        WORK_CHART_STATUS_PANEL_CLASS,
      )}
      data-work-chart-presentation={presentation}
      role={role}
    >
      {content}
    </div>
  );
}
