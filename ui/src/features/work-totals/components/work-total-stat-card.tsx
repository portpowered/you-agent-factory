import { AlertPanel, type AlertPanelProps } from "../../../components/ui/alert-panel";
import { formatNumber } from "../../../i18n";

export interface WorkTotalStatCardProps {
  label: string;
  locale?: string;
  tone: "neutral" | "live" | "success" | "danger";
  value: number;
  valueLabel: string;
}

export function WorkTotalStatCard({
  label,
  locale,
  value,
  valueLabel,
  tone,
}: WorkTotalStatCardProps) {
  return (
    <AlertPanel
      aria-label={valueLabel}
      className="min-h-0 gap-1"
      padding="compact"
      radius="lg"
      tone={workTotalStatAlertTone(tone)}
    >
      <article>
        <span className="block text-[0.68rem] leading-tight uppercase text-on-surface-subtle [overflow-wrap:anywhere]">
          {label}
        </span>
        <strong className="font-display text-[1.2rem] leading-none">
          {formatNumber(value, locale)}
        </strong>
      </article>
    </AlertPanel>
  );
}

function workTotalStatAlertTone(
  tone: WorkTotalStatCardProps["tone"],
): AlertPanelProps["tone"] {
  switch (tone) {
    case "danger":
      return "danger";
    case "live":
      return "info";
    case "success":
      return "success";
    case "neutral":
      return "info";
  }
}
