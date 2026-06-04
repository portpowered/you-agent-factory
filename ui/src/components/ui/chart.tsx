import type {
  CSSProperties,
  HTMLAttributes,
  MouseEvent as ReactMouseEvent,
  ReactNode,
} from "react";
import { createContext, useContext } from "react";
import {
  Legend as RechartsLegend,
  Tooltip as RechartsTooltip,
  ResponsiveContainer,
  type TooltipContentProps,
} from "recharts";
import type {
  LegendPayload,
  Props as RechartsLegendContentProps,
} from "recharts/types/component/DefaultLegendContent";
import type {
  NameType,
  ValueType,
} from "recharts/types/component/DefaultTooltipContent";
import { cn } from "../../lib/cn";
import { Button } from "./button";

export interface ChartConfigEntry {
  color: string;
  label: string;
}

export type ChartConfig = Record<string, ChartConfigEntry>;

const ChartContext = createContext<ChartConfig | null>(null);

export type ChartPresentation = "embedded" | "standalone";

// tailwind-exception: intrinsic-sizing
const CHART_CONTAINER_STANDALONE_CLASS =
  "relative h-[18rem] rounded-2xl border border-outline bg-surface-container-low p-4 text-on-surface";
// tailwind-exception: intrinsic-sizing
const CHART_CONTAINER_EMBEDDED_CLASS =
  "relative flex h-full min-h-0 min-w-0 w-full flex-1 flex-col text-on-surface";
const DEFAULT_CHART_INITIAL_DIMENSION = { height: 288, width: 640 } as const;

function chartContainerClassName(
  presentation: ChartPresentation,
  className?: string,
): string {
  return cn(
    presentation === "embedded"
      ? CHART_CONTAINER_EMBEDDED_CLASS
      : CHART_CONTAINER_STANDALONE_CLASS,
    className,
  );
}

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
  footer,
  interactionAttributes,
  overlay,
  presentation = "standalone",
  rootAttributes,
  style,
  title,
}: {
  children: ReactNode;
  className?: string;
  config: ChartConfig;
  footer?: ReactNode;
  interactionAttributes?: HTMLAttributes<HTMLDivElement>;
  overlay?: ReactNode;
  presentation?: ChartPresentation;
  rootAttributes?: Record<string, string>;
  style?: CSSProperties;
  title: string;
}) {
  const chartSurface = (
    <ResponsiveContainer
      height="100%"
      initialDimension={DEFAULT_CHART_INITIAL_DIMENSION}
      minHeight={0}
      minWidth={0}
      width="100%"
    >
      {children}
    </ResponsiveContainer>
  );

  return (
    <ChartContext.Provider value={config}>
      <div
        aria-label={title}
        className={chartContainerClassName(presentation, className)}
        data-chart-container=""
        data-chart-presentation={presentation}
        role="img"
        {...interactionAttributes}
        {...rootAttributes}
        style={
          {
            ...Object.fromEntries(
              Object.entries(config).map(([key, value]) => [
                `--color-${key}`,
                value.color,
              ]),
            ),
            ...style,
          } as CSSProperties
        }
      >
        {overlay ? (
          <div className="pointer-events-none absolute inset-0">{overlay}</div>
        ) : null}
        {footer ? (
          <div className="flex min-h-0 flex-1 flex-col">
            <div className="min-h-0 flex-1">{chartSurface}</div>
            {footer}
          </div>
        ) : (
          chartSurface
        )}
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
}: TooltipContentProps<ValueType, NameType> & { className?: string }) {
  const config = useChartConfig();

  if (!active || !payload?.length) {
    return null;
  }

  return (
    <div
      className={cn(
        "grid min-w-40 gap-2 rounded-xl border border-outline bg-surface-container-high px-3 py-2 text-sm shadow-af-card",
        className,
      )}
    >
      <p className="m-0 font-semibold text-on-surface">{String(label)}</p>
      <div className="grid gap-1">
        {payload.map((entry) => {
          const key = entry.dataKey?.toString() ?? "";
          const item = config[key];

          return (
            <div
              className="flex items-center justify-between gap-3 text-on-surface-variant"
              key={key}
            >
              <div className="flex items-center gap-2">
                <span
                  className="h-2.5 w-2.5 rounded-full"
                  style={{
                    backgroundColor:
                      item?.color ?? entry.color ?? "currentColor",
                  }}
                />
                <span>{item?.label ?? key}</span>
              </div>
              <span className="font-medium text-on-surface">{entry.value}</span>
            </div>
          );
        })}
      </div>
    </div>
  );
}

export function ChartLegendContent({
  className,
  getToggleLabel,
  hiddenSeries = new Set<string>(),
  itemClassName,
  onToggleSeries,
  payload,
  swatchClassName,
}: RechartsLegendContentProps & {
  className?: string;
  getToggleLabel?: (label: string, hidden: boolean) => string;
  hiddenSeries?: ReadonlySet<string>;
  itemClassName?: string;
  onToggleSeries?: (key: string) => void;
  swatchClassName?: string;
}) {
  const config = useChartConfig();

  if (!payload?.length) {
    return null;
  }

  const stopChartInteraction = (event: ReactMouseEvent<HTMLButtonElement>) => {
    event.stopPropagation();
  };

  return (
    <div
      className={cn(
        "flex flex-wrap items-center gap-4 pt-3 text-xs text-on-surface-variant",
        className,
      )}
    >
      {payload.map((entry: LegendPayload) => {
        const key = entry.dataKey?.toString() ?? "";
        const item = config[key];
        const label = item?.label ?? key;
        const hidden = hiddenSeries.has(key);
        const content = (
          <>
            <span
              aria-hidden="true"
              className={cn(
                "rounded-full",
                swatchClassName ?? "h-2.5 w-2.5",
                hidden ? "border border-outline bg-surface-container-low" : "",
              )}
              style={{
                backgroundColor: hidden
                  ? undefined
                  : (item?.color ?? entry.color ?? "currentColor"),
              }}
            />
            <span>{label}</span>
          </>
        );

        if (!onToggleSeries) {
          return (
            <div
              className={cn("flex items-center gap-2", itemClassName)}
              key={key}
            >
              {content}
            </div>
          );
        }

        return (
          <Button
            aria-label={getToggleLabel?.(label, hidden) ?? label}
            aria-pressed={!hidden}
            className={cn(
              "min-h-0 rounded-md border-transparent px-0 py-1 text-left font-normal focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-af-focus",
              hidden ? "text-on-surface-disabled" : "",
              itemClassName,
            )}
            data-chart-legend-series={key}
            data-chart-legend-series-hidden={hidden ? "true" : "false"}
            key={key}
            onClick={(event) => {
              event.stopPropagation();
              onToggleSeries(key);
            }}
            onMouseDown={stopChartInteraction}
            size="sm"
            tone="ghost"
          >
            {content}
          </Button>
        );
      })}
    </div>
  );
}
