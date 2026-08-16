import { useState } from "react";
import { expect, userEvent, waitFor, within } from "storybook/test";

import { DashboardActionButton } from "../../../components/ui/dashboard-action-button";
import { NoSelectionDetailCard } from "../../current-selection/base/components/detail/no-selection-detail-card";
import { WorkTotalsCard } from "../../work-totals/components/work-totals-card";
import "../../../styles.css";
import {
  AgentBentoCard,
  AgentBentoLayout,
  type AgentBentoLayoutItem,
} from "./agent-bento";

const defaultLayout: AgentBentoLayoutItem[] = [
  { h: 2, id: "summary", widgetType: "summary", w: 4, x: 0, y: 0 },
];

const multiCardLayout: AgentBentoLayoutItem[] = [
  { h: 3, id: "activity", widgetType: "activity", w: 5, x: 0, y: 0 },
  { h: 3, id: "trace", widgetType: "trace", w: 4, x: 5, y: 0 },
  { h: 3, id: "terminal", widgetType: "terminal", w: 3, x: 9, y: 0 },
];

const featureBoardLayout: AgentBentoLayoutItem[] = [
  { h: 2, id: "work-totals", widgetType: "work-totals", w: 4, x: 0, y: 0 },
  {
    h: 4,
    id: "current-selection",
    widgetType: "current-selection",
    w: 8,
    x: 4,
    y: 0,
  },
];

function card(id: string, title: string, body: string) {
  return {
    id,
    widgetType: id,
    children: (
      <AgentBentoCard title={title}>
        <p>{body}</p>
      </AgentBentoCard>
    ),
  };
}

export default {
  title: "you-agent-factory/Bento Layout",
  component: AgentBentoLayout,
};

export const Default = {
  render: () => (
    <div style={{ padding: "1rem" }}>
      <AgentBentoLayout
        cards={[
          card(
            "summary",
            "Factory summary",
            "One bento card can hold plain text.",
          ),
        ]}
        layout={defaultLayout}
      />
    </div>
  ),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);

    await expect(
      await canvas.findByRole("region", {
        name: "you-agent-factory bento board",
      }),
    ).toBeVisible();
    await expect(
      await canvas.findByRole("article", { name: "Factory summary" }),
    ).toBeVisible();
    await expect(
      await canvas.findByText("One bento card can hold plain text."),
    ).toBeVisible();
  },
};

export const MultiCard = {
  render: () => (
    <div style={{ padding: "1rem" }}>
      <AgentBentoLayout
        cards={[
          card(
            "activity",
            "Current activity",
            "Workflow graph card placeholder.",
          ),
          card(
            "trace",
            "Trace grid",
            "Trace rows remain independently testable.",
          ),
          card(
            "terminal",
            "Terminal work",
            "Completed and failed work share the board.",
          ),
        ]}
        layout={multiCardLayout}
      />
    </div>
  ),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const activity = await canvas.findByRole("article", {
      name: "Current activity",
    });
    const dragHeader = within(activity).getByRole("heading", {
      level: 3,
      name: "Current activity",
    });

    await userEvent.pointer([
      { keys: "[MouseLeft>]", target: dragHeader, coords: { x: 12, y: 8 } },
      { target: dragHeader, coords: { x: 180, y: 10 } },
      { keys: "[/MouseLeft]", target: dragHeader, coords: { x: 180, y: 10 } },
    ]);

    await expect(activity).toBeVisible();
    await expect(
      await canvas.findByRole("article", { name: "Trace grid" }),
    ).toBeVisible();
    await expect(
      await canvas.findByRole("article", { name: "Terminal work" }),
    ).toBeVisible();
  },
};

export const RealDashboardState = {
  render: () => (
    <div style={{ padding: "1rem" }}>
      <AgentBentoLayout
        cards={[
          {
            id: "work-totals",
            widgetType: "work-totals",
            children: (
              <WorkTotalsCard
                completedCount={3}
                dispatchedCount={5}
                failedCount={1}
                inFlightDispatchCount={2}
              />
            ),
          },
          {
            id: "current-selection",
            widgetType: "current-selection",
            children: <NoSelectionDetailCard />,
          },
        ]}
        initialWidth={1180}
        layout={featureBoardLayout}
      />
    </div>
  ),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);

    await expect(
      await canvas.findByRole("article", { name: "Work totals" }),
    ).toBeVisible();
    await expect(await canvas.findByText("In progress")).toBeVisible();
    await expect(
      await canvas.findByRole("article", { name: "Current selection" }),
    ).toBeVisible();
    await expect(
      await canvas.findByRole("button", { name: "Undo selection" }),
    ).toBeDisabled();
    await expect(
      await canvas.findByText(
        "Select a workstation, work item, or state node to inspect live details.",
      ),
    ).toBeVisible();
  },
};

