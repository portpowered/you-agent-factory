import { formatNumber } from "../../i18n";
import { cx } from "../../lib/cx";
import { AgentBentoCard } from "../../components/ui";
import { getWorkTotalsMessages } from "./messages/work-totals";

interface WorkTotalsCardProps {
  completedCount: number;
  dispatchedCount: number;
  failedCount: number;
  inFlightDispatchCount: number;
  locale?: string;
}

interface StatCardProps {
  label: string;
  locale?: string;
  tone: "neutral" | "live" | "success" | "danger";
  value: number;
  valueLabel: string;
}

const STAT_CARD_CLASS =
  "min-h-0 rounded-lg border bg-af-surface/72 p-2 px-3 backdrop-blur-lg";

export function WorkTotalsCard({
  completedCount,
  dispatchedCount,
  failedCount,
  inFlightDispatchCount,
  locale,
}: WorkTotalsCardProps) {
  const messages = getWorkTotalsMessages(locale);

  return (
    <AgentBentoCard title={messages.cardTitle}>
      <section
        className="grid grid-cols-2 gap-2 md:grid-cols-4"
        aria-label={messages.regionLabel}
      >
        <StatCard
          label={messages.statLabels.inFlight}
          locale={locale}
          tone="live"
          value={inFlightDispatchCount}
          valueLabel={messages.statValueLabel(
            messages.statLabels.inFlight,
            inFlightDispatchCount,
          )}
        />
        <StatCard
          label={messages.statLabels.completed}
          locale={locale}
          tone="success"
          value={completedCount}
          valueLabel={messages.statValueLabel(
            messages.statLabels.completed,
            completedCount,
          )}
        />
        <StatCard
          label={messages.statLabels.failed}
          locale={locale}
          tone="danger"
          value={failedCount}
          valueLabel={messages.statValueLabel(
            messages.statLabels.failed,
            failedCount,
          )}
        />
        <StatCard
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

function StatCard({ label, locale, value, valueLabel, tone }: StatCardProps) {
  return (
    <article
      aria-label={valueLabel}
      className={cx(
        STAT_CARD_CLASS,
        tone === "neutral" && "border-af-overlay/10",
        tone === "live" && "border-af-info/30",
        tone === "success" && "border-af-success/30",
        tone === "danger" && "border-af-danger/30",
      )}
    >
      <span className="mb-1 block text-[0.68rem] uppercase text-af-ink/64">
        {label}
      </span>
      <strong className="font-display text-[1.35rem] leading-none">
        {formatNumber(value, locale)}
      </strong>
    </article>
  );
}
