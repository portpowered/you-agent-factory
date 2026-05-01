import { createContext, useContext } from "react";
import type { CSSProperties, ReactNode } from "react";
import {
  Legend as RechartsLegend,
  ResponsiveContainer,
  Tooltip as RechartsTooltip,
  type TooltipContentProps,
} from "recharts";
import type { Props as RechartsLegendContentProps, LegendPayload } from "recharts/types/component/DefaultLegendContent";

import { cn } from "../../lib/cn";

export interface ChartConfigEntry {
  color: string;
  label: string;
}

export type ChartConfig = Record<string, ChartConfigEntry>;

const ChartContext = createContext<ChartConfig | null>(null);

function useChartConfig() {
  const context = useContext(ChartContext);

  if (context === null) {
    throw new Error("Chart components must be rendered inside ChartContainer.");
  }

  return context;
}

export function ChartContainer({
  children,
  className,
  config,
  title,
}: {
  children: ReactNode;
  className?: string;
  config: ChartConfig;
  title: string;
}) {
  return (
    <ChartContext.Provider value={config}>
      <div
        aria-label={title}
        className={cn(
          "h-[18rem] rounded-2xl border border-af-overlay/10 bg-af-surface/56 p-4 text-af-ink",
          className,
        )}
        data-chart-container=""
        role="img"
        style={
          Object.fromEntries(
            Object.entries(config).map(([key, value]) => [`--color-${key}`, value.color]),
          ) as CSSProperties
        }
      >
        <ResponsiveContainer>{children}</ResponsiveContainer>
      </div>
    </ChartContext.Provider>
  );
}

export const ChartTooltip = RechartsTooltip;
export const ChartLegend = RechartsLegend;

export function ChartTooltipContent({
  active,
  className,
  label,
  payload,
}: TooltipContentProps<any, any> & { className?: string }) {
  const config = useChartConfig();

  if (!active || !payload?.length) {
    return null;
  }

  return (
    <div
      className={cn(
        "grid min-w-40 gap-2 rounded-xl border border-af-overlay/10 bg-af-surface/96 px-3 py-2 text-sm shadow-af-card",
        className,
      )}
    >
      <p className="m-0 font-semibold text-af-ink">{String(label)}</p>
      <div className="grid gap-1">
        {payload.map((entry) => {
          const key = entry.dataKey?.toString() ?? "";
          const item = config[key];

          return (
            <div className="flex items-center justify-between gap-3 text-af-ink/78" key={key}>
              <div className="flex items-center gap-2">
                <span
                  className="h-2.5 w-2.5 rounded-full"
                  style={{ backgroundColor: item?.color ?? entry.color ?? "currentColor" }}
                />
                <span>{item?.label ?? key}</span>
              </div>
              <span className="font-medium text-af-ink">{entry.value}</span>
            </div>
          );
        })}
      </div>
    </div>
  );
}

export function ChartLegendContent({
  className,
  payload,
}: RechartsLegendContentProps & { className?: string }) {
  const config = useChartConfig();

  if (!payload?.length) {
    return null;
  }

  return (
    <div className={cn("flex flex-wrap items-center gap-4 pt-3 text-xs text-af-ink/66", className)}>
      {payload.map((entry: LegendPayload) => {
        const key = entry.dataKey?.toString() ?? "";
        const item = config[key];

        return (
          <div className="flex items-center gap-2" key={key}>
            <span
              className="h-2.5 w-2.5 rounded-full"
              style={{ backgroundColor: item?.color ?? entry.color ?? "currentColor" }}
            />
            <span>{item?.label ?? key}</span>
          </div>
        );
      })}
    </div>
  );
}
