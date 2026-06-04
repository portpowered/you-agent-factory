import type { ReactNode } from "react";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SUPPORTING_LABEL_CLASS,
} from "../../../components/ui/dashboard-typography";
import { getLocalDateTimeDisplay } from "../../../components/ui/formatters";
import { cn } from "../../../lib/cn";

export function DetailMetric({
  label,
  value,
}: {
  label: string;
  value: number | string | ReactNode;
}) {
  const wrapperClassName = cn("mt-1", DASHBOARD_BODY_TEXT_CLASS);

  return (
    <div className="grid gap-1 py-1.5">
      <span className={DASHBOARD_SUPPORTING_LABEL_CLASS}>{label}</span>
      {typeof value === "string" || typeof value === "number" ? (
        <p className={cn("m-0 [overflow-wrap:anywhere]", wrapperClassName)}>
          {value}
        </p>
      ) : (
        <div className={cn("[overflow-wrap:anywhere]", wrapperClassName)}>
          {value}
        </div>
      )}
    </div>
  );
}

export function TimestampMetricValue({
  locale,
  timestamp,
  unavailableLabel,
}: {
  locale?: string;
  timestamp?: string | null;
  unavailableLabel: string;
}) {
  const timestampDisplay = getLocalDateTimeDisplay(
    timestamp,
    unavailableLabel,
    locale,
  );

  if (!timestampDisplay.rawTimestamp) {
    return timestampDisplay.label;
  }

  return (
    <span className="grid gap-1">
      <span title={timestampDisplay.rawTimestamp}>
        {timestampDisplay.label}
      </span>
    </span>
  );
}
