import type { ReactNode } from "react";
import { DashboardWidgetFrame } from "../../bento/components/dashboard-widget-frame/dashboard-widget-frame";
import { getDashboardWorkChartSeriesDefinitions } from "../lib/chart-contract";
import type { WorkChartModel } from "../lib/trends";
import { getWorkOutcomeMessages } from "../messages/work-outcome";
import type { WorkChartSeriesDefinition, WorkChartState } from "./work-chart";
import { WorkChart } from "./work-chart";

export interface WorkChartCardProps {
  chartState?: WorkChartState;
  className?: string;
  headerAction?: ReactNode;
  locale?: string;
  model: WorkChartModel;
  title?: string;
  widgetId?: string;
}

const WORK_CHART_BODY_CLASS =
  "!flex !h-full !min-h-0 !flex-1 !flex-col !gap-0 !overflow-hidden px-0 pb-5";

export function WorkChartCard({
  chartState,
  className = "",
  headerAction,
  locale,
  model,
  title,
  widgetId,
}: WorkChartCardProps) {
  const messages = getWorkOutcomeMessages(locale);
  const chartMessages = messages.chart;
  const chartRegionID = widgetId
    ? `${widgetId}-chart-region`
    : "work-outcome-chart-region";
  const resolvedTitle = title ?? chartMessages.cardTitle;
  const chartSeries: readonly WorkChartSeriesDefinition[] =
    getDashboardWorkChartSeriesDefinitions([
      {
        key: "queued",
        label: chartMessages.seriesLabels.queued,
      },
      {
        key: "inFlight",
        label: chartMessages.seriesLabels.inFlight,
      },
      {
        key: "completed",
        label: chartMessages.seriesLabels.completed,
      },
      {
        key: "failed",
        label: chartMessages.seriesLabels.failed,
      },
    ]);

  return (
    <DashboardWidgetFrame
      bodyClassName={WORK_CHART_BODY_CLASS}
      bodyScroll={false}
      className={className}
      headerAction={headerAction}
      title={resolvedTitle}
      widgetId={widgetId ?? "work-outcome-chart"}
    >
      <section
        aria-label={chartMessages.cardRegionLabel}
        className="flex h-full min-h-0 flex-1 px-4 sm:px-5"
        id={chartRegionID}
      >
        <WorkChart
          ariaLabel={chartMessages.ariaLabel(model.rangeLabel)}
          className="h-full"
          locale={locale}
          model={model}
          presentation="embedded"
          series={chartSeries}
          state={chartState}
        />
      </section>
    </DashboardWidgetFrame>
  );
}
