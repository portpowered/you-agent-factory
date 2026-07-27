import type { ReactNode } from "react";

import { AgentBentoCard } from "../../bento/components/agent-bento";
import { getWorkTotalsMessages } from "../messages/work-totals";
import { WorkTotalStatCard } from "./work-total-stat-card";

interface WorkTotalsCardProps {
  completedCount: number;
  dispatchedCount: number;
  failedCount: number;
  headerAction?: ReactNode;
  inFlightDispatchCount: number;
  locale?: string;
}

const WORK_TOTALS_BODY_CLASS =
  "!block !h-auto !min-h-0 !gap-1.5 !pb-2.5 !pt-2 [&>*]:pb-0";

export function WorkTotalsCard({
  completedCount,
  dispatchedCount,
  failedCount,
  headerAction,
  inFlightDispatchCount,
  locale,
}: WorkTotalsCardProps) {
  const messages = getWorkTotalsMessages(locale);

  return (
    <AgentBentoCard
      bodyClassName={WORK_TOTALS_BODY_CLASS}
      bodyScroll={false}
      chromeDensity="compact"
      headerAction={headerAction}
      title={messages.cardTitle}
    >
      <section
        className="grid grid-cols-4 gap-2"
        aria-label={messages.regionLabel}
      >
        <WorkTotalStatCard
          label={messages.statLabels.inFlight}
          locale={locale}
          tone="live"
          value={inFlightDispatchCount}
          valueLabel={messages.statValueLabel(
            messages.statLabels.inFlight,
            inFlightDispatchCount,
          )}
        />
        <WorkTotalStatCard
          label={messages.statLabels.completed}
          locale={locale}
          tone="success"
          value={completedCount}
          valueLabel={messages.statValueLabel(
            messages.statLabels.completed,
            completedCount,
          )}
        />
        <WorkTotalStatCard
          label={messages.statLabels.failed}
          locale={locale}
          tone="danger"
          value={failedCount}
          valueLabel={messages.statValueLabel(
            messages.statLabels.failed,
            failedCount,
          )}
        />
        <WorkTotalStatCard
          label={messages.statLabels.dispatched}
          locale={locale}
          tone="neutral"
          value={dispatchedCount}
          valueLabel={messages.statValueLabel(
            messages.statLabels.dispatched,
            dispatchedCount,
          )}
        />
      </section>
    </AgentBentoCard>
  );
}
