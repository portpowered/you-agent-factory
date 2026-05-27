import type { MouseEvent as ReactMouseEvent } from "react";
import { useMemo, useState } from "react";

import type { WorkChartData, WorkChartRow } from "../lib/work-chart-data";

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

  const rect = event.currentTarget.getBoundingClientRect();
  if (!Number.isFinite(rect.width) || rect.width <= 0) {
    return null;
  }

  const relativeX = Math.min(
    Math.max(event.clientX - rect.left, 0),
    rect.width,
  );
  const rowIndex = Math.round((relativeX / rect.width) * (rows.length - 1));
  return rows[rowIndex]?.tick ?? null;
}
