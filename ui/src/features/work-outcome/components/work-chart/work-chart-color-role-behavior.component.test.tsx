// @component-test-runner vitest
// @vitest-environment happy-dom

import path from "node:path";
import { fileURLToPath } from "node:url";
import { render, screen } from "@testing-library/react";
import { afterAll, beforeAll, describe, expect, it } from "vitest";

import { installDashboardBrowserTestShims } from "../../../../components/dashboard/test-browser-shims";
import { resolveCssColor } from "../../../../styles/palette-contrast-test-math";
import { compileDashboardStyles } from "../../../../test-support/compile-dashboard-styles";
import { applyDocumentColorPalette } from "../../../../theme/app-color-palette";
import { COLOR_PALETTE_IDS } from "../../../../theme/color-palette";
import { getDashboardWorkChartSeriesStyle } from "../../lib/chart-contract";
import type { WorkChartModel } from "../../lib/trends";
import { WorkChart, type WorkChartSeriesDefinition } from "./work-chart";

const stylesSourcePath = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "../../../../styles.css",
);

const chartModel: WorkChartModel = {
  delta: { queued: 2, inFlight: 3, completed: 4, failed: 1 },
  failureGroups: [],
  points: [{ label: "Tick 10", observedAt: 1000, order: 0, tick: 10 }],
  rangeID: "15m",
  rangeLabel: "15m",
  samples: [
    {
      completedCount: 4,
      dispatchedCount: 8,
      failedByWorkType: { story: 1 },
      failedCount: 1,
      failedWorkLabels: ["story-failure"],
      inFlightCount: 3,
      observedAt: 1000,
      queuedCount: 2,
      tick: 10,
    },
  ],
  series: [
    {
      key: "queued",
      label: "Queued",
      points: [{ label: "Queued: 2", observedAt: 1000, order: 0, value: 2 }],
      unit: "count",
    },
    {
      key: "inFlight",
      label: "In-flight",
      points: [{ label: "In-flight: 3", observedAt: 1000, order: 0, value: 3 }],
      unit: "count",
    },
    {
      key: "completed",
      label: "Completed",
      points: [{ label: "Completed: 4", observedAt: 1000, order: 0, value: 4 }],
      unit: "count",
    },
    {
      key: "failed",
      label: "Failed/retried",
      points: [{ label: "Failed: 1", observedAt: 1000, order: 0, value: 1 }],
      unit: "count",
    },
  ],
};

const chartSeries: readonly WorkChartSeriesDefinition[] = [
  {
    key: "queued",
    label: "Queued",
    ...getDashboardWorkChartSeriesStyle("queued"),
  },
  {
    key: "inFlight",
    label: "In-flight",
    ...getDashboardWorkChartSeriesStyle("inFlight"),
  },
  {
    key: "completed",
    label: "Completed",
    ...getDashboardWorkChartSeriesStyle("completed"),
  },
  {
    key: "failed",
    label: "Failed/retried",
    ...getDashboardWorkChartSeriesStyle("failed"),
  },
];

function injectCompiledRootRules(compiledCss: string): void {
  const rootBlocks = compiledCss.match(/:root[^{]*\{[^}]*\}/g) ?? [];
  const paletteBlocks =
    compiledCss.match(/\[data-color-palette="[^"]+"\][^{]*\{[^}]*\}/g) ?? [];
  const style = document.createElement("style");
  style.textContent = [...rootBlocks, ...paletteBlocks].join("\n");
  document.head.appendChild(style);
}

const restoreBrowserShims = installDashboardBrowserTestShims();

beforeAll(async () => {
  const compiledCss = await compileDashboardStyles(stylesSourcePath);
  injectCompiledRootRules(compiledCss);
});

afterAll(() => {
  restoreBrowserShims();
});

describe("WorkChart palette role behavior", () => {
  it("renders all four status series with palette-backed role aliases", () => {
    applyDocumentColorPalette("factory-dark");
    render(
      <WorkChart
        ariaLabel="Work chart palette behavior"
        model={chartModel}
        series={chartSeries}
      />,
    );

    const chart = screen.getByRole("img", {
      name: "Work chart palette behavior",
    });
    const renderedSeries = [
      ...chart.querySelectorAll<HTMLElement>("[data-chart-series]"),
    ];

    expect(renderedSeries.map((node) => node.dataset.chartSeries)).toEqual([
      "queued",
      "inFlight",
      "completed",
      "failed",
    ]);
    expect(
      renderedSeries.every(
        (node) => node.dataset.chartSeriesHidden === "false",
      ),
    ).toBe(true);
    expect(chart.textContent).toContain("Queued");
    expect(chart.textContent).toContain("In-flight");
    expect(chart.textContent).toContain("Completed");
    expect(chart.textContent).toContain("Failed/retried");

    const renderedTokens = renderedSeries.map(
      (node) => node.dataset.chartSeriesColor ?? "",
    );
    expect(renderedTokens).toEqual([
      "var(--color-af-chart-queued)",
      "var(--color-af-chart-in-flight)",
      "var(--color-af-chart-completed)",
      "var(--color-af-chart-failed)",
    ]);

    for (const paletteID of COLOR_PALETTE_IDS) {
      applyDocumentColorPalette(paletteID);
      const computedStyle = getComputedStyle(document.documentElement);
      const colors = renderedTokens.map((token) =>
        resolveCssColor(
          paletteID,
          token.slice("var(".length, -1),
          computedStyle,
        ).rgb.join(","),
      );
      expect(
        colors.every((color) => color.length > 0),
        `${paletteID} rendered chart strokes should resolve through compiled role CSS`,
      ).toBe(true);
      expect(new Set(colors).size).toBe(4);
    }
  });
});
