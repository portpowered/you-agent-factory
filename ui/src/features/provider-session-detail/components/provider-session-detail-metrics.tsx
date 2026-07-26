import type { ReactNode } from "react";
import { Label, Text } from "@you-agent-factory/components/primitives";
import { getLocalDateTimeDisplay } from "../../../components/ui/formatters";

export function DetailMetric({
  label,
  value,
}: {
  label: string;
  value: number | string | ReactNode;
}) {
  return (
    <div className="grid gap-1 py-1.5">
      <Label>{label}</Label>
      {typeof value === "string" || typeof value === "number" ? (
        <Text className="m-0 mt-1 [overflow-wrap:anywhere]">{value}</Text>
      ) : (
        <Text as="div" className="mt-1 [overflow-wrap:anywhere]">
          {value}
        </Text>
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