export const ConstrainedWidth = {
  render: () => (
    <div style={{ maxWidth: "520px", padding: "1rem" }}>
      <AgentBentoLayout
        cards={[
          card(
            "activity",
            "Current activity",
            "The layout can render in a narrow shell.",
          ),
          card("trace", "Trace grid", "Cards keep their content on the board."),
        ]}
        initialWidth={520}
        layout={[
          { h: 2, id: "activity", widgetType: "activity", w: 6, x: 0, y: 0 },
          { h: 2, id: "trace", widgetType: "trace", w: 6, x: 0, y: 2 },
        ]}
      />
    </div>
  ),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);

    await expect(
      await canvas.findByText("The layout can render in a narrow shell."),
    ).toBeVisible();
    await expect(
      await canvas.findByText("Cards keep their content on the board."),
    ).toBeVisible();
  },
};

const COMPACT_OVERFLOW_WIDTHS = [320, 390, 640, 768] as const;

function CompactOverflowRegressionBoard({ width }: { width: number }) {
  const cardID = `compact-overflow-${width}`;

  return (
    <div
      data-testid={`compact-overflow-frame-${width}`}
      style={{ boxSizing: "border-box", width: `${width}px` }}
    >
      <AgentBentoLayout
        cards={[
          {
            children: (
              <AgentBentoCard
                headerAction={
                  <DashboardActionButton
                    aria-label={`Open compact card ${width}`}
                    iconOnly
                    type="button"
                  >
                    <span aria-hidden="true">↗</span>
                  </DashboardActionButton>
                }
                title={`Compact card ${width}`}
              >
                <p>Compact card content remains readable.</p>
              </AgentBentoCard>
            ),
            id: cardID,
            widgetType: "compact-card",
          },
        ]}
        initialWidth={width}
        layout={[
          { h: 2, id: cardID, widgetType: "compact-card", w: 6, x: 0, y: 0 },
        ]}
      />
    </div>
  );
}

export const CompactOverflowRegression = {
  tags: ["test"],
  render: () => (
    <div className="grid gap-4">
      {COMPACT_OVERFLOW_WIDTHS.map((width) => (
        <CompactOverflowRegressionBoard key={width} width={width} />
      ))}
    </div>
  ),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);

    for (const width of COMPACT_OVERFLOW_WIDTHS) {
      const frame = canvas.getByTestId(`compact-overflow-frame-${width}`);
      const board = within(frame).getByRole("region", {
        name: "you-agent-factory bento board",
      });

      await expect(board).toBeVisible();
      await waitFor(() => {
        expect(board.clientWidth).toBeGreaterThan(0);
        expect(board.scrollWidth).toBeLessThanOrEqual(board.clientWidth);
      });

      const item = board.querySelector<HTMLElement>(".react-grid-item");
      if (!item) {
        throw new Error(`expected compact ${width}px grid item`);
      }

      const boardWidth = board.getBoundingClientRect().width;
      const itemWidth = item.getBoundingClientRect().width;
      expect(itemWidth).toBeGreaterThanOrEqual(boardWidth - 1);
      expect(itemWidth).toBeLessThanOrEqual(boardWidth + 1);
    }
  },
};

function ResizeHandleControlIsolationCard() {
  const [exported, setExported] = useState(false);

  return (
    <AgentBentoCard bodyScroll={false} title="Resize handle control isolation">
      <div className="flex h-full min-h-0 items-end justify-end">
        <DashboardActionButton
          aria-label="Export PNG"
          iconOnly
          onClick={() => setExported(true)}
          type="button"
        >
          <span aria-hidden="true">↗</span>
        </DashboardActionButton>
        {exported ? <span className="sr-only">Export complete</span> : null}
      </div>
    </AgentBentoCard>
  );
}

export const ResizeHandleControlIsolation = {
  tags: ["test"],
  render: () => (
    <div style={{ maxWidth: "720px", padding: "1rem", width: "100%" }}>
      <AgentBentoLayout
        cards={[
          {
            children: <ResizeHandleControlIsolationCard />,
            id: "resize-handle-control-isolation",
            widgetType: "resize-handle-control-isolation",
          },
        ]}
        initialWidth={720}
        layout={[
          {
            h: 2,
            id: "resize-handle-control-isolation",
            widgetType: "resize-handle-control-isolation",
            w: 12,
            x: 0,
            y: 0,
          },
        ]}
        responsiveMode="interactive"
      />
    </div>
  ),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const exportButton = await canvas.findByRole("button", {
      name: "Export PNG",
    });
    const resizeHandle = canvasElement.querySelector<HTMLElement>(
      ".react-resizable-handle-se",
    );

    if (!resizeHandle) {
      throw new Error("expected southeast bento resize handle");
    }

    const exportBounds = exportButton.getBoundingClientRect();
    const topmost = document.elementFromPoint(
      exportBounds.left + exportBounds.width / 2,
      exportBounds.top + exportBounds.height / 2,
    );
    expect(topmost === exportButton || exportButton.contains(topmost)).toBe(
      true,
    );
    expect(resizeHandle.getBoundingClientRect().width).toBeGreaterThanOrEqual(
      44,
    );
    expect(resizeHandle.getBoundingClientRect().height).toBeGreaterThanOrEqual(
      44,
    );

    await userEvent.click(exportButton);
    await expect(await canvas.findByText("Export complete")).toBeVisible();
  },
};
