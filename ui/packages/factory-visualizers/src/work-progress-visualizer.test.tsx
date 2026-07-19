// @vitest-environment happy-dom

import { render, screen } from "@testing-library/react";
import type { FactoryWorkProgressProjection } from "@you-agent-factory/factory-replay";
import { axe } from "jest-axe";
import { describe, expect, it, vi } from "vitest";

import {
  WorkProgressVisualizer,
  type WorkProgressVisualizerMessages,
} from "./work-progress-visualizer";

const messages: WorkProgressVisualizerMessages = {
  categories: {
    queued: {
      singular: (count) => `${count} queued item`,
      plural: (count) => `${count} queued items`,
    },
    active: {
      singular: (count) => `${count} active item`,
      plural: (count) => `${count} active items`,
    },
    completed: {
      singular: (count) => `${count} completed item`,
      plural: (count) => `${count} completed items`,
    },
    failed: {
      singular: (count) => `${count} failed item`,
      plural: (count) => `${count} failed items`,
    },
    unclassified: {
      singular: (count) => `${count} unclassified item`,
      plural: (count) => `${count} unclassified items`,
    },
  },
  empty: "No work is present in this projection.",
  regionLabel: "Factory work progress",
  title: "Work progress",
  total: (count) => `${count} total items`,
};

function item(id: string) {
  return { id };
}

function projection(
  counts: FactoryWorkProgressProjection["counts"],
): FactoryWorkProgressProjection {
  const categories = {
    active: Array.from({ length: counts.active }, (_, index) =>
      item(`active-${index}`),
    ),
    completed: Array.from({ length: counts.completed }, (_, index) =>
      item(`completed-${index}`),
    ),
    failed: Array.from({ length: counts.failed }, (_, index) =>
      item(`failed-${index}`),
    ),
    queued: Array.from({ length: counts.queued }, (_, index) =>
      item(`queued-${index}`),
    ),
    unclassified: Array.from({ length: counts.unclassified }, (_, index) =>
      item(`unclassified-${index}`),
    ),
  };

  return {
    ...categories,
    counts,
    selectedTick: 12,
    total: Object.values(counts).reduce((total, count) => total + count, 0),
  };
}

describe("WorkProgressVisualizer", () => {
  it("renders every mutually exclusive category and the projection total", () => {
    const input = projection({
      queued: 1,
      active: 2,
      completed: 3,
      failed: 4,
      unclassified: 5,
    });

    render(
      <WorkProgressVisualizer
        formatNumber={String}
        messages={messages}
        projection={input}
      />,
    );

    expect(
      screen.getByRole("region", { name: "Factory work progress" }),
    ).toHaveAttribute("data-work-progress-total", "15");
    expect(screen.getByText("1 queued item")).toBeInTheDocument();
    expect(screen.getByText("2 active items")).toBeInTheDocument();
    expect(screen.getByText("3 completed items")).toBeInTheDocument();
    expect(screen.getByText("4 failed items")).toBeInTheDocument();
    expect(screen.getByText("5 unclassified items")).toBeInTheDocument();
    expect(screen.getByText("15 total items")).toBeInTheDocument();
  });

  it("renders an explicit empty presentation for an all-zero projection", () => {
    render(
      <WorkProgressVisualizer
        formatNumber={String}
        messages={messages}
        projection={projection({
          queued: 0,
          active: 0,
          completed: 0,
          failed: 0,
          unclassified: 0,
        })}
      />,
    );

    expect(screen.getByRole("status")).toHaveTextContent(
      "No work is present in this projection.",
    );
    expect(screen.queryByRole("list")).not.toBeInTheDocument();
    expect(screen.getByText("0 total items")).toBeInTheDocument();
  });

  it("has no automated accessibility violations with every status category", async () => {
    const { container } = render(
      <WorkProgressVisualizer
        formatNumber={String}
        messages={messages}
        projection={projection({
          queued: 1,
          active: 1,
          completed: 1,
          failed: 1,
          unclassified: 1,
        })}
      />,
    );

    expect(await axe(container)).toHaveNoViolations();
  });
});

describe("WorkProgressVisualizer host control", () => {
  it("uses host formatters and singular/plural messages", () => {
    const formatNumber = new Intl.NumberFormat("de-DE").format;
    const localizedMessages: WorkProgressVisualizerMessages = {
      ...messages,
      categories: {
        ...messages.categories,
        queued: {
          singular: (count) => `${count} Auftrag wartet`,
          plural: (count) => `${count} Aufträge warten`,
        },
      },
      total: (count) => `${count} Aufträge insgesamt`,
    };

    render(
      <WorkProgressVisualizer
        formatNumber={formatNumber}
        messages={localizedMessages}
        projection={projection({
          queued: 1234,
          active: 0,
          completed: 0,
          failed: 0,
          unclassified: 0,
        })}
      />,
    );

    expect(screen.getByText("1.234 Aufträge warten")).toBeInTheDocument();
    expect(screen.getByText("1.234 Aufträge insgesamt")).toBeInTheDocument();
  });

  it("replaces controlled values without mutating either projection", () => {
    const initial = projection({
      queued: 1,
      active: 0,
      completed: 0,
      failed: 0,
      unclassified: 0,
    });
    const replacement = projection({
      queued: 0,
      active: 1,
      completed: 1,
      failed: 0,
      unclassified: 0,
    });
    const initialSnapshot = structuredClone(initial);
    const replacementSnapshot = structuredClone(replacement);
    const formatNumber = vi.fn(String);
    const { rerender } = render(
      <WorkProgressVisualizer
        formatNumber={formatNumber}
        messages={messages}
        projection={initial}
      />,
    );

    expect(screen.getByText("1 queued item")).toBeInTheDocument();

    rerender(
      <WorkProgressVisualizer
        formatNumber={formatNumber}
        messages={messages}
        projection={replacement}
      />,
    );

    expect(screen.queryByText("1 queued item")).not.toBeInTheDocument();
    expect(screen.getByText("1 active item")).toBeInTheDocument();
    expect(screen.getByText("1 completed item")).toBeInTheDocument();
    expect(screen.getByText("2 total items")).toBeInTheDocument();
    expect(initial).toEqual(initialSnapshot);
    expect(replacement).toEqual(replacementSnapshot);
  });
});
