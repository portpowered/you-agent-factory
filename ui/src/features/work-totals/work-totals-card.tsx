import { AgentBentoCard } from "../../components/dashboard/bento";
import { cx } from "../../components/dashboard/classnames";

interface WorkTotalsCardProps {
  completedCount: number;
  dispatchedCount: number;
  failedCount: number;
  inFlightDispatchCount: number;
}

interface StatCardProps {
  label: string;
  tone: "neutral" | "live" | "success" | "danger";
  value: number;
}

const STAT_CARD_CLASS =
  "min-h-0 rounded-lg border border-af-overlay/10 bg-af-surface/72 p-2 px-3 backdrop-blur-[18px]";

export function WorkTotalsCard({
  completedCount,
  dispatchedCount,
  failedCount,
  inFlightDispatchCount,
}: WorkTotalsCardProps) {
  return (
    <AgentBentoCard title="Work totals">
      <section
        className="grid grid-cols-4 gap-2 max-[720px]:grid-cols-2"
        aria-label="work totals"
      >
        <StatCard label="In progress" value={inFlightDispatchCount} tone="live" />
        <StatCard label="Completed" value={completedCount} tone="success" />
        <StatCard label="Failed" value={failedCount} tone="danger" />
        <StatCard label="Dispatched" value={dispatchedCount} tone="neutral" />
      </section>
    </AgentBentoCard>
  );
}

function StatCard({ label, value, tone }: StatCardProps) {
  return (
    <article
      className={cx(
        STAT_CARD_CLASS,
        tone === "live" && "border-af-info/30",
        tone === "success" && "border-af-success/30",
        tone === "danger" && "border-af-danger/30",
      )}
    >
      <span className="mb-1 block text-[0.68rem] uppercase text-af-ink/64">{label}</span>
      <strong className="font-display text-[1.35rem] leading-none">{value}</strong>
    </article>
  );
}
