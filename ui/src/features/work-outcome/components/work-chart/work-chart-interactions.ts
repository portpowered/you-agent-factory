import type { MouseEvent as ReactMouseEvent } from "react";
import { useMemo, useState } from "react";

import type { WorkChartData, WorkChartRow } from "../../lib/work-chart-data";

export interface WorkChartZoomRange {
  endTick: number;
  startTick: number;
}

export function useReadyWorkChartInteractions(chartData: WorkChartData) {
  const [zoomRange, setZoomRange] = useState<WorkChartZoomRange | null>(null);
  const [hiddenSeriesKeys, setHiddenSeriesKeys] = useState<ReadonlySet<string>>(
    () => new Set(),
  );
  const [selectionStartTick, setSelectionStartTick] = useState<number | null>(
    null,
  );
  const [selectionEndTick, setSelectionEndTick] = useState<number | null>(null);
  const visibleRows = useMemo(
    () => filterRowsForZoomRange(chartData.rows, zoomRange),
    [chartData.rows, zoomRange],
  );
  const selectionRange = buildSelectionRange(
    selectionStartTick,
    selectionEndTick,
  );
  const visibleSeriesKeys = chartData.series
    .filter((seriesEntry) => !hiddenSeriesKeys.has(seriesEntry.key))
    .map((seriesEntry) => seriesEntry.key);

  const resetZoom = () => {
    setZoomRange(null);
  };

  const toggleSeries = (key: string) => {
    setHiddenSeriesKeys((current) => {
      const next = new Set(current);
      if (next.has(key)) {
        next.delete(key);
      } else {
        next.add(key);
      }
      return next;
    });
  };

  const beginSelection = (event: ReactMouseEvent<HTMLDivElement>) => {
    const tick = readPointerTick(event, visibleRows);
    setSelectionStartTick(tick);
    setSelectionEndTick(tick);
  };

  const updateSelection = (event: ReactMouseEvent<HTMLDivElement>) => {
    if (selectionStartTick === null) {
      return;
    }

    const tick = readPointerTick(event, visibleRows);
    if (tick !== null) {
      setSelectionEndTick(tick);
    }
  };

  const commitSelection = (event: ReactMouseEvent<HTMLDivElement>) => {
    const endTick = readPointerTick(event, visibleRows) ?? selectionEndTick;
    const nextRange = buildSelectionRange(selectionStartTick, endTick);
    setSelectionStartTick(null);
    setSelectionEndTick(null);

    if (nextRange === null || nextRange.startTick === nextRange.endTick) {
      return;
    }

    setZoomRange(nextRange);
  };

  return {
    beginSelection,
    commitSelection,
    hiddenSeriesKeys,
    resetZoom,
    selectionRange,
    toggleSeries,
    updateSelection,
    visibleRows,
    visibleSeriesKeys,
    zoomRange,
  };
}

function filterRowsForZoomRange(
  rows: readonly WorkChartRow[],
  zoomRange: WorkChartZoomRange | null,
): WorkChartRow[] {
  if (zoomRange === null) {
    return [...rows];
  }

  const filteredRows = rows.filter(
    (row) => row.tick >= zoomRange.startTick && row.tick <= zoomRange.endTick,
  );
  return filteredRows.length >= 2 ? filteredRows : [...rows];
}

function buildSelectionRange(
  startTick: number | null,
  endTick: number | null,
): WorkChartZoomRange | null {
  if (startTick === null || endTick === null) {
    return null;
  }

  return {
    endTick: Math.max(startTick, endTick),
    startTick: Math.min(startTick, endTick),
  };
}

function readPointerTick(
  event: ReactMouseEvent<HTMLDivElement>,
  rows: readonly WorkChartRow[],
): number | null {
  if (rows.length === 0) {
    return null;
  }

  const bounds = readChartPlotBounds(event.currentTarget);
  if (!Number.isFinite(bounds.width) || bounds.width <= 0) {
    return null;
  }

  const relativeX = Math.min(
    Math.max(event.clientX - bounds.left, 0),
    bounds.width,
  );
  const rowIndex = Math.round((relativeX / bounds.width) * (rows.length - 1));
  return rows[rowIndex]?.tick ?? null;
}

function readChartPlotBounds(container: HTMLDivElement): {
  left: number;
  width: number;
} {
  const gridLine = container.querySelector<SVGLineElement>(
    ".recharts-cartesian-grid-horizontal line",
  );
  const svg = gridLine?.ownerSVGElement;
  if (gridLine && svg) {
    const x1 = Number(gridLine.getAttribute("x1"));
    const x2 = Number(gridLine.getAttribute("x2"));
    const svgRect = svg.getBoundingClientRect();
    const viewBoxWidth =
      svg.viewBox?.baseVal?.width ||
      Number(svg.getAttribute("width")) ||
      svgRect.width;

    if (
      Number.isFinite(x1) &&
      Number.isFinite(x2) &&
      Number.isFinite(svgRect.width) &&
      Number.isFinite(viewBoxWidth) &&
      svgRect.width > 0 &&
      viewBoxWidth > 0
    ) {
      const scaleX = svgRect.width / viewBoxWidth;
      const left = svgRect.left + Math.min(x1, x2) * scaleX;
      const width = Math.abs(x2 - x1) * scaleX;

      if (Number.isFinite(left) && Number.isFinite(width) && width > 0) {
        return { left, width };
      }
    }
  }

  const rect = container.getBoundingClientRect();
  return { left: rect.left, width: rect.width };
}
