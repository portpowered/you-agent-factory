import type { WorkChartModel, WorkChartSeriesKey } from "./trends";

export interface WorkChartSeriesDefinition {
  key: WorkChartSeriesKey;
  label: string;
  lineColor: string;
  lineClassName: string;
  pointClassName?: string;
  pointRadius?: number;
}

export interface WorkChartBuiltSeries {
  key: string;
  label: string;
  lineColor: string;
  lineClassName: string;
  pointClassName?: string;
  pointRadius?: number;
  strokeDasharray?: string;
}

export interface WorkChartData {
  config: Record<string, { color: string; label: string }>;
  rows: WorkChartRow[];
  series: WorkChartBuiltSeries[];
}

export interface WorkChartRow {
  label: string;
  tick: number;
  [seriesKey: string]: number | string | undefined;
}

export type WorkChartDataResult =
  | { data: WorkChartData; status: "ready" }
  | { status: "empty" }
  | { status: "invalid" };

export function buildWorkChartData(
  model: WorkChartModel | undefined,
  series: readonly WorkChartSeriesDefinition[],
): WorkChartDataResult {
  if (!isWorkChartModel(model) || !isWorkChartSeriesDefinitionArray(series)) {
    return { status: "invalid" };
  }

  if (model.points.length === 0 || series.length === 0) {
    return { status: "empty" };
  }

  const seriesByKey = new Map(
    model.series.map((definition) => [definition.key, definition.points]),
  );
  const rows = model.points.map((point, index) => {
    const row: WorkChartRow = {
      label: point.label,
      tick: point.tick,
    };

    for (const definition of series) {
      const value = seriesByKey
        .get(definition.key)
        ?.find((seriesPoint) => seriesPoint.order === index)?.value;
      if (value !== undefined) {
        row[definition.key] = value;
      }
    }

    return row;
  });

  const builtSeries = series
    .filter((definition) =>
      rows.some((row) => hasSeriesValue(row, definition.key)),
    )
    .map((definition) => ({
      key: definition.key,
      label: definition.label,
      lineClassName: definition.lineClassName,
      lineColor: definition.lineColor,
      pointClassName: definition.pointClassName,
      pointRadius: definition.pointRadius,
      strokeDasharray: extractStrokeDasharray(definition.lineClassName),
    }));

  return {
    data: {
      config: Object.fromEntries(
        builtSeries.map((seriesEntry) => [
          seriesEntry.key,
          { color: seriesEntry.lineColor, label: seriesEntry.label },
        ]),
      ),
      rows,
      series: builtSeries,
    },
    status: "ready",
  };
}

function hasSeriesValue(row: WorkChartRow, key: string): boolean {
  return Object.hasOwn(row, key) && typeof row[key] === "number";
}

function extractStrokeDasharray(className: string): string | undefined {
  const dashArrayMatch = className.match(/\[stroke-dasharray:([^\]]+)\]/);
  return dashArrayMatch?.[1]?.replaceAll("_", " ");
}

function isWorkChartSeriesDefinitionArray(
  value: unknown,
): value is readonly WorkChartSeriesDefinition[] {
  return Array.isArray(value) && value.every(isWorkChartSeriesDefinition);
}

function isWorkChartSeriesDefinition(
  value: unknown,
): value is WorkChartSeriesDefinition {
  return (
    isRecord(value) &&
    typeof value.key === "string" &&
    typeof value.label === "string" &&
    typeof value.lineColor === "string" &&
    typeof value.lineClassName === "string" &&
    (value.pointClassName === undefined ||
      typeof value.pointClassName === "string") &&
    (value.pointRadius === undefined || isFiniteNumber(value.pointRadius))
  );
}

function isWorkChartModel(value: unknown): value is WorkChartModel {
  return (
    isRecord(value) &&
    Array.isArray(value.points) &&
    Array.isArray(value.series) &&
    value.points.every(isWorkChartSample) &&
    value.series.every(isWorkChartSeries)
  );
}

function isWorkChartSample(
  value: unknown,
): value is WorkChartModel["points"][number] {
  return (
    isRecord(value) &&
    typeof value.label === "string" &&
    isFiniteNumber(value.observedAt) &&
    isFiniteNumber(value.order) &&
    isFiniteNumber(value.tick)
  );
}

function isWorkChartSeries(
  value: unknown,
): value is WorkChartModel["series"][number] {
  return (
    isRecord(value) &&
    typeof value.key === "string" &&
    typeof value.label === "string" &&
    Array.isArray(value.points) &&
    value.points.every(isWorkChartSeriesPoint)
  );
}

function isWorkChartSeriesPoint(
  value: unknown,
): value is WorkChartModel["series"][number]["points"][number] {
  return (
    isRecord(value) &&
    typeof value.label === "string" &&
    isFiniteNumber(value.observedAt) &&
    isFiniteNumber(value.order) &&
    isFiniteNumber(value.value)
  );
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function isFiniteNumber(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value);
}
