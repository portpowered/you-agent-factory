import { getDashboardWorkChartSeriesDefinitions } from "./chart-contract";
import { cn } from "../../lib/cn";
import type { WorkChartModel } from "./trends";
import { WorkChart } from "./work-chart";
import type { WorkChartSeriesDefinition, WorkChartState } from "./work-chart";
import { DASHBOARD_WIDGET_CLASS } from "../../components/dashboard/widget-board";
import { AgentBentoCard } from "../../components/ui";
import { getWorkOutcomeMessages } from "./messages/work-outcome";

export interface WorkChartCardProps {
  chartState?: WorkChartState;
  className?: string;
  locale?: string;
  model: WorkChartModel;
  title?: string;
  widgetId?: string;
}

const WORK_CHART_BODY_CLASS = "!flex !gap-0 !overflow-hidden !p-0";
const WORK_CHART_REGION_CLASS = "min-h-0 flex-1 px-4 sm:px-5";

export function WorkChartCard({
  chartState,
  className = "",
  locale,
  model,
  title,
  widgetId,
}: WorkChartCardProps) {
  const messages = getWorkOutcomeMessages(locale);
  const chartMessages = messages.chart;
  const chartRegionID = widgetId ? `${widgetId}-chart-region` : "work-outcome-chart-region";
  const cardClassName = cn(DASHBOARD_WIDGET_CLASS, className);
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
    <AgentBentoCard
      bodyClassName={WORK_CHART_BODY_CLASS}
      className={cardClassName}
      title={resolvedTitle}
    >
      <section
        aria-label={chartMessages.cardRegionLabel}
        className={WORK_CHART_REGION_CLASS}
        id={chartRegionID}
      >
        <WorkChart
          ariaLabel={chartMessages.ariaLabel(model.rangeLabel)}
          className="h-full"
          locale={locale}
          model={model}
          series={chartSeries}
          state={chartState}
        />
      </section>
    </AgentBentoCard>
  );
}

export const D3CompletionInformationCard = WorkChartCard;
